// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

// Benchmarks for the hot logging path. Run with:
//
//	go test -run xxx -bench . -benchmem ./...
package sypl_test

import (
	"io"
	"testing"

	"github.com/thalesfsp/sypl/v2"
	"github.com/thalesfsp/sypl/v2/fields"
	"github.com/thalesfsp/sypl/v2/formatter"
	"github.com/thalesfsp/sypl/v2/level"
	"github.com/thalesfsp/sypl/v2/output"
)

// discardOutput builds an Info-capped output that mimics a console output -
// text formatter, same pipeline - but writes to io.Discard, so the benchmark
// measures the logging path, not terminal I/O.
func discardOutput(name string) output.IOutput {
	return output.New(name, level.Info, io.Discard).SetFormatter(formatter.Text())
}

// BenchmarkPrint_SingleConsoleOutput measures the simplest hot path: one
// enabled output, message level allowed.
func BenchmarkPrint_SingleConsoleOutput(b *testing.B) {
	l := sypl.New("bench", discardOutput("Discard"))

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		l.Print(level.Info, "benchmark message")
	}
}

// BenchmarkPrint_TwoOutputs measures the concurrent fan-out path: two enabled
// outputs, message level allowed on both.
func BenchmarkPrint_TwoOutputs(b *testing.B) {
	l := sypl.New(
		"bench",
		discardOutput("DiscardA"),
		discardOutput("DiscardB"),
	)

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		l.Print(level.Info, "benchmark message")
	}
}

// BenchmarkPrint_MutedLevel measures the cost of a message that will NOT be
// printed: Debug against an Info-capped output.
func BenchmarkPrint_MutedLevel(b *testing.B) {
	l := sypl.New("bench", discardOutput("Discard"))

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		l.Print(level.Debug, "benchmark message")
	}
}

// BenchmarkPrint_MutedLevel_FastGate measures the same muted call with the
// opt-in fast level gate enabled: it must return before any message
// construction.
func BenchmarkPrint_MutedLevel_FastGate(b *testing.B) {
	l := sypl.New("bench", discardOutput("Discard")).SetFastGate(true)

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		l.Print(level.Debug, "benchmark message")
	}
}

// BenchmarkWith measures derived-logger construction (fields merge + state
// copy).
func BenchmarkWith(b *testing.B) {
	l := sypl.New("bench", discardOutput("Discard"))
	l.SetFields(fields.Fields{"parent": "value"})

	f := fields.Fields{"child": "value"}

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		_ = l.With(f)
	}
}

// BenchmarkPrintWithOptions_Fields measures the structured-fields path.
func BenchmarkPrintWithOptions_Fields(b *testing.B) {
	l := sypl.New("bench", discardOutput("Discard"))

	f := fields.Fields{"key1": "value1", "key2": 2}

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		l.PrintWithOptions(level.Info, "benchmark message", sypl.WithFields(f))
	}
}
