// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package builtin

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/thalesfsp/sypl/v2/safebuffer"
)

// The contract this package exists for: the message is written exactly
// as given - NO forced newline, no padding.
func TestOutputBuiltin_NoForcedNewline(t *testing.T) {
	var b strings.Builder

	l := NewBuiltin(&b)

	if err := l.OutputBuiltin("no newline"); err != nil {
		t.Fatal(err)
	}

	if got := b.String(); got != "no newline" {
		t.Errorf("expected %q got %q", "no newline", got)
	}
}

// SetOutput redirects subsequent writes - the sanctioned way to point an
// existing output somewhere else (v2 removed IOutput.SetBuiltinLogger).
func TestSetOutput_Redirects(t *testing.T) {
	var first, second strings.Builder

	l := NewBuiltin(&first)

	if err := l.OutputBuiltin("a"); err != nil {
		t.Fatal(err)
	}

	l.SetOutput(&second)

	if err := l.OutputBuiltin("b"); err != nil {
		t.Fatal(err)
	}

	if first.String() != "a" || second.String() != "b" {
		t.Errorf(`expected "a"/"b", got %q/%q`, first.String(), second.String())
	}
}

// errWriter always fails.
type errWriter struct{ err error }

func (w errWriter) Write([]byte) (int, error) { return 0, w.err }

// Write errors must surface to the caller.
func TestOutputBuiltin_PropagatesWriteError(t *testing.T) {
	sentinel := errors.New("sink failed")

	l := NewBuiltin(errWriter{err: sentinel})

	if err := l.OutputBuiltin("x"); !errors.Is(err, sentinel) {
		t.Errorf("expected the sink error, got %v", err)
	}
}

// Concurrent writers must serialize on the mutex - run with -race. Every
// message arrives whole: the reused internal buffer never interleaves.
func TestOutputBuiltin_ConcurrentWritesSerialize(t *testing.T) {
	var buf safebuffer.Buffer

	l := NewBuiltin(&buf)

	var wg sync.WaitGroup

	for i := 0; i < 8; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for range 50 {
				if err := l.OutputBuiltin("0123456789\n"); err != nil {
					t.Error(err)
				}
			}
		}()
	}

	wg.Wait()

	lines := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")

	if len(lines) != 8*50 {
		t.Fatalf("expected %d lines, got %d", 8*50, len(lines))
	}

	for _, line := range lines {
		if line != "0123456789" {
			t.Fatalf("interleaved write detected: %q", line)
		}
	}
}
