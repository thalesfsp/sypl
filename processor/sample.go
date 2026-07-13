// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package processor

import (
	"sync"
	"time"

	"github.com/thalesfsp/sypl/flag"
	"github.com/thalesfsp/sypl/message"
)

// sampleMaxKeys bounds the sampler's internal key map, protecting against
// unbounded-key growth (e.g.: highly dynamic content used as key). See
// `sampler.run` for the eviction strategy.
const sampleMaxKeys = 4096

// SampleConfig configures the `Sample` processor.
type SampleConfig struct {
	// First is the number of messages, per key, that always pass within a
	// window.
	First uint64

	// Thereafter, after the first `First` messages, only every
	// `Thereafter`-th message passes - the rest is muted. `0` drops all
	// messages after `First`.
	Thereafter uint64

	// Window is the sampling window. Counters reset when it elapses. `<= 0`
	// means a single infinite window - counters never reset.
	Window time.Duration

	// KeyFn derives the sampling key from a message. Defaults to level +
	// processed content.
	KeyFn func(m message.IMessage) string

	// now is the injectable clock. Defaults to `time.Now`. Tests use it to
	// make time-dependent logic deterministic.
	now func() time.Time
}

// sampleEntry is the per-key sampling state.
type sampleEntry struct {
	// count of messages observed in the current window.
	count uint64

	// windowStart is the point in time the current window started.
	windowStart time.Time
}

// sampler holds the `Sample` processor state.
type sampler struct {
	// cfg is the - defaults applied - configuration.
	cfg SampleConfig

	// mu guards `entries`.
	mu sync.Mutex

	// entries per-key sampling state. Bounded by `sampleMaxKeys`.
	entries map[string]*sampleEntry
}

// run implements the sampling algorithm. It mutes - `flag.Mute` - messages
// filtered out by the sampling decision.
func (s *sampler) run(m message.IMessage) error {
	key := s.cfg.KeyFn(m)

	now := s.cfg.now()

	s.mu.Lock()

	entry, ok := s.entries[key]

	if !ok {
		// Bounds the map. Eviction strategy: evict-all. It's O(1), simple,
		// and its failure mode is safe - evicted keys restart counting, so,
		// at worst, a few extra messages pass. An LRU would add per-hit
		// bookkeeping to the hot logging path for little gain.
		if len(s.entries) >= sampleMaxKeys {
			s.entries = map[string]*sampleEntry{}
		}

		entry = &sampleEntry{windowStart: now}

		s.entries[key] = entry
	}

	// Window elapse resets counters.
	if s.cfg.Window > 0 && now.Sub(entry.windowStart) >= s.cfg.Window {
		entry.count = 0

		entry.windowStart = now
	}

	entry.count++

	n := entry.count

	s.mu.Unlock()

	// The first `First` messages of the window pass...
	if n <= s.cfg.First {
		return nil
	}

	// ...then only every `Thereafter`-th passes.
	if s.cfg.Thereafter > 0 && (n-s.cfg.First)%s.cfg.Thereafter == 0 {
		return nil
	}

	m.SetFlag(flag.Mute)

	return nil
}

// defaultSampleKeyFn is the default sampling key: level + processed content.
func defaultSampleKeyFn(m message.IMessage) string {
	return m.GetLevel().String() + ":" + m.GetContent().GetProcessed()
}

// newSampler is the `sampler` factory. It applies defaults.
func newSampler(cfg SampleConfig) *sampler {
	if cfg.KeyFn == nil {
		cfg.KeyFn = defaultSampleKeyFn
	}

	if cfg.now == nil {
		cfg.now = time.Now
	}

	return &sampler{
		cfg:     cfg,
		entries: map[string]*sampleEntry{},
	}
}

// Sample is a zap-style sampling processor. Per key, within each window, the
// first `First` messages pass; afterwards, only every `Thereafter`-th message
// passes - the rest is muted (`flag.Mute`). See `SampleConfig` for defaults,
// and edge cases.
//
// Notes:
//   - State is per-instance: attach a given instance to exactly ONE output -
//     messages are copied, and processed per output, so a shared instance
//     would count each logical message once per output. Create one instance
//     per output instead.
//   - The internal key map is bounded (`sampleMaxKeys`). On overflow, ALL
//     entries are evicted - counting restarts, so a few extra messages may
//     pass. This is the documented, safe failure mode.
func Sample(cfg SampleConfig) IProcessor {
	return New("Sample", newSampler(cfg).run)
}
