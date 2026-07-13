// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package sypl_test

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"

	"github.com/thalesfsp/sypl"
	"github.com/thalesfsp/sypl/level"
	"github.com/thalesfsp/sypl/output"
)

var (
	errFlushBoom = errors.New("flush boom")
	errCloseBoom = errors.New("close boom")
)

// callLog is a concurrency-safe ordered call recorder.
type callLog struct {
	mu    sync.Mutex
	calls []string
}

func (c *callLog) add(call string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.calls = append(c.calls, call)
}

func (c *callLog) snapshot() []string {
	c.mu.Lock()
	defer c.mu.Unlock()

	return append([]string{}, c.calls...)
}

// flushCloseOutput wraps a real output with Flush/Close capabilities -
// exactly the small interfaces `(*Sypl).Flush`/`(*Sypl).Close` detect.
type flushCloseOutput struct {
	output.IOutput

	log      *callLog
	flushErr error
	closeErr error
}

func (f *flushCloseOutput) Flush() error {
	f.log.add("flush:" + f.GetName())

	return f.flushErr
}

func (f *flushCloseOutput) Close() error {
	f.log.add("close:" + f.GetName())

	return f.closeErr
}

// Flush calls every flush-capable output in REGISTRATION order, skipping
// outputs lacking the capability, and returns nil when all succeed.
func TestFlush_RegistrationOrderAndSkip(t *testing.T) {
	log := &callLog{}

	_, oA := output.SafeBuffer(level.Trace)
	oA.SetName("A")

	_, oPlain := output.SafeBuffer(level.Trace)
	oPlain.SetName("Plain") // No Flush/Close: must be skipped.

	_, oB := output.SafeBuffer(level.Trace)
	oB.SetName("B")

	l := sypl.New(
		"flush-order",
		&flushCloseOutput{IOutput: oA, log: log},
		oPlain,
		&flushCloseOutput{IOutput: oB, log: log},
	)

	if err := l.Flush(); err != nil {
		t.Fatalf("Flush() = %v, want nil", err)
	}

	want := []string{"flush:A", "flush:B"}

	if got := log.snapshot(); !equalStrings(got, want) {
		t.Fatalf("Flush calls = %v, want %v", got, want)
	}
}

// Close mirrors Flush: registration order, skip, nil on success.
func TestClose_RegistrationOrderAndSkip(t *testing.T) {
	log := &callLog{}

	_, oA := output.SafeBuffer(level.Trace)
	oA.SetName("A")

	_, oPlain := output.SafeBuffer(level.Trace)
	oPlain.SetName("Plain")

	_, oB := output.SafeBuffer(level.Trace)
	oB.SetName("B")

	l := sypl.New(
		"close-order",
		&flushCloseOutput{IOutput: oA, log: log},
		oPlain,
		&flushCloseOutput{IOutput: oB, log: log},
	)

	if err := l.Close(); err != nil {
		t.Fatalf("Close() = %v, want nil", err)
	}

	want := []string{"close:A", "close:B"}

	if got := log.snapshot(); !equalStrings(got, want) {
		t.Fatalf("Close calls = %v, want %v", got, want)
	}
}

// ALL errors are aggregated via errors.Join - a failing output does not
// shadow its siblings, and every output is still visited.
func TestFlushClose_ErrorsAggregate(t *testing.T) {
	log := &callLog{}

	_, oA := output.SafeBuffer(level.Trace)
	oA.SetName("A")

	_, oB := output.SafeBuffer(level.Trace)
	oB.SetName("B")

	errB2 := fmt.Errorf("wrapped: %w", errCloseBoom)

	l := sypl.New(
		"flushclose-errors",
		&flushCloseOutput{IOutput: oA, log: log, flushErr: errFlushBoom, closeErr: errCloseBoom},
		&flushCloseOutput{IOutput: oB, log: log, flushErr: errFlushBoom, closeErr: errB2},
	)

	flushErr := l.Flush()

	if !errors.Is(flushErr, errFlushBoom) {
		t.Fatalf("Flush() = %v, want it to wrap %v", flushErr, errFlushBoom)
	}

	closeErr := l.Close()

	if !errors.Is(closeErr, errCloseBoom) {
		t.Fatalf("Close() = %v, want it to wrap %v", closeErr, errCloseBoom)
	}

	// Both outputs must have been visited for BOTH operations.
	want := []string{"flush:A", "flush:B", "close:A", "close:B"}

	if got := log.snapshot(); !equalStrings(got, want) {
		t.Fatalf("calls = %v, want %v", got, want)
	}
}

