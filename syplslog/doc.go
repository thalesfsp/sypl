// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

// Package syplslog bridges sypl, and the standard library's structured
// logger, log/slog - in both directions, using only the standard library:
//
//   - `NewHandler` adapts a sypl logger into a `slog.Handler`, so code using
//     the slog API logs through sypl's pipeline (outputs, processors,
//     formatters).
//   - `Output` adapts an `*slog.Logger` into a sypl output, so a sypl logger
//     forwards its processed messages - fields as attrs - to any slog
//     handler.
//
// # Level mapping
//
// slog levels are sparse integers (Debug=-4, Info=0, Warn=4, Error=8),
// designed to accommodate in-between levels. The bridge maps them as
// follows:
//
//	slog level              sypl level      note
//	----------------------  --------------  ----------------------------------
//	< LevelDebug (-4)       level.Trace     e.g.: syplslog.LevelTrace (-8)
//	[LevelDebug, LevelInfo) level.Debug
//	[LevelInfo, LevelWarn)  level.Info
//	[LevelWarn, LevelError) level.Warn
//	>= LevelError (8)       level.Error     incl. syplslog.LevelFatal (12)
//
// In-between levels (e.g.: Info+2) map conservatively - DOWN to the nearest
// standard level - so they are MORE likely to be printed. The reverse - sypl
// to slog - mapping:
//
//	sypl level     slog level
//	-------------  ------------------------
//	level.Trace    syplslog.LevelTrace (-8)
//	level.Debug    slog.LevelDebug (-4)
//	level.Info     slog.LevelInfo (0)
//	level.Warn     slog.LevelWarn (4)
//	level.Error    slog.LevelError (8)
//	level.Fatal    syplslog.LevelFatal (12)
//	level.None     slog.LevelInfo (0)
//
// `ToSyplLevel` NEVER yields `level.Fatal` - sypl exits the process on
// `Fatal`, which a logging bridge must never do.
//
// NOTE: In sypl's ordering, Warn is MORE verbose than Info: an output @
// `maxLevel` Info does not print Warn. The bridge is faithful to that.
//
// # Groups, and attrs
//
// slog groups are flattened into sypl field keys as "group.key" - e.g.:
// attr "err" under groups "req", and "db" becomes the field "req.db.err".
// `LogValuer`s are resolved - a panicking `LogValue` is recovered, yielding
// an error value describing the panic. The record PC (source) is not
// forwarded.
package syplslog
