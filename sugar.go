// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package sypl

import (
	"strconv"

	"github.com/thalesfsp/sypl/v2/fields"
	"github.com/thalesfsp/sypl/v2/level"
)

//////
// Key-value sugar.
//
// Loosely-typed, slog/zap-style printers: alternating key-value pairs become
// structured fields. Malformed input NEVER panics, and the message is always
// still logged:
//   - a non-string key becomes the field "!BADKEY<idx>" carrying the
//     offending element (one element is consumed - the next one is treated
//     as a key again);
//   - an odd trailing key gets the value "(MISSING)".
//////

// badKeyPrefix prefixes the synthetic field name generated for a non-string
// key; missingValue is the synthetic value for an odd trailing key.
const (
	badKeyPrefix = "!BADKEY"

	missingValue = "(MISSING)"

	// kvPairWidth is the width of one key-value pair.
	kvPairWidth = 2
)

// kvToFields converts alternating key-value pairs into fields - tolerating
// non-string keys, and an odd trailing key. Never panics.
func kvToFields(keysAndValues []any) fields.Fields {
	f := make(fields.Fields, len(keysAndValues)/kvPairWidth)

	i := 0

	for i < len(keysAndValues) {
		k, ok := keysAndValues[i].(string)

		// Non-string key: synthesize a field carrying the offending
		// element, and consume ONLY it - the next element may be a valid
		// key.
		if !ok {
			f[badKeyPrefix+strconv.Itoa(i)] = keysAndValues[i]

			i++

			continue
		}

		// Odd trailing key: no value left.
		if i+1 >= len(keysAndValues) {
			f[k] = missingValue

			i++

			continue
		}

		f[k] = keysAndValues[i+1]

		i += kvPairWidth
	}

	return f
}

// Logw prints `msg` at the specified level, with the alternating key-value
// pairs as structured fields. See the package sugar notes for the malformed-
// input tolerance rules.
func (sypl *Sypl) Logw(l level.Level, msg string, keysAndValues ...any) {
	// Respects the opt-in fast gate: fields cannot alter the message's
	// level, nor flag, so a gated level is safe to drop before the
	// key-value conversion. Fatal is never gated.
	if sypl.fastGated(l) {
		return
	}

	sypl.PrintWithOptions(l, msg, WithFields(kvToFields(keysAndValues)))
}

// Tracew prints @ the Trace level - key-value pairs become fields.
func (sypl *Sypl) Tracew(msg string, keysAndValues ...any) {
	sypl.Logw(level.Trace, msg, keysAndValues...)
}

// Debugw prints @ the Debug level - key-value pairs become fields.
func (sypl *Sypl) Debugw(msg string, keysAndValues ...any) {
	sypl.Logw(level.Debug, msg, keysAndValues...)
}

// Infow prints @ the Info level - key-value pairs become fields.
func (sypl *Sypl) Infow(msg string, keysAndValues ...any) {
	sypl.Logw(level.Info, msg, keysAndValues...)
}

// Warnw prints @ the Warn level - key-value pairs become fields.
func (sypl *Sypl) Warnw(msg string, keysAndValues ...any) {
	sypl.Logw(level.Warn, msg, keysAndValues...)
}

// Errorw prints @ the Error level - key-value pairs become fields.
func (sypl *Sypl) Errorw(msg string, keysAndValues ...any) {
	sypl.Logw(level.Error, msg, keysAndValues...)
}

// Fatalw prints @ the Fatal level - key-value pairs become fields - then
// exits with os.Exit(1).
func (sypl *Sypl) Fatalw(msg string, keysAndValues ...any) {
	sypl.Logw(level.Fatal, msg, keysAndValues...)
}
