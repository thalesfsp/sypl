// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package sypl

import (
	"context"
	"fmt"

	"github.com/thalesfsp/sypl/v2/fields"
	"github.com/thalesfsp/sypl/v2/level"
)

//////
// Context helpers.
//
// Two independent capabilities:
//
//  1. Carrying a logger THROUGH a context: `NewContext`/`FromContext`/
//     `FromContextOrDefault`.
//  2. Extracting fields FROM a context: `SetContextExtractor` +
//     `PrintWithContext` (and the leveled `*WithContext` variants).
//
// Sypl deliberately imports NO tracing library (e.g. otel): applications
// wire their own extractor pulling whatever they need - trace IDs, request
// IDs, tenant IDs - out of their contexts. See the
// `ExampleSypl_SetContextExtractor` example test.
//////

// contextKey is the unexported context key type - collision-proof by
// construction.
type contextKey struct{}

// NewContext returns a copy of `ctx` carrying `l`.
func NewContext(ctx context.Context, l *Sypl) context.Context {
	return context.WithValue(ctx, contextKey{}, l)
}

// FromContext returns the `*Sypl` carried by `ctx`, and whether one was
// found. Tolerates a nil `ctx`, and a stored nil logger: (nil, false).
func FromContext(ctx context.Context) (*Sypl, bool) {
	if ctx == nil {
		return nil, false
	}

	l, ok := ctx.Value(contextKey{}).(*Sypl)
	if !ok || l == nil {
		return nil, false
	}

	return l, true
}

// FromContextOrDefault returns the `*Sypl` carried by `ctx`; `fallback` when
// none is found; and - when `fallback` is also nil - a fresh
// `NewDefault("sypl", level.Info)` logger, so the result is ALWAYS usable.
func FromContextOrDefault(ctx context.Context, fallback *Sypl) *Sypl {
	if l, ok := FromContext(ctx); ok {
		return l
	}

	if fallback != nil {
		return fallback
	}

	return NewDefault("sypl", level.Info)
}

//////
// Context extractor.
//////

// SetContextExtractor sets the function `PrintWithContext` (and the leveled
// `*WithContext` variants) use to pull structured fields out of a context.
// The extracted fields are merged into the message as MESSAGE-level fields -
// so they take precedence over the logger's global fields on key conflict.
// A nil extractor (the default) makes the `*WithContext` printers behave
// exactly like their plain counterparts.
func (sypl *Sypl) SetContextExtractor(fn func(ctx context.Context) fields.Fields) *Sypl {
	sypl.lock()
	defer sypl.unlock()

	sypl.contextExtractor = fn

	return sypl
}

// GetContextExtractor returns the registered context extractor - nil if
// none.
func (sypl *Sypl) GetContextExtractor() func(ctx context.Context) fields.Fields {
	sypl.rLock()
	defer sypl.rUnlock()

	return sypl.contextExtractor
}

// extractFields runs the registered extractor against `ctx` - nil-safe on
// every axis (nil receiver, nil context, nil extractor, nil result).
func (sypl *Sypl) extractFields(ctx context.Context) fields.Fields {
	if sypl == nil || ctx == nil {
		return nil
	}

	fn := sypl.GetContextExtractor()
	if fn == nil {
		return nil
	}

	return fn(ctx)
}

//////
// Context-aware printers.
//////

// PrintWithContext prints at the specified level; when a context extractor
// is registered (see `SetContextExtractor`), the extracted fields are merged
// into the message - message-level fields win on conflict.
func (sypl *Sypl) PrintWithContext(ctx context.Context, l level.Level, args ...interface{}) ISypl {
	// Gated BEFORE content formatting, and extraction - fields cannot alter
	// the message's level, nor flag. See `fastGated`.
	if sypl.fastGated(l) {
		return sypl
	}

	extracted := sypl.extractFields(ctx)

	if len(extracted) == 0 {
		return sypl.PrintWithOptions(l, fmt.Sprint(args...))
	}

	return sypl.PrintWithOptions(l, fmt.Sprint(args...), WithFields(extracted))
}

// DebugWithContext prints @ the Debug level, merging extracted context
// fields. See `PrintWithContext`.
func (sypl *Sypl) DebugWithContext(ctx context.Context, args ...interface{}) ISypl {
	return sypl.PrintWithContext(ctx, level.Debug, args...)
}

// InfoWithContext prints @ the Info level, merging extracted context
// fields. See `PrintWithContext`.
func (sypl *Sypl) InfoWithContext(ctx context.Context, args ...interface{}) ISypl {
	return sypl.PrintWithContext(ctx, level.Info, args...)
}

// WarnWithContext prints @ the Warn level, merging extracted context
// fields. See `PrintWithContext`.
func (sypl *Sypl) WarnWithContext(ctx context.Context, args ...interface{}) ISypl {
	return sypl.PrintWithContext(ctx, level.Warn, args...)
}

// ErrorWithContext prints @ the Error level, merging extracted context
// fields. See `PrintWithContext`.
func (sypl *Sypl) ErrorWithContext(ctx context.Context, args ...interface{}) ISypl {
	return sypl.PrintWithContext(ctx, level.Error, args...)
}