// A logger with no flush/close-capable outputs returns nil from both.
func TestFlushClose_NoCapableOutputs(t *testing.T) {
	_, o := output.SafeBuffer(level.Trace)

	l := sypl.New("flushclose-none", o)

	if err := l.Flush(); err != nil {
		t.Fatalf("Flush() = %v, want nil", err)
	}

	if err := l.Close(); err != nil {
		t.Fatalf("Close() = %v, want nil", err)
	}
}

// Fatal must best-effort Flush BEFORE os.Exit(1) - and flush errors go to
// the error handler. Asserted via subprocess re-exec: the child's stderr
// carries the flush, and handler markers.
func TestFlushClose_FatalFlushesBeforeExit(t *testing.T) {
	if os.Getenv("SYPL_TEST_FATAL_FLUSH") == "1" {
		log := &callLog{}

		_, o := output.SafeBuffer(level.Trace)
		o.SetName("FatalFlush")

		stub := &flushCloseOutput{IOutput: o, log: log, flushErr: errFlushBoom}

		l := sypl.New("fatal-flush", stub)

		// The fatal-path Flush error reaches the handler BEFORE os.Exit -
		// the marker carries both the recorded Flush call, and the error.
		l.SetErrorHandler(func(err error) {
			fmt.Fprintf(os.Stderr, "FLUSHED-MARKER calls=%v err=%v\n", log.snapshot(), err)
		})

		l.Fatal("fatal flushes")

		os.Exit(42) // Sentinel: Fatal did not exit.
	}

	//nolint:gosec // Re-running the test binary itself.
	cmd := exec.Command(os.Args[0], "-test.run=TestFlushClose_FatalFlushesBeforeExit$")

	cmd.Env = append(os.Environ(), "SYPL_TEST_FATAL_FLUSH=1")

	var stderr bytes.Buffer

	cmd.Stderr = &stderr

	err := cmd.Run()

	var exitErr *exec.ExitError

	if !errors.As(err, &exitErr) {
		t.Fatalf("expected subprocess to exit with an error, got %v (stderr: %s)", err, stderr.String())
	}

	if code := exitErr.ExitCode(); code != 1 {
		t.Errorf("expected exit code 1 (Fatal), got %d (stderr: %s)", code, stderr.String())
	}

	// The Flush ran (calls recorded), and its error reached the handler -
	// all before the exit.
	if !strings.Contains(stderr.String(), "FLUSHED-MARKER calls=[flush:FatalFlush] err=flush boom") {
		t.Errorf("Fatal did not flush before exit, stderr: %s", stderr.String())
	}
}

// Flush/Close during concurrent logging must be race-clean.
func TestFlushClose_ConcurrentWithLogging(t *testing.T) {
	log := &callLog{}

	_, o := output.SafeBuffer(level.Trace)
	o.SetName("Concurrent")

	l := sypl.New("flushclose-race", &flushCloseOutput{IOutput: o, log: log})

	const goroutines = 8

	var wg sync.WaitGroup

	wg.Add(goroutines * 2)

	for range goroutines {
		go func() {
			defer wg.Done()

			for range 25 {
				l.Println(level.Info, "concurrent logging")
			}
		}()

		go func() {
			defer wg.Done()

			for range 25 {
				_ = l.Flush()
				_ = l.Close()
			}
		}()
	}

	wg.Wait()
}

// equalStrings compares two string slices element-wise.
func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}
