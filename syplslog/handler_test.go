// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package syplslog

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/thalesfsp/sypl/v2"
	"github.com/thalesfsp/sypl/v2/fields"
	"github.com/thalesfsp/sypl/v2/level"
	"github.com/thalesfsp/sypl/v2/message"
	"github.com/thalesfsp/sypl/v2/output"
	"github.com/thalesfsp/sypl/v2/processor"
	"github.com/thalesfsp/sypl/v2/safebuffer"
	"github.com/thalesfsp/sypl/v2/status"
)

//////
// Test helpers.
//////

// recorded is a snapshot of a message observed by the recorder processor.
type recorded struct {
	content   string
	fields    fields.Fields
	level     level.Level
	timestamp time.Time
}

// newRecorderLogger builds a sypl logger whose single output snapshots every
// processed message - level, processed content, fields, and timestamp - also
// writing it to the returned buffer.
func newRecorderLogger(maxLevel level.Level) (*sypl.Sypl, *safebuffer.Buffer, func() []recorded) {
	var mu sync.Mutex

	records := []recorded{}

	rec := processor.New("Recorder", func(m message.IMessage) error {
		mu.Lock()
		defer mu.Unlock()

		records = append(records, recorded{
			content:   m.GetContent().GetProcessed(),
			fields:    fields.Copy(m.GetFields(), fields.Fields{}),
			level:     m.GetLevel(),
			timestamp: m.GetTimestamp(),
		})

		return nil
	})

	buf, o := output.SafeBuffer(maxLevel, rec)

	l := sypl.New("recorder", o)

	snapshot := func() []recorded {
		mu.Lock()
		defer mu.Unlock()

		out := make([]recorded, len(records))
		copy(out, records)

		return out
	}

	return l, buf, snapshot
}

// lastRecord returns the single record snapshotted by `snapshot`, failing the
// test if there isn't exactly one.
func lastRecord(t *testing.T, snapshot func() []recorded) recorded {
	t.Helper()

	records := snapshot()

	if len(records) != 1 {
		t.Fatalf("got %d records, expected exactly 1: %+v", len(records), records)
	}

	return records[0]
}

//////
// Enabled tests.
//////

// Enabled consults the sypl outputs: a level is enabled if any enabled output
// would print it.
func TestHandler_EnabledRespectsOutputs(t *testing.T) {
	l, _, _ := newRecorderLogger(level.Info)

	h := NewHandler(l)

	ctx := context.Background()

	if !h.Enabled(ctx, slog.LevelInfo) {
		t.Fatal("Info must be enabled - the output accepts it")
	}

	if !h.Enabled(ctx, slog.LevelError) {
		t.Fatal("Error must be enabled - the output accepts it")
	}

	// In sypl's ordering, Debug is MORE verbose than Info.
	if h.Enabled(ctx, slog.LevelDebug) {
		t.Fatal("Debug must be disabled - it's beyond the output's max level")
	}
}

// A logger without outputs enables nothing.
func TestHandler_EnabledNoOutputs(t *testing.T) {
	h := NewHandler(sypl.New("no-outputs"))

	if h.Enabled(context.Background(), slog.LevelError) {
		t.Fatal("a logger without outputs must enable nothing")
	}
}

// A disabled output doesn't enable levels.
func TestHandler_EnabledDisabledOutput(t *testing.T) {
	l, _, _ := newRecorderLogger(level.Trace)

	l.GetOutput("Buffer").SetStatus(status.Disabled)

	h := NewHandler(l)

	if h.Enabled(context.Background(), slog.LevelInfo) {
		t.Fatal("a disabled output must not enable levels")
	}
}

// HandlerWithLevel sets a floor: records below it are disabled - even if the
// outputs would accept them.
func TestHandler_EnabledFloor(t *testing.T) {
	l, _, _ := newRecorderLogger(level.Trace)

	floor := &slog.LevelVar{}
	floor.Set(slog.LevelWarn)

	h := NewHandler(l, HandlerWithLevel(floor))

	ctx := context.Background()

	if h.Enabled(ctx, slog.LevelInfo) {
		t.Fatal("Info must be disabled - it's below the floor")
	}

	if !h.Enabled(ctx, slog.LevelWarn) {
		t.Fatal("Warn must be enabled - it's at the floor, and the output accepts it")
	}
}

