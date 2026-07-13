// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package syplslog

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/thalesfsp/sypl"
	"github.com/thalesfsp/sypl/fields"
	"github.com/thalesfsp/sypl/flag"
	"github.com/thalesfsp/sypl/level"
	"github.com/thalesfsp/sypl/processor"
	"github.com/thalesfsp/sypl/safebuffer"
)

//////
// Test helpers.
//////

// newTextSlog builds an *slog.Logger writing `logfmt` lines - time attr
// dropped for determinism - to the returned buffer, accepting records from
// `minLevel` up.
func newTextSlog(minLevel slog.Level) (*slog.Logger, *safebuffer.Buffer) {
	buf := &safebuffer.Buffer{}

	sl := slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{
		Level: minLevel,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey && len(groups) == 0 {
				return slog.Attr{}
			}

			return a
		},
	}))

	return sl, buf
}

// stubHandler is a controllable `slog.Handler`.
type stubHandler struct {
	mu      sync.Mutex
	records []slog.Record
	err     error
	enabled bool
}

func (h *stubHandler) Enabled(context.Context, slog.Level) bool { return h.enabled }

func (h *stubHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.records = append(h.records, r)

	return h.err
}

func (h *stubHandler) WithAttrs([]slog.Attr) slog.Handler { return h }

func (h *stubHandler) WithGroup(string) slog.Handler { return h }

func (h *stubHandler) count() int {
	h.mu.Lock()
	defer h.mu.Unlock()

	return len(h.records)
}

//////
// Forwarding tests.
//////

// Content, fields - as attrs - and mapped levels reach the slog side.
func TestOutput_ForwardsContentFieldsAndLevels(t *testing.T) {
	sl, buf := newTextSlog(LevelTrace)

	l := sypl.New("bridge", Output("slog", sl, level.Trace))

	l.PrintWithOptions(level.Info, "hello", sypl.WithFields(fields.Fields{"user": "thales"}))

	if got := buf.String(); got != "level=INFO msg=hello user=thales\n" {
		t.Fatalf("got %q", got)
	}

	buf.Reset()

	tests := []struct {
		level    level.Level
		expected string
	}{
		{level.Trace, "level=DEBUG-4"},
		{level.Debug, "level=DEBUG"},
		{level.Info, "level=INFO"},
		{level.Warn, "level=WARN"},
		{level.Error, "level=ERROR"},
	}

	for _, tc := range tests {
		buf.Reset()

		l.Print(tc.level, "m")

		if !strings.Contains(buf.String(), tc.expected) {
			t.Fatalf("level %v: got %q, expected it to contain %q", tc.level, buf.String(), tc.expected)
		}
	}
}

// Fields are forwarded as attrs in deterministic - sorted - order.
func TestOutput_FieldsSortedAsAttrs(t *testing.T) {
	sl, buf := newTextSlog(slog.LevelInfo)

	l := sypl.New("bridge", Output("slog", sl, level.Trace))

	l.PrintWithOptions(level.Info, "sorted", sypl.WithFields(fields.Fields{
		"zebra": 1,
		"alpha": 2,
		"mid":   3,
	}))

	if got := buf.String(); got != "level=INFO msg=sorted alpha=2 mid=3 zebra=1\n" {
		t.Fatalf("got %q", got)
	}
}

// The output's max level gates messages - standard sypl dispatch semantics.
func TestOutput_MaxLevelGates(t *testing.T) {
	sl, buf := newTextSlog(LevelTrace)

	l := sypl.New("bridge", Output("slog", sl, level.Info))

	l.Print(level.Debug, "too-verbose")

	if buf.String() != "" {
		t.Fatalf("Debug must not pass a maxLevel=Info output: %q", buf.String())
	}

	// Negative control: an accepted level IS forwarded.
	l.Print(level.Info, "accepted")

	if !strings.Contains(buf.String(), "msg=accepted") {
		t.Fatalf("Info must pass a maxLevel=Info output: %q", buf.String())
	}
}

// Muted messages aren't forwarded; forced messages are - despite the max
// level.
func TestOutput_MuteAndForceFlags(t *testing.T) {
	sl, buf := newTextSlog(LevelTrace)

	l := sypl.New("bridge", Output("slog", sl, level.Info))

	l.PrintWithOptions(level.Info, "muted", sypl.WithFlag(flag.Mute))

	if buf.String() != "" {
		t.Fatalf("muted message must not be forwarded: %q", buf.String())
	}

	l.PrintWithOptions(level.Trace, "forced", sypl.WithFlag(flag.Force))

	if !strings.Contains(buf.String(), "msg=forced") {
		t.Fatalf("forced message must be forwarded despite the max level: %q", buf.String())
	}
}

// Skip-flagged messages bypass sypl processing, and formatting - they are
// forwarded raw @ the Info level.
func TestOutput_SkipForwardsRaw(t *testing.T) {
	sl, buf := newTextSlog(LevelTrace)

	l := sypl.New("bridge", Output("slog", sl, level.Trace))

	l.PrintPretty(level.Warn, struct{ Key string }{Key: "value"})

	got := buf.String()

	if !strings.Contains(got, "level=INFO") {
		t.Fatalf("skip-flagged content is forwarded @ Info: %q", got)
	}

	if !strings.Contains(got, `\"Key\": \"value\"`) && !strings.Contains(got, `"Key": "value"`) {
		t.Fatalf("prettified content must be forwarded: %q", got)
	}

	buf.Reset()

	// SkipAndForce - e.g.: PrintNewLine - is forwarded raw too.
	l.PrintNewLine()

	if !strings.Contains(buf.String(), `msg=""`) {
		t.Fatalf("PrintNewLine is forwarded as an empty Info record: %q", buf.String())
	}
}

