// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package sypl_test

import (
	"errors"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/thalesfsp/sypl/v2"
	"github.com/thalesfsp/sypl/v2/level"
	"github.com/thalesfsp/sypl/v2/message"
	"github.com/thalesfsp/sypl/v2/output"
	"github.com/thalesfsp/sypl/v2/status"
)

// spawnedByProcessOutputs reports whether the captured goroutine stack
// belongs to a goroutine SPAWNED by processOutputs (the fan-out path). An
// inline write runs on the caller's goroutine, whose "created by" ancestor is
// `process` - not `processOutputs`.
func spawnedByProcessOutputs(stack string) bool {
	return strings.Contains(stack, "created by github.com/thalesfsp/sypl/v2.(*Sypl).processOutputs")
}

// stackCapturingOutput wraps a real output, recording the goroutine stack of
// every Write call.
type stackCapturingOutput struct {
	output.IOutput

	mu     sync.Mutex
	stacks []string
}

func (s *stackCapturingOutput) Write(m message.IMessage) error {
	buf := make([]byte, 16384)
	n := runtime.Stack(buf, false)

	s.mu.Lock()
	s.stacks = append(s.stacks, string(buf[:n]))
	s.mu.Unlock()

	return s.IOutput.Write(m)
}

func (s *stackCapturingOutput) snapshot() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]string{}, s.stacks...)
}

// A single receiving output must be written INLINE - on the goroutine
// running processOutputs - not via goroutine+WaitGroup.
func TestInlineWrite_SingleOutputWritesInline(t *testing.T) {
	buf, o := output.SafeBuffer(level.Trace)

	stub := &stackCapturingOutput{IOutput: o}

	l := sypl.New("inline-single", stub)

	l.Println(level.Info, "inline write")

	stacks := stub.snapshot()

	if len(stacks) != 1 {
		t.Fatalf("Write called %d times, want 1", len(stacks))
	}

	if spawnedByProcessOutputs(stacks[0]) {
		t.Fatalf("single-output write was dispatched on a spawned goroutine, want inline:\n%s", stacks[0])
	}

	if !strings.Contains(buf.String(), "inline write") {
		t.Fatalf("inline write lost the message, output %q", buf.String())
	}
}

// Two, or more receiving outputs keep the concurrent fan-out.
func TestInlineWrite_MultiOutputKeepsFanOut(t *testing.T) {
	bufA, oA := namedSafeBuffer("A", level.Trace)

	bufB, oB := namedSafeBuffer("B", level.Trace)

	stubA := &stackCapturingOutput{IOutput: oA}
	stubB := &stackCapturingOutput{IOutput: oB}

	l := sypl.New("inline-multi", stubA, stubB)

	l.Println(level.Info, "fan-out write")

	for name, stub := range map[string]*stackCapturingOutput{"A": stubA, "B": stubB} {
		stacks := stub.snapshot()

		if len(stacks) != 1 {
			t.Fatalf("output %s: Write called %d times, want 1", name, len(stacks))
		}

		if !spawnedByProcessOutputs(stacks[0]) {
			t.Fatalf("output %s: multi-output write ran inline, want spawned fan-out:\n%s", name, stacks[0])
		}
	}

	if !strings.Contains(bufA.String(), "fan-out write") || !strings.Contains(bufB.String(), "fan-out write") {
		t.Fatalf("fan-out lost a message: A=%q B=%q", bufA.String(), bufB.String())
	}
}

// A disabled sibling must not disqualify the inline fast path: only ONE
// output actually receives the message.
func TestInlineWrite_DisabledSiblingStillInline(t *testing.T) {
	_, oDisabled := namedSafeBuffer("Disabled", level.Trace)
	oDisabled.SetStatus(status.Disabled)

	buf, oEnabled := namedSafeBuffer("Enabled", level.Trace)

	stub := &stackCapturingOutput{IOutput: oEnabled}

	l := sypl.New("inline-disabled-sibling", oDisabled, stub)

	l.Println(level.Info, "still inline")

	stacks := stub.snapshot()

	if len(stacks) != 1 {
		t.Fatalf("Write called %d times, want 1", len(stacks))
	}

	if spawnedByProcessOutputs(stacks[0]) {
		t.Fatalf("single effective output was not written inline:\n%s", stacks[0])
	}

	if !strings.Contains(buf.String(), "still inline") {
		t.Fatalf("message lost, output %q", buf.String())
	}
}

// failingOutput always errors on Write.
type failingOutput struct {
	output.IOutput
}

func (f *failingOutput) Write(_ message.IMessage) error {
	return errors.New("boom")
}

// Inline writes must preserve the historical error-swallow behavior: a
// failing output neither panics, nor stops the caller.
func TestInlineWrite_ErrorSwallowPreserved(t *testing.T) {
	_, o := output.SafeBuffer(level.Trace)

	l := sypl.New("inline-swallow", &failingOutput{IOutput: o})

	// Must not panic; chaining must survive.
	l.Println(level.Info, "swallowed").Println(level.Info, "still alive")
}
