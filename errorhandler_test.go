// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package sypl_test

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/thalesfsp/sypl"
	"github.com/thalesfsp/sypl/level"
	"github.com/thalesfsp/sypl/message"
	"github.com/thalesfsp/sypl/output"
)

var errWriteBoom = errors.New("write boom")

// namedFailingOutput always errors on Write, keeping its wrapped identity.
type namedFailingOutput struct {
	output.IOutput
}

func (f *namedFailingOutput) Write(_ message.IMessage) error {
	return errWriteBoom
}

// errCollector is a concurrency-safe error sink.
type errCollector struct {
	mu   sync.Mutex
	errs []error
}

func (c *errCollector) handler() func(err error) {
	return func(err error) {
		c.mu.Lock()
		defer c.mu.Unlock()

		c.errs = append(c.errs, err)
	}
}

func (c *errCollector) snapshot() []error {
	c.mu.Lock()
	defer c.mu.Unlock()

	return append([]error{}, c.errs...)
}

// Getter/setter surface: nil by default, chainable, retrievable.
func TestErrorHandler_GetSet(t *testing.T) {
	l := sypl.New("errorhandler-getset")

	if l.GetErrorHandler() != nil {
		t.Fatal("GetErrorHandler() != nil on a fresh logger")
	}

	called := false

	if got := l.SetErrorHandler(func(_ error) { called = true }); got != l {
		t.Fatal("SetErrorHandler must return the same *Sypl for chaining")
	}

	h := l.GetErrorHandler()

	if h == nil {
		t.Fatal("GetErrorHandler() = nil after SetErrorHandler")
	}

	h(errWriteBoom)

	if !called {
		t.Fatal("retrieved handler is not the registered one")
	}
}

// A failing output must deliver the error to the handler, wrapped with the
// output's name - on the INLINE (single output) path.
func TestErrorHandler_SingleOutputDelivery(t *testing.T) {
	_, o := output.SafeBuffer(level.Trace)
	o.SetName("FailingSingle")

	collector := &errCollector{}

	l := sypl.New("errorhandler-single", &namedFailingOutput{IOutput: o})
	l.SetErrorHandler(collector.handler())

	l.Println(level.Info, "will fail")

	errs := collector.snapshot()

	if len(errs) != 1 {
		t.Fatalf("handler called %d times, want 1", len(errs))
	}

	if !errors.Is(errs[0], errWriteBoom) {
		t.Fatalf("handler error %v does not wrap the output's error", errs[0])
	}

	if !strings.Contains(errs[0].Error(), "output FailingSingle:") {
		t.Fatalf("handler error %q lacks the output-name context", errs[0].Error())
	}
}

// Both failing outputs must deliver on the concurrent fan-out path - one
// error each, wrapped with their own names.
func TestErrorHandler_MultiOutputDelivery(t *testing.T) {
	_, oA := output.SafeBuffer(level.Trace)
	oA.SetName("FailA")

	_, oB := output.SafeBuffer(level.Trace)
	oB.SetName("FailB")

	collector := &errCollector{}

	l := sypl.New(
		"errorhandler-multi",
		&namedFailingOutput{IOutput: oA},
		&namedFailingOutput{IOutput: oB},
	)
	l.SetErrorHandler(collector.handler())

	l.Println(level.Info, "will fail twice")

	errs := collector.snapshot()

	if len(errs) != 2 {
		t.Fatalf("handler called %d times, want 2", len(errs))
	}

	names := []string{}

	for _, err := range errs {
		if !errors.Is(err, errWriteBoom) {
			t.Fatalf("handler error %v does not wrap the output's error", err)
		}

		names = append(names, err.Error())
	}

	joined := strings.Join(names, "|")

	if !strings.Contains(joined, "output FailA:") || !strings.Contains(joined, "output FailB:") {
		t.Fatalf("handler errors lack per-output name context: %v", names)
	}
}

// No handler: errors stay silently swallowed (historical behavior), and a
// SUCCESSFUL write never invokes the handler.
func TestErrorHandler_NilHandlerAndSuccessSilent(t *testing.T) {
	// nil handler + failing output: must not panic.
	_, oFail := output.SafeBuffer(level.Trace)

	sypl.New("errorhandler-nil", &namedFailingOutput{IOutput: oFail}).
		Println(level.Info, "silently swallowed")

	// Handler + healthy output: must not be called.
	buf, oOK := output.SafeBuffer(level.Trace)

	collector := &errCollector{}

	l := sypl.New("errorhandler-success", oOK)
	l.SetErrorHandler(collector.handler())

	l.Println(level.Info, "healthy")

	if errs := collector.snapshot(); len(errs) != 0 {
		t.Fatalf("handler called %d times on a successful write, want 0", len(errs))
	}

	if !strings.Contains(buf.String(), "healthy") {
		t.Fatalf("healthy write lost the message: %q", buf.String())
	}
}

// The handler must NOT run holding sypl's mutex: reconfiguring the logger
// from inside the handler must not deadlock.
func TestErrorHandler_NotCalledHoldingMutex(t *testing.T) {
	_, o := output.SafeBuffer(level.Trace)

	l := sypl.New("errorhandler-mutex", &namedFailingOutput{IOutput: o})

	reconfigured := false

	l.SetErrorHandler(func(_ error) {
		// Both need sypl's write lock: they deadlock if the handler is
		// invoked while the mutex is held.
		l.SetTags("from-handler")
		l.SetFields(nil)

		reconfigured = true
	})

	done := make(chan struct{})

	go func() {
		defer close(done)

		l.Println(level.Info, "fail under mutex probe")
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("deadlock: handler was invoked while sypl's mutex was held")
	}

	if !reconfigured {
		t.Fatal("handler did not run")
	}
}

// Concurrent failing writes + concurrent handler reconfiguration must be
// race-clean.
func TestErrorHandler_ConcurrentRaceClean(t *testing.T) {
	_, o := output.SafeBuffer(level.Trace)

	collector := &errCollector{}

	l := sypl.New("errorhandler-race", &namedFailingOutput{IOutput: o})

	const goroutines = 8

	var wg sync.WaitGroup

	wg.Add(goroutines * 2)

	for range goroutines {
		go func() {
			defer wg.Done()

			for range 25 {
				l.Println(level.Info, "concurrent failure")
			}
		}()

		go func() {
			defer wg.Done()

			for range 25 {
				l.SetErrorHandler(collector.handler())
				l.SetErrorHandler(nil)
			}
		}()
	}

	wg.Wait()
}
