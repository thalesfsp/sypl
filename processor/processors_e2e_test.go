// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

// End-to-end mocked pipeline tests for the flow-control processors:
// logger -> processor (Sample | RateLimit | Dedup) -> output writer.
package processor_test

import (
	"strings"
	"testing"
	"time"

	"github.com/thalesfsp/sypl/v2"
	"github.com/thalesfsp/sypl/v2/level"
	"github.com/thalesfsp/sypl/v2/output"
	"github.com/thalesfsp/sypl/v2/processor"
)

// countLines returns the number of non-empty lines in `s`.
func countLines(s string) int {
	if s == "" {
		return 0
	}

	return len(strings.Split(strings.TrimSuffix(s, "\n"), "\n"))
}

// The full pipeline emits EXACTLY the sampled counts: with First = 2, and
// Thereafter = 3, 10 identical messages produce 4 lines (n = 1, 2, 5, 8).
func TestE2E_SampleExactCounts(t *testing.T) {
	const total = 10

	buf, o := output.SafeBuffer(
		level.Trace,
		processor.Sample(processor.SampleConfig{First: 2, Thereafter: 3}),
	)

	l := sypl.New("sample-e2e", o)

	for range total {
		l.Println(level.Info, "repeated")
	}

	if lines := countLines(buf.String()); lines != 4 {
		t.Fatalf("got %d lines, expected 4: %q", lines, buf.String())
	}

	// Negative control: without the sampler, all messages are printed.
	buf2, o2 := output.SafeBuffer(level.Trace)

	l2 := sypl.New("sample-e2e-control", o2)

	for range total {
		l2.Println(level.Info, "repeated")
	}

	if lines := countLines(buf2.String()); lines != total {
		t.Fatalf("control got %d lines, expected %d", lines, total)
	}
}

// Sampling is per key: alternating two distinct messages, each is sampled
// independently.
func TestE2E_SamplePerKey(t *testing.T) {
	buf, o := output.SafeBuffer(
		level.Trace,
		processor.Sample(processor.SampleConfig{First: 1}),
	)

	l := sypl.New("sample-e2e-keys", o)

	l.Println(level.Info, "a")
	l.Println(level.Info, "b")
	l.Println(level.Info, "a")
	l.Println(level.Info, "b")

	got := buf.String()

	if lines := countLines(got); lines != 2 {
		t.Fatalf("got %d lines, expected 2: %q", lines, got)
	}

	if !strings.Contains(got, "a\n") || !strings.Contains(got, "b\n") {
		t.Fatalf("each key must appear exactly once: %q", got)
	}
}

// The full pipeline enforces the rate limit exactly.
func TestE2E_RateLimitExactCounts(t *testing.T) {
	const (
		total = 5
		limit = 2
	)

	buf, o := output.SafeBuffer(
		level.Trace,
		// window = 0: single infinite window - deterministic without a
		// fake clock.
		processor.RateLimit(limit, 0),
	)

	l := sypl.New("ratelimit-e2e", o)

	for i := range total {
		l.Printlnf(level.Info, "message-%d", i)
	}

	got := buf.String()

	if lines := countLines(got); lines != limit {
		t.Fatalf("got %d lines, expected %d: %q", lines, limit, got)
	}

	// The limiter is global (not per-key): the FIRST two messages passed.
	if !strings.Contains(got, "message-0\n") || !strings.Contains(got, "message-1\n") {
		t.Fatalf("the first %d messages must be the ones printed: %q", limit, got)
	}
}

// The full pipeline dedups by content: duplicates are muted within the
// window, distinct messages pass.
func TestE2E_DedupExactCounts(t *testing.T) {
	buf, o := output.SafeBuffer(level.Trace, processor.Dedup(time.Minute))

	l := sypl.New("dedup-e2e", o)

	l.Println(level.Info, "dup")
	l.Println(level.Info, "dup")
	l.Println(level.Info, "other")
	l.Println(level.Info, "dup")

	got := buf.String()

	if lines := countLines(got); lines != 2 {
		t.Fatalf("got %d lines, expected 2: %q", lines, got)
	}

	if !strings.Contains(got, "dup\n") || !strings.Contains(got, "other\n") {
		t.Fatalf("'dup', and 'other' must each appear exactly once: %q", got)
	}
}