//////
// Handle tests.
//////

// The record's message, level, and attrs land in the sypl message as content,
// mapped level, and fields - with native Go types.
func TestHandler_HandleContentLevelFields(t *testing.T) {
	l, buf, snapshot := newRecorderLogger(level.Trace)

	sl := slog.New(NewHandler(l))

	sl.Info("hello", "k", "v", "n", 42)

	rec := lastRecord(t, snapshot)

	if rec.level != level.Info {
		t.Fatalf("level = %v, expected Info", rec.level)
	}

	if rec.content != "hello" {
		t.Fatalf("content = %q, expected %q", rec.content, "hello")
	}

	if rec.fields["k"] != "v" {
		t.Fatalf(`fields["k"] = %v, expected "v"`, rec.fields["k"])
	}

	if rec.fields["n"] != int64(42) {
		t.Fatalf(`fields["n"] = %v (%T), expected int64(42)`, rec.fields["n"], rec.fields["n"])
	}

	// Each record is one line in the output.
	if buf.String() != "hello\n" {
		t.Fatalf("output = %q, expected %q", buf.String(), "hello\n")
	}
}

// Every slog level maps to the documented sypl level.
func TestHandler_HandleLevelMapping(t *testing.T) {
	l, _, snapshot := newRecorderLogger(level.Trace)

	sl := slog.New(NewHandler(l))

	ctx := context.Background()

	sl.Log(ctx, LevelTrace, "m")
	sl.Debug("m")
	sl.Info("m")
	sl.Warn("m")
	sl.Error("m")
	sl.Log(ctx, LevelFatal, "m")

	expected := []level.Level{
		level.Trace, level.Debug, level.Info, level.Warn, level.Error, level.Error,
	}

	records := snapshot()

	if len(records) != len(expected) {
		t.Fatalf("got %d records, expected %d", len(records), len(expected))
	}

	for i, e := range expected {
		if records[i].level != e {
			t.Fatalf("record %d: level = %v, expected %v", i, records[i].level, e)
		}
	}
}

// A non-zero record time is honored; a zero record time stays zero.
func TestHandler_TimestampHonored(t *testing.T) {
	l, _, snapshot := newRecorderLogger(level.Trace)

	h := NewHandler(l)

	fixed := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)

	if err := h.Handle(context.Background(), slog.NewRecord(fixed, slog.LevelInfo, "ts", 0)); err != nil {
		t.Fatalf("Handle failed: %s", err)
	}

	if rec := lastRecord(t, snapshot); !rec.timestamp.Equal(fixed) {
		t.Fatalf("timestamp = %v, expected %v", rec.timestamp, fixed)
	}
}

func TestHandler_ZeroTimestampStaysZero(t *testing.T) {
	l, _, snapshot := newRecorderLogger(level.Trace)

	h := NewHandler(l)

	if err := h.Handle(context.Background(), slog.NewRecord(time.Time{}, slog.LevelInfo, "zt", 0)); err != nil {
		t.Fatalf("Handle failed: %s", err)
	}

	if rec := lastRecord(t, snapshot); !rec.timestamp.IsZero() {
		t.Fatalf("timestamp = %v, expected the zero time", rec.timestamp)
	}
}

// A record message already ending with a linebreak isn't double-terminated.
func TestHandler_NewlinePreserved(t *testing.T) {
	l, buf, _ := newRecorderLogger(level.Trace)

	h := NewHandler(l)

	if err := h.Handle(
		context.Background(),
		slog.NewRecord(time.Now(), slog.LevelInfo, "already\n", 0),
	); err != nil {
		t.Fatalf("Handle failed: %s", err)
	}

	if buf.String() != "already\n" {
		t.Fatalf("output = %q, expected %q", buf.String(), "already\n")
	}
}

//////
// Groups, and attrs tests.
//////