// The slog side may filter further: records below the slog handler's level
// aren't forwarded.
func TestOutput_SlogSideFiltering(t *testing.T) {
	sl, buf := newTextSlog(slog.LevelWarn)

	l := sypl.New("bridge", Output("slog", sl, level.Trace))

	l.Print(level.Info, "filtered")

	if buf.String() != "" {
		t.Fatalf("Info must be filtered by a Warn-level slog handler: %q", buf.String())
	}

	// Negative control.
	l.Print(level.Warn, "passed")

	if !strings.Contains(buf.String(), "msg=passed") {
		t.Fatalf("Warn must pass: %q", buf.String())
	}
}

// Processors attached to the output run before forwarding - standard
// processor semantics.
func TestOutput_ProcessorsRun(t *testing.T) {
	sl, buf := newTextSlog(slog.LevelInfo)

	l := sypl.New("bridge", Output("slog", sl, level.Trace, processor.Prefixer("P: ")))

	l.Print(level.Info, "content")

	if !strings.Contains(buf.String(), `msg="P: content"`) {
		t.Fatalf("processors must run before forwarding: %q", buf.String())
	}

	// Muting processors suppress forwarding entirely.
	buf.Reset()

	l2 := sypl.New(
		"bridge-muted",
		Output("slog", sl, level.Trace, processor.MuteBasedOnLevel(level.Info)),
	)

	l2.Print(level.Info, "muted-by-processor")

	if buf.String() != "" {
		t.Fatalf("muted message must not be forwarded: %q", buf.String())
	}
}

// A slog handler error must not crash the pipeline - and the message must not
// be double-forwarded through the raw path.
func TestOutput_HandlerErrorIsContained(t *testing.T) {
	stub := &stubHandler{enabled: true, err: errors.New("handle failed")}

	l := sypl.New("bridge", Output("slog", slog.New(stub), level.Trace))

	l.Print(level.Info, "erroring")

	if got := stub.count(); got != 1 {
		t.Fatalf("Handle called %d times, expected exactly 1 (no raw re-forward)", got)
	}
}

// A disabled slog handler receives nothing - neither structured, nor raw.
func TestOutput_DisabledSlogHandler(t *testing.T) {
	stub := &stubHandler{enabled: false}

	l := sypl.New("bridge", Output("slog", slog.New(stub), level.Trace))

	l.Print(level.Info, "dropped")

	if got := stub.count(); got != 0 {
		t.Fatalf("Handle called %d times, expected 0", got)
	}
}

// The sypl message timestamp is honored on the slog record.
func TestOutput_TimestampHonored(t *testing.T) {
	stub := &stubHandler{enabled: true}

	l := sypl.New("bridge", Output("slog", slog.New(stub), level.Trace))

	l.Print(level.Info, "ts")

	stub.mu.Lock()
	defer stub.mu.Unlock()

	if len(stub.records) != 1 {
		t.Fatalf("got %d records, expected 1", len(stub.records))
	}

	if stub.records[0].Time.IsZero() {
		t.Fatal("the record must carry the sypl message timestamp")
	}
}

//////
// Writer tests.
//////

// The writer drops content the forwarder already handled - marked - and raw
// content takes the raw path.
func TestSlogWriter_SentinelAndRawPaths(t *testing.T) {
	sl, buf := newTextSlog(slog.LevelInfo)

	w := &slogWriter{sl: sl}

	marked := []byte(forwardedMark + "\n")

	n, err := w.Write(marked)
	if err != nil || n != len(marked) {
		t.Fatalf("Write = (%d, %v), expected (%d, nil)", n, err, len(marked))
	}

	if buf.String() != "" {
		t.Fatalf("marked content must be dropped: %q", buf.String())
	}

	raw := []byte("raw line\r\n")

	n, err = w.Write(raw)
	if err != nil || n != len(raw) {
		t.Fatalf("Write = (%d, %v), expected (%d, nil)", n, err, len(raw))
	}

	if got := buf.String(); got != `level=INFO msg="raw line"`+"\n" {
		t.Fatalf("raw content must be forwarded @ Info, trimmed: %q", got)
	}
}

//////
// Chaining, and concurrency tests.
//////

// Both directions chained: slog API -> Handler -> sypl -> Output -> slog
// TextHandler.
func TestOutput_ChainedBothDirections(t *testing.T) {
	sl, buf := newTextSlog(LevelTrace)

	syplLogger := sypl.New("chain", Output("slog", sl, level.Trace))

	api := slog.New(NewHandler(syplLogger))

	api.Info("chained", "k", "v")

	if got := buf.String(); got != "level=INFO msg=chained k=v\n" {
		t.Fatalf("got %q", got)
	}
}

// Concurrent use is race-clean, and lossless.
func TestOutput_ConcurrentUse(t *testing.T) {
	const (
		goroutines         = 8
		messagesPerRoutine = 25
	)

	sl, buf := newTextSlog(slog.LevelInfo)

	l := sypl.New("bridge", Output("slog", sl, level.Trace))

	var wg sync.WaitGroup

	wg.Add(goroutines)

	for g := range goroutines {
		go func(g int) {
			defer wg.Done()

			for j := range messagesPerRoutine {
				l.Printf(level.Info, "g%02d-m%02d", g, j)
			}
		}(g)
	}

	wg.Wait()

	lines := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")

	if len(lines) != goroutines*messagesPerRoutine {
		t.Fatalf("got %d lines, expected %d", len(lines), goroutines*messagesPerRoutine)
	}
}
