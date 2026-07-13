// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package syplslog

import (
	"log/slog"

	"github.com/thalesfsp/sypl/level"
)

// Bridge-specific slog levels, extending the standard ones so every sypl
// level - but `None` - has a distinct slog counterpart.
const (
	// LevelTrace is the slog level sypl's `Trace` maps to, and from. It sits
	// below `slog.LevelDebug`, mirroring the standard four-apart spacing.
	LevelTrace slog.Level = slog.LevelDebug - 4

	// LevelFatal is the slog level sypl's `Fatal` maps to. It sits above
	// `slog.LevelError`, mirroring the standard four-apart spacing.
	//
	// NOTE: The bridge only maps TO it - `ToSyplLevel` never yields
	// `level.Fatal`, as sypl exits the process on `Fatal`.
	LevelFatal slog.Level = slog.LevelError + 4
)

// ToSyplLevel maps a slog level to a sypl level. See the package
// documentation for the mapping table. In-between levels map conservatively -
// down to the nearest standard level - so they are MORE likely to be printed.
// It never yields `level.Fatal` - sypl exits the process on `Fatal`.
func ToSyplLevel(l slog.Level) level.Level {
	switch {
	case l < slog.LevelDebug:
		return level.Trace
	case l < slog.LevelInfo:
		return level.Debug
	case l < slog.LevelWarn:
		return level.Info
	case l < slog.LevelError:
		return level.Warn
	default:
		return level.Error
	}
}

// ToSlogLevel maps a sypl level to a slog level. See the package
// documentation for the mapping table. `None` - which sypl never prints,
// unless forced - and unknown levels map to `slog.LevelInfo`.
func ToSlogLevel(l level.Level) slog.Level {
	switch l {
	case level.Trace:
		return LevelTrace
	case level.Debug:
		return slog.LevelDebug
	case level.Info:
		return slog.LevelInfo
	case level.Warn:
		return slog.LevelWarn
	case level.Error:
		return slog.LevelError
	case level.Fatal:
		return LevelFatal
	case level.None:
		return slog.LevelInfo
	default:
		return slog.LevelInfo
	}
}
