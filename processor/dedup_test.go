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

	"github.com/thalesfsp/sypl/flag"
	"github.com/thalesfsp/sypl/level"
	"github.com/thalesfsp/sypl/message"
)

//////
// Dedup tests.
//////

// Duplicates within the window are muted - distinct messages pass.
func TestDedup_DuplicateMutedDistinctPass(t *testing.T) {
	p := Dedup(time.Minute)

	if mutedBy(t, p, "a") {
		t.Fatal("first 'a' must pass")
	}

	if !mutedBy(t, p, "a") {
		t.Fatal("duplicate 'a' must be muted")
	}

	if mutedBy(t, p, "b") {
		t.Fatal("distinct 'b' must pass")
	}
}

// Window expiry re-allows the key, and the counter callback reports the
// number of suppressed messages.
func TestDedup_WindowExpiryReallowsAndReportsCount(t *testing.T) {
	clock := newFakeClock()

	type report struct {
		key        string
		suppressed uint64
	}

	reports := []report{}

	p := Dedup(
		time.Second,
		DedupWithCounter(func(key string, suppressed uint64) {
			reports = append(reports, report{key: key, suppressed: suppressed})
		}),
		dedupWithClock(clock.Now),
	)

	// First passes, two duplicates suppressed.
	assertPattern(t, p, "a", []bool{false, true, true})

	// Negative control: still within the window.
	clock.Advance(500 * time.Millisecond)

	if !mutedBy(t, p, "a") {
		t.Fatal("duplicate within the window must stay muted")
	}

	// Window elapsed: the key logs again, and the counter reports.
	clock.Advance(500 * time.Millisecond)

	if mutedBy(t, p, "a") {
		t.Fatal("'a' must pass after the window elapsed")
	}

	expectedKey := message.New(level.Info, "a").GetContentBasedHashID()

	if len(reports) != 1 || reports[0].key != expectedKey || reports[0].suppressed != 3 {
		t.Fatalf("reports = %+v, expected [{%s 3}]", reports, expectedKey)
	}
}

// Negative control: a key re-passing without suppressed duplicates must NOT
// fire the counter callback.
func TestDedup_CounterNotCalledWithoutSuppressions(t *testing.T) {
	clock := newFakeClock()

	var calls atomic.Uint64

	p := Dedup(
		time.Second,
		DedupWithCounter(func(key string, suppressed uint64) { calls.Add(1) }),
		dedupWithClock(clock.Now),
	)

	if mutedBy(t, p, "a") {
		t.Fatal("first 'a' must pass")
	}

	clock.Advance(time.Second)

	if mutedBy(t, p, "a") {
		t.Fatal("'a' must pass after the window elapsed")
	}

	if calls.Load() != 0 {
		t.Fatalf("counter fired %d times, expected 0", calls.Load())
	}
}

// A custom key function overrides the default content-based hash key.
func TestDedup_CustomKeyFn(t *testing.T) {
	p := Dedup(
		time.Minute,
		DedupWithKeyFn(func(m message.IMessage) string { return m.GetLevel().String() }),
	)

	if mutedBy(t, p, "x") {
		t.Fatal("first Info message must pass")
	}

	if !mutedBy(t, p, "y") {
		t.Fatal("second Info message must be muted - same key (level)")
	}

	if runProcessor(t, p, level.Warn, "z").GetFlag() == flag.Mute {
		t.Fatal("first Warn message must pass - different key (level)")
	}
}

// window <= 0 is a single infinite window: duplicates are muted forever.
func TestDedup_InfiniteWindow(t *testing.T) {
	clock := newFakeClock()

	p := Dedup(0, dedupWithClock(clock.Now))

	if mutedBy(t, p, "a") {
		t.Fatal("first 'a' must pass")
	}

	clock.Advance(24 * time.Hour)

	if !mutedBy(t, p, "a") {
		t.Fatal("duplicate must stay muted - window <= 0 never expires")
	}
}

// An unbounded-key attack must not grow the internal map beyond the cap.
func TestDedup_BoundEnforcement(t *testing.T) {
	d := newDeduper(time.Minute)

	p := New("Dedup", d.run)

	if mutedBy(t, p, "k0") {
		t.Fatal("first 'k0' must pass")
	}

	// Negative control: before the eviction, "k0" is muted.
	if !mutedBy(t, p, "k0") {
		t.Fatal("duplicate 'k0' must be muted")
	}

	// Attack: flood distinct keys up to the cap.
	for i := 1; i < dedupMaxKeys; i++ {
		mutedBy(t, p, fmt.Sprintf("k%d", i))
	}

	d.mu.Lock()
	entries := len(d.entries)
	d.mu.Unlock()

	if entries > dedupMaxKeys {
		t.Fatalf("map grew to %d entries, cap is %d", entries, dedupMaxKeys)
	}

	// One more NEW key overflows the cap, triggering the evict-all.
	if mutedBy(t, p, "overflow") {
		t.Fatal("first 'overflow' must pass")
	}

	d.mu.Lock()
	entries = len(d.entries)
	d.mu.Unlock()

	if entries != 1 {
		t.Fatalf("map has %d entries after the evict-all, expected 1", entries)
	}

	// "k0" was evicted with everything else: it passes again - the documented
	// bounded-memory trade-off.
	if mutedBy(t, p, "k0") {
		t.Fatal("'k0' must pass after the evict-all")
	}
}

// Concurrent duplicates are race-clean: exactly one passes.
func TestDedup_Concurrent(t *testing.T) {
	const (
		goroutines         = 8
		messagesPerRoutine = 50
	)

	p := Dedup(time.Minute)

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

	if passed.Load() != 1 {
		t.Fatalf("%d messages passed, expected exactly 1", passed.Load())
	}
}
