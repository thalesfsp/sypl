// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package safebuffer

import (
	"strings"
	"sync"
	"testing"
)

// Zero-value Buffer must be immediately usable.
func TestBuffer_ZeroValue(t *testing.T) {
	var buf Buffer

	if got := buf.String(); got != "" {
		t.Errorf("zero-value String() = %q, want empty", got)
	}

	n, err := buf.Write([]byte{})
	if err != nil {
		t.Errorf("Write(empty) error = %v, want nil", err)
	}

	if n != 0 {
		t.Errorf("Write(empty) n = %d, want 0", n)
	}

	// Reset on an empty buffer must not panic.
	buf.Reset()

	if got := buf.String(); got != "" {
		t.Errorf("String() after Reset = %q, want empty", got)
	}
}

// Writes-only concurrency: with no Reset in flight, every byte written must
// be accounted for - nothing lost, nothing duplicated, no interleaving inside
// a single Write.
func TestBuffer_ConcurrentWrites_ByteAccounting(t *testing.T) {
	const (
		goroutines       = 16
		writesPerRoutine = 50
		payload          = "payload|"
	)

	var buf Buffer

	var wg sync.WaitGroup

	for range goroutines {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for range writesPerRoutine {
				n, err := buf.Write([]byte(payload))
				if err != nil {
					t.Errorf("Write() error = %v, want nil", err)

					return
				}

				if n != len(payload) {
					t.Errorf("Write() n = %d, want %d", n, len(payload))

					return
				}
			}
		}()
	}

	wg.Wait()

	got := buf.String()

	wantLen := goroutines * writesPerRoutine * len(payload)

	if len(got) != wantLen {
		t.Errorf("total bytes = %d, want %d", len(got), wantLen)
	}

	wantCount := goroutines * writesPerRoutine

	if count := strings.Count(got, payload); count != wantCount {
		t.Errorf("payload occurrences = %d, want %d (payloads interleaved or lost)", count, wantCount)
	}
}

// Mixed concurrency: Write, String, and Reset racing. Run under -race this
// asserts the mutex actually guards every operation; the invariant checked
// at the end is that content is always a whole number of payloads (Reset,
// and Write are atomic - no torn writes).
func TestBuffer_ConcurrentWriteStringReset(t *testing.T) {
	const (
		goroutines = 8
		iterations = 100
		payload    = "payload|"
	)

	var buf Buffer

	var wg sync.WaitGroup

	for g := range goroutines {
		wg.Add(1)

		go func(id int) {
			defer wg.Done()

			for i := range iterations {
				switch (id + i) % 3 {
				case 0:
					//nolint:errcheck // Accounting asserted in the writes-only test.
					buf.Write([]byte(payload))
				case 1:
					s := buf.String()

					// Any snapshot must be a whole number of payloads.
					if len(s)%len(payload) != 0 {
						t.Errorf("torn snapshot: len %d not multiple of %d", len(s), len(payload))

						return
					}
				case 2:
					buf.Reset()
				}
			}
		}(g)
	}

	wg.Wait()

	final := buf.String()

	if len(final)%len(payload) != 0 {
		t.Errorf("final content torn: len %d not multiple of %d", len(final), len(payload))
	}

	if count := strings.Count(final, payload); count*len(payload) != len(final) {
		t.Errorf("final content contains non-payload bytes: %q", final)
	}
}
