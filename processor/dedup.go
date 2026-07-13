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

// dedupMaxKeys bounds the deduper's internal key map, protecting against
// unbounded-key growth (e.g.: highly dynamic content used as key). See
// `deduper.run` for the eviction strategy.
const dedupMaxKeys = 4096

// dedupConfig is the `Dedup` optional configuration.
type dedupConfig struct {
	// counter is invoked when a key re-passes after the window, with the
	// number of messages suppressed meanwhile - only if at least one was.
	counter func(key string, suppressed uint64)

	// keyFn derives the deduplication key from a message. Defaults to the
	// message's content-based hash ID.
	keyFn func(m message.IMessage) string

	// now is the injectable clock. Defaults to `time.Now`.
	now func() time.Time
}

// DedupOption allows to specify optional `Dedup` configuration.
type DedupOption func(*dedupConfig)

// DedupWithKeyFn sets the deduplication key function. Defaults to the
// message's content-based hash ID - `GetContentBasedHashID` - which is
// level-agnostic. Use a custom key function to, for example, include the
// level.
func DedupWithKeyFn(keyFn func(m message.IMessage) string) DedupOption {
	return func(cfg *dedupConfig) {
		cfg.keyFn = keyFn
	}
}

// DedupWithCounter sets a suppression-count callback, invoked when a key
// re-passes after the window, with the number of messages suppressed
// meanwhile. It's only invoked if at least one message was suppressed.
//
// Notes:
//   - The callback runs synchronously, on the logging goroutine that
//     re-passed the key - keep it fast.
//   - It runs outside the deduper's lock, so it may safely log - even through
//     the same pipeline.
//   - Suppressions of a key that never re-passes are never reported.
func DedupWithCounter(counter func(key string, suppressed uint64)) DedupOption {
	return func(cfg *dedupConfig) {
		cfg.counter = counter
	}
}

// dedupWithClock sets the clock. Tests use it to make time-dependent logic
// deterministic.
func dedupWithClock(now func() time.Time) DedupOption {
	return func(cfg *dedupConfig) {
		cfg.now = now
	}
}

// dedupEntry is the per-key deduplication state.
type dedupEntry struct {
	// lastLogged is the point in time the key last passed.
	lastLogged time.Time

	// suppressed counts messages muted since `lastLogged`.
	suppressed uint64
}

// deduper holds the `Dedup` processor state.
type deduper struct {
	// cfg is the - defaults applied - optional configuration.
	cfg dedupConfig

	// window is the deduplication window. `<= 0` means a single infinite
	// window.
	window time.Duration

	// mu guards `entries`.
	mu sync.Mutex

	// entries per-key deduplication state. Bounded by `dedupMaxKeys`.
	entries map[string]*dedupEntry
}

// run implements the deduplication algorithm. It mutes - `flag.Mute` -
// messages whose key already logged within the window.
func (d *deduper) run(m message.IMessage) error {
	key := d.cfg.keyFn(m)

	now := d.cfg.now()

	var muted bool

	var report uint64

	d.mu.Lock()

	entry, ok := d.entries[key]

	switch {
	case !ok:
		// Bounds the map. Eviction strategy: evict-all. It's O(1), simple,
		// and its failure mode is safe - evicted keys log again, so, at
		// worst, a few extra messages pass. An LRU would add per-hit
		// bookkeeping to the hot logging path for little gain.
		if len(d.entries) >= dedupMaxKeys {
			d.entries = map[string]*dedupEntry{}
		}

		d.entries[key] = &dedupEntry{lastLogged: now}
	case d.window > 0 && now.Sub(entry.lastLogged) >= d.window:
		// Window elapsed: the key logs again. Snapshot the suppressed count
		// for reporting, and reset.
		report = entry.suppressed

		entry.suppressed = 0

		entry.lastLogged = now
	default:
		entry.suppressed++

		muted = true
	}

	d.mu.Unlock()

	// Outside the lock, so the callback may re-enter the processor - e.g.:
	// logging the suppression notice through the same pipeline - without
	// deadlocking.
	if report > 0 && d.cfg.counter != nil {
		d.cfg.counter(key, report)
	}

	if muted {
		m.SetFlag(flag.Mute)
	}

	return nil
}

// defaultDedupKeyFn is the default deduplication key: the message's
// content-based hash ID.
func defaultDedupKeyFn(m message.IMessage) string {
	return m.GetContentBasedHashID()
}

// newDeduper is the `deduper` factory. It applies defaults.
func newDeduper(window time.Duration, opts ...DedupOption) *deduper {
	cfg := dedupConfig{
		keyFn: defaultDedupKeyFn,
		now:   time.Now,
	}

	for _, opt := range opts {
		opt(&cfg)
	}

	return &deduper{
		cfg:     cfg,
		window:  window,
		entries: map[string]*dedupEntry{},
	}
}

// Dedup is a "log once per window" processor: it mutes - `flag.Mute` -
// messages whose key was already logged within the window. When the window
// elapses, the key logs again.
//
// Notes:
//   - `window <= 0` means a single infinite window: duplicates are muted
//     forever.
//   - The default key is the message's content-based hash ID. See
//     `DedupWithKeyFn`, and `DedupWithCounter` for options.
//   - State is per-instance: attach a given instance to exactly ONE output -
//     messages are copied, and processed per output, so a shared instance
//     would count each logical message once per output. Create one instance
//     per output instead.
//   - The internal key map is bounded (`dedupMaxKeys`). On overflow, ALL
//     entries are evicted - evicted keys log again, so a few extra messages
//     may pass. This is the documented, safe failure mode.
func Dedup(window time.Duration, opts ...DedupOption) IProcessor {
	return New("Dedup", newDeduper(window, opts...).run)
}
