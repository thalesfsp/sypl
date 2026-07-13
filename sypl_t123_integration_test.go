// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package sypl_test

import (
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/thalesfsp/sypl"
	"github.com/thalesfsp/sypl/fields"
	"github.com/thalesfsp/sypl/level"
	"github.com/thalesfsp/sypl/output"
	"github.com/thalesfsp/sypl/processor"
	"github.com/thalesfsp/sypl/syplslog"
)

// Cross-lane integration tests: these exercise the seams BETWEEN the three
// Tier 1-3 workstreams (hot-path/core API, reliability outputs, processors +
// slog bridge), which no single workstream's suite covers end to end.

const integrationPayload = "integration-payload"

// Sypl.Flush (core API) must drain an Async-wrapped output (reliability).
func TestIntegration_FlushDrainsAsyncOutput(t *testing.T) {
	buf, inner := output.SafeBuffer(level.Trace)
	l := sypl.New("flush-async", output.Async(inner))

	for range 50 {
		l.Infoln(integrationPayload)
	}

	if err := l.Flush(); err != nil {
		t.Fatalf("Flush() error = %v, want nil", err)
	}

	if got := strings.Count(buf.String(), integrationPayload); got != 50 {
		t.Fatalf("after Flush, buffer has %d payloads, want 50", got)
	}
}

// Sypl.Close must stop the async worker; a write after Close must surface
// through the Sypl-level error handler with the output name wrapped in.
func TestIntegration_CloseAsyncThenWriteReportsError(t *testing.T) {
	_, inner := output.SafeBuffer(level.Trace)

	var handled []error

	l := sypl.New("close-async", output.Async(inner))
	l.SetErrorHandler(func(err error) { handled = append(handled, err) })

	l.Infoln(integrationPayload)

	if err := l.Close(); err != nil {
		t.Fatalf("Close() error = %v, want nil", err)
	}

	l.Infoln("after-close")

	if len(handled) == 0 {
		t.Fatal("write after Close was silently swallowed, want error via handler")
	}

	if !strings.Contains(handled[0].Error(), "output Buffer:") {
		t.Errorf("handler error = %q, want the output name wrapped in", handled[0])
	}
}

// A Fatal message routed through an Async output must be flushed to the
// inner sink before the process exits (core Fatal-flush + async drain).
func TestIntegration_FatalFlushesAsyncBeforeExit(t *testing.T) {
	const marker = "fatal-through-async"

	if path, ok := os.LookupEnv("SYPL_T123_FATAL_PATH"); ok {
		inner := output.File("file", path, level.Trace)
		l := sypl.New("fatal-async", output.Async(inner))

		l.Fatalln(marker)

		os.Exit(42) // Sentinel: Fatalln must have exited already.
	}

	path := t.TempDir() + "/fatal.log"

	//nolint:gosec // Re-executing the test binary itself.
	cmd := exec.Command(os.Args[0], "-test.run", "^TestIntegration_FatalFlushesAsyncBeforeExit$", "-test.v")
	cmd.Env = append(os.Environ(), "SYPL_T123_FATAL_PATH="+path)

	err := cmd.Run()

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
		t.Fatalf("child = %v, want exit code 1 from Fatalln", err)
	}

	content, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("reading the fatal log: %v", readErr)
	}

	if !strings.Contains(string(content), marker) {
		t.Fatalf("fatal message was not flushed before exit; file = %q", string(content))
	}
}

// Dedup (processors) keys on the content hash, which is now lazy and shared
// across per-output copies (hot-path): suppression must still be exact when
// one processor instance serves two outputs.
func TestIntegration_DedupAcrossOutputsWithLazyHash(t *testing.T) {
	dedup := processor.Dedup(time.Hour)

	bufA, outA := output.SafeBuffer(level.Trace, dedup)
	bufB, outB := output.SafeBuffer(level.Trace, dedup)

	l := sypl.New("dedup-lazy", outA, outB)

	l.Infoln(integrationPayload)
	l.Infoln(integrationPayload)

	// The shared dedup instance sees each print once per output copy: the
	// first print passes on one output's copy and is suppressed on the
	// other (same key, same window). The invariant that matters: exactly
	// ONE line total across both buffers, not zero, not three.
	total := strings.Count(bufA.String(), integrationPayload) +
		strings.Count(bufB.String(), integrationPayload)
	if total != 1 {
		t.Fatalf("dedup across outputs let %d lines through, want exactly 1", total)
	}
}

// The fast gate (hot-path) must coexist with a sampling processor
// (processors): gated levels never reach the sampler; passing levels sample
// exactly.
func TestIntegration_FastGateWithSampler(t *testing.T) {
	sampler := processor.Sample(processor.SampleConfig{First: 2})

	buf, out := output.SafeBuffer(level.Info, sampler)

	l := sypl.New("gate-sample", out).SetFastGate(true)

	for range 5 {
		l.Debugln("gated-away") // Above Info: gated, sampler never runs.
		l.Infoln(integrationPayload)
	}

	if got := strings.Count(buf.String(), "gated-away"); got != 0 {
		t.Fatalf("fast gate leaked %d Debug lines, want 0", got)
	}

	if got := strings.Count(buf.String(), integrationPayload); got != 2 {
		t.Fatalf("sampler passed %d Info lines, want exactly 2 (First=2)", got)
	}
}

// A With-derived logger (hot-path) must flow its merged fields into the
// Recorder output's structured snapshots (reliability).
func TestIntegration_WithDerivedFieldsRecorded(t *testing.T) {
	rec, out := output.Recorder(level.Trace)

	parent := sypl.New("with-recorder", out)
	child := parent.With(fields.Fields{"request_id": "r-42"})

	child.Errorln("boom")

	records := rec.Messages()
	if len(records) != 1 {
		t.Fatalf("recorder captured %d records, want 1", len(records))
	}

	if got := records[0].Fields["request_id"]; got != "r-42" {
		t.Errorf(`record field request_id = %v, want "r-42"`, got)
	}

	if records[0].Level != level.Error {
		t.Errorf("record level = %v, want %v", records[0].Level, level.Error)
	}

	if !strings.Contains(records[0].ProcessedContent, "boom") {
		t.Errorf("record content = %q, want it to contain 'boom'", records[0].ProcessedContent)
	}
}

// A library speaking slog (bridge) must land in a sypl pipeline whose sink
// is async (reliability) and be drained by Sypl.Flush (core API).
func TestIntegration_SlogThroughAsyncFlush(t *testing.T) {
	buf, inner := output.SafeBuffer(level.Trace)

	l := sypl.New("slog-async", output.Async(inner))

	slogger := slog.New(syplslog.NewHandler(l))
	slogger.Info(integrationPayload, "tenant", "acme")

	if err := l.Flush(); err != nil {
		t.Fatalf("Flush() error = %v, want nil", err)
	}

	if !strings.Contains(buf.String(), integrationPayload) {
		t.Fatalf("slog record did not reach the async sink; buffer = %q", buf.String())
	}
}
