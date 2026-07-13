// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package sypl_test

import (
	"context"
	"sync"
	"testing"

	"github.com/thalesfsp/sypl"
	"github.com/thalesfsp/sypl/fields"
	"github.com/thalesfsp/sypl/formatter"
	"github.com/thalesfsp/sypl/level"
	"github.com/thalesfsp/sypl/output"
)

// traceIDKey is this test's private context key for the fake trace ID.
type traceIDKey struct{}

// traceIDExtractor pulls the fake trace ID out of the context.
func traceIDExtractor(ctx context.Context) fields.Fields {
	if traceID, ok := ctx.Value(traceIDKey{}).(string); ok {
		return fields.Fields{"trace_id": traceID}
	}

	return nil
}

// Round-trip: the logger stored via NewContext is returned by FromContext.
func TestContext_RoundTrip(t *testing.T) {
	l := sypl.New("ctx-roundtrip")

	ctx := sypl.NewContext(context.Background(), l)

	got, ok := sypl.FromContext(ctx)

	if !ok || got != l {
		t.Fatalf("FromContext = (%v, %v), want the stored logger, true", got, ok)
	}
}

// Missing, nil-ctx, and nil-logger lookups: (nil, false) - never a panic.
func TestContext_MissingAndNil(t *testing.T) {
	if got, ok := sypl.FromContext(context.Background()); ok || got != nil {
		t.Fatalf("FromContext(empty ctx) = (%v, %v), want (nil, false)", got, ok)
	}

	//nolint:staticcheck // Deliberate nil-context robustness probe.
	if got, ok := sypl.FromContext(nil); ok || got != nil {
		t.Fatalf("FromContext(nil) = (%v, %v), want (nil, false)", got, ok)
	}

	// A stored NIL logger does not count as found.
	ctx := sypl.NewContext(context.Background(), nil)

	if got, ok := sypl.FromContext(ctx); ok || got != nil {
		t.Fatalf("FromContext(ctx with nil logger) = (%v, %v), want (nil, false)", got, ok)
	}
}

// FromContextOrDefault: context logger > fallback > NewDefault("sypl", Info).
func TestContext_FromContextOrDefault(t *testing.T) {
	stored := sypl.New("ctx-stored")
	fallback := sypl.New("ctx-fallback")

	ctx := sypl.NewContext(context.Background(), stored)

	if got := sypl.FromContextOrDefault(ctx, fallback); got != stored {
		t.Fatalf("with stored logger: got %v, want the stored one", got)
	}

	if got := sypl.FromContextOrDefault(context.Background(), fallback); got != fallback {
		t.Fatalf("without stored logger: got %v, want the fallback", got)
	}

	got := sypl.FromContextOrDefault(context.Background(), nil)

	if got == nil {
		t.Fatal("nil fallback: got nil, want a NewDefault logger")
	}

	if got.GetName() != "sypl" {
		t.Fatalf("nil fallback: name = %q, want %q", got.GetName(), "sypl")
	}

	if len(got.GetOutputs()) == 0 {
		t.Fatal("nil fallback: default logger has no outputs")
	}
}

// SetContextExtractor is chainable, and retrievable.
func TestContext_ExtractorGetSet(t *testing.T) {
	l := sypl.New("ctx-getset")

	if l.GetContextExtractor() != nil {
		t.Fatal("GetContextExtractor() != nil on a fresh logger")
	}

	if got := l.SetContextExtractor(traceIDExtractor); got != l {
		t.Fatal("SetContextExtractor must return the same *Sypl for chaining")
	}

	fn := l.GetContextExtractor()

	if fn == nil {
		t.Fatal("GetContextExtractor() = nil after SetContextExtractor")
	}

	ctx := context.WithValue(context.Background(), traceIDKey{}, "t-1")

	if f := fn(ctx); f["trace_id"] != "t-1" {
		t.Fatalf("retrieved extractor returned %v, want trace_id=t-1", f)
	}
}

// PrintWithContext merges the extracted fields into the message; extracted
// fields are message-level, so they WIN over the logger's global fields.
func TestContext_PrintWithContextExtractsAndMerges(t *testing.T) {
	buf, o := output.SafeBuffer(level.Trace)
	o.SetFormatter(formatter.JSON())

	l := sypl.New("ctx-extract", o)
	l.SetFields(fields.Fields{"env": "prod", "trace_id": "global-loses"})
	l.SetContextExtractor(traceIDExtractor)

	ctx := context.WithValue(context.Background(), traceIDKey{}, "abc-123")

	l.PrintWithContext(ctx, level.Info, "handling request")

	decoded := sugarLine(t, buf)

	if decoded["trace_id"] != "abc-123" {
		t.Fatalf("trace_id = %v, want abc-123 (extracted, message-level, wins)", decoded["trace_id"])
	}

	if decoded["env"] != "prod" {
		t.Fatalf("global field env = %v, want prod", decoded["env"])
	}

	if decoded["message"] != "handling request" {
		t.Fatalf("message = %v, want %q", decoded["message"], "handling request")
	}
}

