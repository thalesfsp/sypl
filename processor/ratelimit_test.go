// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package processor

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/thalesfsp/sypl/v2/flag"
	"github.com/thalesfsp/sypl/v2/level"
	"github.com/thalesfsp/sypl/v2/message"
)

//////
// RateLimit tests.
//////

// Exactly `maxPerWindow` messages pass - the boundary is exact.
func TestRateLimit_ExactLimitBoundary(t *testing.T) {
	clock := newFakeClock()

	p := RateLimit(3, time.Second, rateLimitWithClock(clock.Now))

	assertPattern(t, p, "rl", []bool{false, false, false, true, true})
}

// Window rollover resets the counters, and fires the callback with the exact
// number of muted messages.
func TestRateLimit_RolloverResetsAndCallbackFires(t *testing.T) {
	clock := newFakeClock()

	calls := []uint64{}

	p := RateLimit(
		1,
		time.Second,
		RateLimitWithCallback(func(dropped uint64) { calls = append(calls, dropped) }),
		rateLimitWithClock(clock.Now),
	)

	// Window 1: one passes, two muted.
	assertPattern(t, p, "rl", []bool{false, true, true})

	// Rollover: callback reports the two muted, and counting restarts.
	clock.Advance(time.Second)

	assertPattern(t, p, "rl", []bool{false, true})

	// Second rollover: reports the single muted message.
	clock.Advance(time.Second)

	assertPattern(t, p, "rl", []bool{false})

	if len(calls) != 2 || calls[0] != 2 || calls[1] != 1 {
		t.Fatalf("callback calls = %v, expected [2 1]", calls)
	}
}

// Negative control: rollover without any muted message must NOT fire the
// callback.
func TestRateLimit_RolloverWithoutDropsNoCallback(t *testing.T) {
	clock := newFakeClock()

	var calls atomic.Uint64

	p := RateLimit(
		5,
		time.Second,
		RateLimitWithCallback(func(dropped uint64) { calls.Add(1) }),
		rateLimitWithClock(clock.Now),
	)

	assertPattern(t, p, "rl", []bool{false, false})

	clock.Advance(time.Second)

	assertPattern(t, p, "rl", []bool{false})

	if calls.Load() != 0 {
		t.Fatalf("callback fired %d times, expected 0", calls.Load())
	}
}

// A rollover with muted messages, and no callback set must not panic.
func TestRateLimit_NilCallbackWithDrops(t *testing.T) {
	clock := newFakeClock()

	p := RateLimit(1, time.Second, rateLimitWithClock(clock.Now))

	assertPattern(t, p, "rl", []bool{false, true})

	clock.Advance(time.Second)

	assertPattern(t, p, "rl", []bool{false})
}

// The callback runs outside the limiter's lock: re-entering the processor
// from within the callback - e.g.: logging the drop notice through the same
// pipeline - must not deadlock.
func TestRateLimit_CallbackReentrancy(t *testing.T) {
	clock := newFakeClock()

	var p IProcessor

	var reentered atomic.Uint64

	p = RateLimit(
		1,
		time.Second,
		RateLimitWithCallback(func(dropped uint64) {
			reentered.Add(1)

			mutedBy(t, p, "from-callback")
		}),
		rateLimitWithClock(clock.Now),
	)

	assertPattern(t, p, "rl", []bool{false, true})

	clock.Advance(time.Second)

	assertPattern(t, p, "rl", []bool{false})

	if reentered.Load() != 1 {
		t.Fatalf("callback re-entered %d times, expected 1", reentered.Load())
	}
}

// maxPerWindow = 0 mutes everything.
func TestRateLimit_ZeroMaxMutesEverything(t *testing.T) {
	p := RateLimit(0, time.Second)

	assertPattern(t, p, "rl", []bool{true, true, true})
}

// window <= 0 is a single infinite window: only the first `maxPerWindow`
// messages EVER pass.
func TestRateLimit_ZeroWindowIsInfinite(t *testing.T) {
	clock := newFakeClock()

	p := RateLimit(2, 0, rateLimitWithClock(clock.Now))

	assertPattern(t, p, "rl", []bool{false, false, true})

	// Time passing never resets the counters.
	clock.Advance(24 * time.Hour)

	if !mutedBy(t, p, "rl") {
		t.Fatal("message must stay muted - window <= 0 never rolls over")
	}
}

// Concurrent messages are race-clean, and the limit is exact.
func TestRateLimit_Concurrent(t *testing.T) {
	const (
		goroutines         = 8
		messagesPerRoutine = 50
		maxPerWindow       = 100
	)

	p := RateLimit(maxPerWindow, time.Minute)

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

	if passed.Load() != maxPerWindow {
		t.Fatalf("%d messages passed, expected exactly %d", passed.Load(), maxPerWindow)
	}
}
