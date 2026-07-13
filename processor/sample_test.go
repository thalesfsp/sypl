// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package processor

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/thalesfsp/sypl/v2/flag"
	"github.com/thalesfsp/sypl/v2/level"
	"github.com/thalesfsp/sypl/v2/message"
)

//////
// Test helpers.
//////

// fakeClock is a manually-advanced clock for deterministic time-dependent
// tests - no sleep-based assertions.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

// newFakeClock is the `fakeClock` factory.
func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Unix(0, 0)}
}

// Now returns the current fake time.
func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.now
}

// Advance moves the fake time forward by `d`.
func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.now = c.now.Add(d)
}

// mutedBy runs `p` against a fresh message with the given `content` @ the
// Info level, returning `true` if the processor muted it.
func mutedBy(t *testing.T, p IProcessor, content string) bool {
	t.Helper()

	return runProcessor(t, p, level.Info, content).GetFlag() == flag.Mute
}

// assertPattern runs `p` against `len(expected)` identical messages, asserting
// the mute decision of each one.
func assertPattern(t *testing.T, p IProcessor, content string, expected []bool) {
	t.Helper()

	for i, expectedMuted := range expected {
		if muted := mutedBy(t, p, content); muted != expectedMuted {
			t.Fatalf("message %d: muted = %v, expected %v", i+1, muted, expectedMuted)
		}
	}
}

//////
// Sample tests.
//////

// First `First` pass, then only every `Thereafter`-th passes.
func TestSample_FirstNThenEveryMth(t *testing.T) {
	p := Sample(SampleConfig{First: 2, Thereafter: 3})

	// n:                 1      2      3     4     5      6     7     8
	assertPattern(t, p, "repeated", []bool{
		false, false, true, true, false, true, true, false, true, true,
	})
}

// Window elapse resets counters.
func TestSample_WindowResetsCounters(t *testing.T) {
	clock := newFakeClock()

	p := Sample(SampleConfig{
		First:  1,
		Window: time.Second,
		now:    clock.Now,
	})

	if mutedBy(t, p, "w") {
		t.Fatal("first message must pass")
	}

	// Negative control: window has NOT elapsed yet.
	clock.Advance(400 * time.Millisecond)

	if !mutedBy(t, p, "w") {
		t.Fatal("second message within the window must be muted")
	}

	// Window elapsed - counters reset, message passes again.
	clock.Advance(600 * time.Millisecond)

	if mutedBy(t, p, "w") {
		t.Fatal("first message of the new window must pass")
	}

	if !mutedBy(t, p, "w") {
		t.Fatal("second message of the new window must be muted")
	}
}

// Each key is sampled independently. The default key is level + processed
// content.
func TestSample_PerKeyIndependence(t *testing.T) {
	p := Sample(SampleConfig{First: 1})

	if mutedBy(t, p, "a") {
		t.Fatal("first 'a' must pass")
	}

	if mutedBy(t, p, "b") {
		t.Fatal("first 'b' must pass - keys are independent")
	}

	if !mutedBy(t, p, "a") {
		t.Fatal("second 'a' must be muted")
	}

	if !mutedBy(t, p, "b") {
		t.Fatal("second 'b' must be muted")
	}

	// Same content @ a different level is a different (default) key.
	if runProcessor(t, p, level.Warn, "a").GetFlag() == flag.Mute {
		t.Fatal("first Warn 'a' must pass - level is part of the default key")
	}
}

// Thereafter = 0 drops everything after the first `First` messages - even
// after (fake) time passes, given `Window` <= 0 is a single infinite window.
func TestSample_ThereafterZeroInfiniteWindow(t *testing.T) {
	clock := newFakeClock()

	p := Sample(SampleConfig{First: 2, now: clock.Now})

	assertPattern(t, p, "t0", []bool{false, false, true, true, true})

	// Window <= 0: time passing never resets the counters.
	clock.Advance(24 * time.Hour)

	if !mutedBy(t, p, "t0") {
		t.Fatal("message must stay muted - window <= 0 never resets")
	}
}

// A custom `KeyFn` overrides the default key derivation.
func TestSample_CustomKeyFn(t *testing.T) {
	p := Sample(SampleConfig{
		First: 1,
		KeyFn: func(m message.IMessage) string { return "constant" },
	})

	if mutedBy(t, p, "x") {
		t.Fatal("first message must pass")
	}

	if !mutedBy(t, p, "y") {
		t.Fatal("different content must share the constant key, and be muted")
	}
}

// First = 0, and Thereafter = 0 mutes everything.
func TestSample_ZeroFirstZeroThereafterMutesAll(t *testing.T) {
	p := Sample(SampleConfig{})

	assertPattern(t, p, "z", []bool{true, true, true})
}

// An unbounded-key attack must not grow the internal map beyond the cap.
func TestSample_BoundedKeysAttack(t *testing.T) {
	s := newSampler(SampleConfig{First: 1})

	p := New("Sample", s.run)

	// Exhaust "k0".
	if mutedBy(t, p, "k0") {
		t.Fatal("first 'k0' must pass")
	}

	// Negative control: before the eviction, "k0" is muted.
	if !mutedBy(t, p, "k0") {
		t.Fatal("second 'k0' must be muted")
	}

	// Attack: flood distinct keys up to the cap.
	for i := 1; i < sampleMaxKeys; i++ {
		mutedBy(t, p, fmt.Sprintf("k%d", i))
	}

	s.mu.Lock()
	entries := len(s.entries)
	s.mu.Unlock()

	if entries > sampleMaxKeys {
		t.Fatalf("map grew to %d entries, cap is %d", entries, sampleMaxKeys)
	}

	// One more NEW key overflows the cap, triggering the evict-all.
	if mutedBy(t, p, "overflow") {
		t.Fatal("first 'overflow' must pass")
	}

	s.mu.Lock()
	entries = len(s.entries)
	s.mu.Unlock()

	if entries != 1 {
		t.Fatalf("map has %d entries after the evict-all, expected 1", entries)
	}

	// "k0" was evicted with everything else: it passes again - the documented
	// bounded-memory trade-off.
	if mutedBy(t, p, "k0") {
		t.Fatal("'k0' must pass after the evict-all")
	}
}

// Concurrent messages are race-clean, and the sampled count is exact.
func TestSample_Concurrent(t *testing.T) {
	const (
		goroutines         = 8
		messagesPerRoutine = 50
		first              = 5
	)

	p := Sample(SampleConfig{First: first})

	var passed atomic.Uint64

	var wg sync.WaitGroup

	wg.Add(goroutines)

	for range goroutines {
		go func() {
			defer wg.Done()

			for range messagesPerRoutine {
				m := message.New(level.Info, "concurrent")

				if err := p.Run(m); err != nil {
					t.Errorf("Run failed: %s", err)

					return
				}

				if m.GetFlag() != flag.Mute {
					passed.Add(1)
				}
			}
		}()
	}

	wg.Wait()

	if passed.Load() != first {
		t.Fatalf("%d messages passed, expected exactly %d", passed.Load(), first)
	}
}
