// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package processor

import (
	"sync"
	"time"

	"github.com/thalesfsp/sypl/v2/flag"
	"github.com/thalesfsp/sypl/v2/message"
)

// rateLimitConfig is the `RateLimit` optional configuration.
type rateLimitConfig struct {
	// callback is invoked at window rollover with the number of messages
	// muted in the elapsed window - only if at least one was muted.
	callback func(dropped uint64)

	// now is the injectable clock. Defaults to `time.Now`.
	now func() time.Time
}

// RateLimitOption allows to specify optional `RateLimit` configuration.
type RateLimitOption func(*rateLimitConfig)

// RateLimitWithCallback sets an overflow callback, invoked at window rollover
// with the number of messages muted in the elapsed window. It's only invoked
// if at least one message was muted.
//
// Notes:
//   - The callback runs synchronously, on the logging goroutine that
//     triggered the rollover - keep it fast.
//   - It runs outside the limiter's lock, so it may safely log - even through
//     the same pipeline.
//   - Messages muted in the FINAL window are never reported - reporting only
//     happens at rollover.
func RateLimitWithCallback(callback func(dropped uint64)) RateLimitOption {
	return func(cfg *rateLimitConfig) {
		cfg.callback = callback
	}
}

// rateLimitWithClock sets the clock. Tests use it to make time-dependent
// logic deterministic.
func rateLimitWithClock(now func() time.Time) RateLimitOption {
	return func(cfg *rateLimitConfig) {
		cfg.now = now
	}
}

// rateLimiter holds the `RateLimit` processor state.
type rateLimiter struct {
	// cfg is the - defaults applied - optional configuration.
	cfg rateLimitConfig

	// maxPerWindow is the number of messages allowed per window.
	maxPerWindow uint64

	// window is the limiting window. `<= 0` means a single infinite window.
	window time.Duration

	// mu guards the mutable state below.
	mu sync.Mutex

	// count of messages observed in the current window.
	count uint64

	// dropped counts messages muted in the current window.
	dropped uint64

	// started indicates whether the first window has been opened.
	started bool

	// windowStart is the point in time the current window started.
	windowStart time.Time
}

// run implements the limiting algorithm. It mutes - `flag.Mute` - messages
// beyond `maxPerWindow`, within each window.
func (r *rateLimiter) run(m message.IMessage) error {
	now := r.cfg.now()

	r.mu.Lock()

	// The first window opens at the first message.
	if !r.started {
		r.windowStart = now

		r.started = true
	}

	// Window rollover: snapshot the muted count for reporting, and reset.
	var report uint64

	if r.window > 0 && now.Sub(r.windowStart) >= r.window {
		report = r.dropped

		r.count = 0

		r.dropped = 0

		r.windowStart = now
	}

	r.count++

	muted := r.count > r.maxPerWindow

	if muted {
		r.dropped++
	}

	r.mu.Unlock()

	// Outside the lock, so the callback may re-enter the processor - e.g.:
	// logging the drop notice through the same pipeline - without
	// deadlocking.
	if report > 0 && r.cfg.callback != nil {
		r.cfg.callback(report)
	}

	if muted {
		m.SetFlag(flag.Mute)
	}

	return nil
}

// newRateLimiter is the `rateLimiter` factory. It applies defaults.
func newRateLimiter(
	maxPerWindow uint64,
	window time.Duration,
	opts ...RateLimitOption,
) *rateLimiter {
	cfg := rateLimitConfig{now: time.Now}

	for _, opt := range opts {
		opt(&cfg)
	}

	return &rateLimiter{
		cfg:          cfg,
		maxPerWindow: maxPerWindow,
		window:       window,
	}
}

// RateLimit is a global - NOT per-key - token-window limiting processor.
// Within each window, the first `maxPerWindow` messages pass - the rest is
// muted (`flag.Mute`). When the window elapses, counting restarts.
//
// Notes:
//   - `window <= 0` means a single infinite window: only the first
//     `maxPerWindow` messages EVER pass.
//   - `maxPerWindow = 0` mutes everything.
//   - State is per-instance: attach a given instance to exactly ONE output -
//     messages are copied, and processed per output, so a shared instance
//     would count each logical message once per output. Create one instance
//     per output instead.
//   - See `RateLimitWithCallback` for muted-message reporting.
func RateLimit(maxPerWindow uint64, window time.Duration, opts ...RateLimitOption) IProcessor {
	return New("RateLimit", newRateLimiter(maxPerWindow, window, opts...).run)
}