// WithAttrs-accumulated attrs, and WithGroup-opened groups qualify keys as
// "group.key".
func TestHandler_WithAttrsAndGroupsFlattened(t *testing.T) {
	l, _, snapshot := newRecorderLogger(level.Trace)

	sl := slog.New(NewHandler(l))

	sl.With("base", "x").
		WithGroup("g").
		With("mid", "y").
		WithGroup("h").
		Info("msg", "leaf", "z")

	rec := lastRecord(t, snapshot)

	expected := fields.Fields{"base": "x", "g.mid": "y", "g.h.leaf": "z"}

	for k, v := range expected {
		if rec.fields[k] != v {
			t.Fatalf(`fields[%q] = %v, expected %v (fields: %+v)`, k, rec.fields[k], v, rec.fields)
		}
	}
}

// Group-valued attrs are flattened as "group.key".
func TestHandler_GroupValueFlattened(t *testing.T) {
	l, _, snapshot := newRecorderLogger(level.Trace)

	sl := slog.New(NewHandler(l))

	sl.Info("msg", slog.Group("G", slog.String("c", "d"), slog.Group("H", slog.Int("i", 1))))

	rec := lastRecord(t, snapshot)

	if rec.fields["G.c"] != "d" {
		t.Fatalf(`fields["G.c"] = %v, expected "d"`, rec.fields["G.c"])
	}

	if rec.fields["G.H.i"] != int64(1) {
		t.Fatalf(`fields["G.H.i"] = %v, expected int64(1)`, rec.fields["G.H.i"])
	}
}

// emptyGroupValuer is a LogValuer resolving to a group with no attrs.
type emptyGroupValuer struct{}

func (emptyGroupValuer) LogValue() slog.Value { return slog.GroupValue() }

// A group with no attrs is elided - even if it has a name. `slog.Logger`
// pre-filters literal empty groups, so the branch is also exercised through
// the paths that DO reach the handler: a LogValuer resolving to an empty
// group, and a direct `WithAttrs`.
func TestHandler_EmptyGroupElided(t *testing.T) {
	l, _, snapshot := newRecorderLogger(level.Trace)

	h := NewHandler(l).WithAttrs([]slog.Attr{slog.Group("G"), slog.String("a", "b")})

	sl := slog.New(h)

	sl.LogAttrs(
		context.Background(),
		slog.LevelInfo,
		"msg",
		slog.Group("H"),
		slog.Any("k", emptyGroupValuer{}),
	)

	rec := lastRecord(t, snapshot)

	if len(rec.fields) != 1 || rec.fields["a"] != "b" {
		t.Fatalf("fields = %+v, expected only a=b - empty groups elided", rec.fields)
	}
}

// A group with an empty name is inlined.
func TestHandler_AnonymousGroupInlined(t *testing.T) {
	l, _, snapshot := newRecorderLogger(level.Trace)

	sl := slog.New(NewHandler(l))

	sl.Info("msg", slog.Group("", slog.String("a", "b")))

	if rec := lastRecord(t, snapshot); rec.fields["a"] != "b" {
		t.Fatalf(`fields["a"] = %v, expected "b"`, rec.fields["a"])
	}
}

// An attr with both a zero key, and a zero value is ignored.
func TestHandler_ZeroAttrIgnored(t *testing.T) {
	l, _, snapshot := newRecorderLogger(level.Trace)

	sl := slog.New(NewHandler(l))

	sl.LogAttrs(context.Background(), slog.LevelInfo, "msg", slog.String("a", "b"), slog.Attr{})

	rec := lastRecord(t, snapshot)

	if len(rec.fields) != 1 || rec.fields["a"] != "b" {
		t.Fatalf("fields = %+v, expected only a=b", rec.fields)
	}
}

//////
// Immutable-derivation tests.
//////