// Robustness: no extractor, nil context, and an extractor returning nil all
// behave like a plain Print - never panic, never add fields.
func TestContext_PrintWithContextEdgeCases(t *testing.T) {
	buf, o := output.SafeBuffer(level.Trace)
	o.SetFormatter(formatter.JSON())

	l := sypl.New("ctx-edge", o)

	// No extractor set.
	ctx := context.WithValue(context.Background(), traceIDKey{}, "unused")

	l.PrintWithContext(ctx, level.Info, "no extractor")

	if decoded := sugarLine(t, buf); decoded["trace_id"] != nil {
		t.Fatalf("no extractor: trace_id = %v, want absent", decoded["trace_id"])
	}

	// Extractor set, nil context.
	l.SetContextExtractor(traceIDExtractor)

	buf.Reset()

	//nolint:staticcheck // Deliberate nil-context robustness probe.
	l.PrintWithContext(nil, level.Info, "nil ctx")

	if decoded := sugarLine(t, buf); decoded["message"] != "nil ctx" {
		t.Fatalf("nil ctx: message = %v, want %q", decoded["message"], "nil ctx")
	}

	// Extractor returning nil fields (no trace ID in ctx).
	buf.Reset()

	l.PrintWithContext(context.Background(), level.Info, "no trace id")

	if decoded := sugarLine(t, buf); decoded["trace_id"] != nil {
		t.Fatalf("extractor returned nil: trace_id = %v, want absent", decoded["trace_id"])
	}
}

// The leveled variants print at THEIR levels, carrying extracted fields.
func TestContext_LeveledVariants(t *testing.T) {
	buf, o := output.SafeBuffer(level.Trace)
	o.SetFormatter(formatter.JSON())

	l := sypl.New("ctx-levels", o)
	l.SetContextExtractor(traceIDExtractor)

	ctx := context.WithValue(context.Background(), traceIDKey{}, "lvl-1")

	tests := []struct {
		call  func()
		level string
	}{
		{func() { l.DebugWithContext(ctx, "m") }, "debug"},
		{func() { l.InfoWithContext(ctx, "m") }, "info"},
		{func() { l.WarnWithContext(ctx, "m") }, "warn"},
		{func() { l.ErrorWithContext(ctx, "m") }, "error"},
	}

	for _, tt := range tests {
		buf.Reset()

		tt.call()

		decoded := sugarLine(t, buf)

		if decoded["level"] != tt.level {
			t.Fatalf("level = %v, want %s", decoded["level"], tt.level)
		}

		if decoded["trace_id"] != "lvl-1" {
			t.Fatalf("%s: trace_id = %v, want lvl-1", tt.level, decoded["trace_id"])
		}
	}
}

// A derived (With) logger inherits the context extractor.
func TestContext_WithInheritsExtractor(t *testing.T) {
	buf, o := output.SafeBuffer(level.Trace)
	o.SetFormatter(formatter.JSON())

	parent := sypl.New("ctx-with", o)
	parent.SetContextExtractor(traceIDExtractor)

	child := parent.With(fields.Fields{"component": "child"})

	ctx := context.WithValue(context.Background(), traceIDKey{}, "inherited")

	child.InfoWithContext(ctx, "child with ctx")

	decoded := sugarLine(t, buf)

	if decoded["trace_id"] != "inherited" {
		t.Fatalf("trace_id = %v, want inherited (child lost the extractor)", decoded["trace_id"])
	}

	if decoded["component"] != "child" {
		t.Fatalf("component = %v, want child", decoded["component"])
	}
}

// Concurrent PrintWithContext + extractor reconfiguration must be race-clean.
func TestContext_ConcurrentRaceClean(t *testing.T) {
	_, o := output.SafeBuffer(level.Trace)

	l := sypl.New("ctx-race", o)

	ctx := context.WithValue(context.Background(), traceIDKey{}, "race")

	const goroutines = 8

	var wg sync.WaitGroup

	wg.Add(goroutines * 2)

	for range goroutines {
		go func() {
			defer wg.Done()

			for range 25 {
				l.PrintWithContext(ctx, level.Info, "concurrent ctx print")
			}
		}()

		go func() {
			defer wg.Done()

			for range 25 {
				l.SetContextExtractor(traceIDExtractor)
				l.SetContextExtractor(nil)
			}
		}()
	}

	wg.Wait()
}
