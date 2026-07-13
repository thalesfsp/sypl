// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package syplslog

import (
	"log/slog"
	"testing"

	"github.com/thalesfsp/sypl/v2/level"
)

// The slog -> sypl mapping table, including the boundaries between the
// standard slog levels.
func TestToSyplLevel(t *testing.T) {
	tests := []struct {
		in       slog.Level
		expected level.Level
	}{
		{LevelTrace, level.Trace},
		{slog.LevelDebug - 1, level.Trace},
		{slog.LevelDebug, level.Debug},
		{slog.LevelInfo - 1, level.Debug},
		{slog.LevelInfo, level.Info},
		{slog.LevelWarn - 1, level.Info},
		{slog.LevelWarn, level.Warn},
		{slog.LevelError - 1, level.Warn},
		{slog.LevelError, level.Error},
		{LevelFatal, level.Error},
	}

	for _, tc := range tests {
		if got := ToSyplLevel(tc.in); got != tc.expected {
			t.Fatalf("ToSyplLevel(%v) = %v, expected %v", tc.in, got, tc.expected)
		}
	}
}

// The sypl -> slog mapping table. `Fatal` maps to `LevelFatal` - the bridge
// NEVER exits the process. `None`, and unknown levels map to `Info`.
func TestToSlogLevel(t *testing.T) {
	const unknownLevel = level.Level(42)

	tests := []struct {
		in       level.Level
		expected slog.Level
	}{
		{level.Trace, LevelTrace},
		{level.Debug, slog.LevelDebug},
		{level.Info, slog.LevelInfo},
		{level.Warn, slog.LevelWarn},
		{level.Error, slog.LevelError},
		{level.Fatal, LevelFatal},
		{level.None, slog.LevelInfo},
		{unknownLevel, slog.LevelInfo},
	}

	for _, tc := range tests {
		if got := ToSlogLevel(tc.in); got != tc.expected {
			t.Fatalf("ToSlogLevel(%v) = %v, expected %v", tc.in, got, tc.expected)
		}
	}
}

// The mappings round-trip for the levels both sides share.
func TestLevelRoundTrip(t *testing.T) {
	for _, l := range []level.Level{
		level.Trace, level.Debug, level.Info, level.Warn, level.Error,
	} {
		if got := ToSyplLevel(ToSlogLevel(l)); got != l {
			t.Fatalf("round-trip of %v = %v", l, got)
		}
	}
}