// WithAttrs, and WithGroup derive NEW handlers - the receiver, and siblings
// are unaffected.
func TestHandler_ImmutableDerivation(t *testing.T) {
	l, _, snapshot := newRecorderLogger(level.Trace)

	h := NewHandler(l)

	h1 := h.WithAttrs([]slog.Attr{slog.String("one", "1")})
	h2 := h.WithAttrs([]slog.Attr{slog.String("two", "2")})

	// Two siblings derived from the SAME parent must not clobber each other.
	h11 := h1.WithAttrs([]slog.Attr{slog.String("a", "a")})
	h12 := h1.WithAttrs([]slog.Attr{slog.String("b", "b")})

	ctx := context.Background()

	slog.New(h).InfoContext(ctx, "via-h")
	slog.New(h1).InfoContext(ctx, "via-h1")
	slog.New(h2).InfoContext(ctx, "via-h2")
	slog.New(h11).InfoContext(ctx, "via-h11")
	slog.New(h12).InfoContext(ctx, "via-h12")

	records := snapshot()

	if len(records) != 5 {
		t.Fatalf("got %d records, expected 5", len(records))
	}

	expected := []fields.Fields{
		{},
		{"one": "1"},
		{"two": "2"},
		{"one": "1", "a": "a"},
		{"one": "1", "b": "b"},
	}

	for i, e := range expected {
		if len(records[i].fields) != len(e) {
			t.Fatalf("record %d: fields = %+v, expected %+v", i, records[i].fields, e)
		}

		for k, v := range e {
			if records[i].fields[k] != v {
				t.Fatalf("record %d: fields[%q] = %v, expected %v", i, k, records[i].fields[k], v)
			}
		}
	}
}

// WithAttrs with no attrs, and WithGroup with an empty name return the
// receiver.
func TestHandler_NoOpDerivationsReturnReceiver(t *testing.T) {
	l, _, _ := newRecorderLogger(level.Trace)

	h := NewHandler(l)

	if got := h.WithAttrs(nil); got != h {
		t.Fatal("WithAttrs(nil) must return the receiver")
	}

	if got := h.WithAttrs([]slog.Attr{}); got != h {
		t.Fatal("WithAttrs(empty) must return the receiver")
	}

	if got := h.WithGroup(""); got != h {
		t.Fatal(`WithGroup("") must return the receiver`)
	}
}

//////
// LogValuer tests.
//////

// stringValuer is a LogValuer resolving to a string.
type stringValuer struct{}

func (stringValuer) LogValue() slog.Value { return slog.StringValue("resolved") }

// panicValuer is a LogValuer that panics.
type panicValuer struct{}

func (panicValuer) LogValue() slog.Value { panic("boom") }

// LogValuer values are resolved.
func TestHandler_LogValuerResolved(t *testing.T) {
	l, _, snapshot := newRecorderLogger(level.Trace)

	sl := slog.New(NewHandler(l))

	sl.Info("msg", "k", stringValuer{})

	if rec := lastRecord(t, snapshot); rec.fields["k"] != "resolved" {
		t.Fatalf(`fields["k"] = %v, expected "resolved"`, rec.fields["k"])
	}
}

// A panicking LogValuer must not crash the pipeline: the recovered panic is
// stored as an error field value.
func TestHandler_LogValuerPanicIsSafe(t *testing.T) {
	l, _, snapshot := newRecorderLogger(level.Trace)

	sl := slog.New(NewHandler(l))

	sl.Info("msg", "k", panicValuer{})

	rec := lastRecord(t, snapshot)

	err, ok := rec.fields["k"].(error)

	if !ok {
		t.Fatalf(`fields["k"] = %v (%T), expected an error`, rec.fields["k"], rec.fields["k"])
	}

	if !strings.Contains(err.Error(), "panic") {
		t.Fatalf("error = %q, expected it to describe the panic", err.Error())
	}
}

//////
// Concurrency tests.
//////

// Concurrent use is race-clean, and lossless.
func TestHandler_ConcurrentUse(t *testing.T) {
	const (
		goroutines         = 8
		messagesPerRoutine = 25
	)

	l, buf, snapshot := newRecorderLogger(level.Trace)

	sl := slog.New(NewHandler(l))

	var wg sync.WaitGroup

	wg.Add(goroutines)

	for g := range goroutines {
		go func(g int) {
			defer wg.Done()

			for j := range messagesPerRoutine {
				sl.Info("concurrent", "g", g, "j", j)
			}
		}(g)
	}

	wg.Wait()

	if got := len(snapshot()); got != goroutines*messagesPerRoutine {
		t.Fatalf("got %d records, expected %d", got, goroutines*messagesPerRoutine)
	}

	lines := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")

	if len(lines) != goroutines*messagesPerRoutine {
		t.Fatalf("got %d lines, expected %d", len(lines), goroutines*messagesPerRoutine)
	}
}
