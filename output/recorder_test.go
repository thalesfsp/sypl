// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package output

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/thalesfsp/sypl/fields"
	"github.com/thalesfsp/sypl/level"
	"github.com/thalesfsp/sypl/message"
	"github.com/thalesfsp/sypl/processor"
)

//////
// Capture.
//////

func TestRecorder_CapturesStructuredSnapshots(t *testing.T) {
	recorder, o := Recorder(level.Trace, processor.Prefixer("p: "))

	if o.GetName() != "Recorder" {
		t.Errorf("GetName() = %q, want %q", o.GetName(), "Recorder")
	}

	m := message.New(level.Info, "hello\n")

	m.SetFields(fields.Fields{"fkey": "fval"})
	m.AddTags("tag-b", "tag-a")
	m.SetOutputName("Recorder")

	before := time.Now()

	if err := o.Write(m); err != nil {
		t.Fatalf("Write() error = %v, want nil", err)
	}

	if recorder.Len() != 1 {
		t.Fatalf("Len() = %d, want 1", recorder.Len())
	}

	records := recorder.Messages()

	if len(records) != 1 {
		t.Fatalf("Messages() len = %d, want 1", len(records))
	}

	record := records[0]

	if record.Level != level.Info {
		t.Errorf("Level = %v, want %v", record.Level, level.Info)
	}

	if record.OriginalContent != "hello\n" {
		t.Errorf("OriginalContent = %q, want %q", record.OriginalContent, "hello\n")
	}

	// Processed: prefixed, line break restored - exactly what was written.
	if record.ProcessedContent != "p: hello\n" {
		t.Errorf("ProcessedContent = %q, want %q", record.ProcessedContent, "p: hello\n")
	}

	if record.Fields["fkey"] != "fval" {
		t.Errorf("Fields = %v, want fkey=fval", record.Fields)
	}

	// Tags are lexicographically sorted by the message.
	if len(record.Tags) != 2 || record.Tags[0] != "tag-a" || record.Tags[1] != "tag-b" {
		t.Errorf("Tags = %v, want [tag-a tag-b]", record.Tags)
	}

	if record.OutputName != "Recorder" {
		t.Errorf("OutputName = %q, want %q", record.OutputName, "Recorder")
	}

	if len(record.ProcessorsNames) != 1 || record.ProcessorsNames[0] != "Prefixer" {
		t.Errorf("ProcessorsNames = %v, want [Prefixer]", record.ProcessorsNames)
	}

	if record.Timestamp.After(before) {
		t.Errorf("Timestamp = %v, want the message creation time", record.Timestamp)
	}
}

func TestRecorder_FallsBackToTheOutputName(t *testing.T) {
	recorder, o := Recorder(level.Trace)

	// A message written DIRECTLY - not routed by a logger - has no output
	// name: the recorder's own name is used.
	if err := o.Write(message.New(level.Info, "direct\n")); err != nil {
		t.Fatalf("Write() error = %v, want nil", err)
	}

	if records := recorder.Messages(); records[0].OutputName != "Recorder" {
		t.Errorf("OutputName = %q, want %q", records[0].OutputName, "Recorder")
	}
}

func TestRecorder_RespectsLevelGating(t *testing.T) {
	recorder, o := Recorder(level.Info)

	// Above the max level: the standard pipeline mutes it - no record.
	if err := o.Write(message.New(level.Debug, "muted\n")); err != nil {
		t.Fatalf("Write() error = %v, want nil", err)
	}

	if recorder.Len() != 0 {
		t.Errorf("Len() = %d, want 0 - level-gated messages must not be recorded", recorder.Len())
	}

	if err := o.Write(message.New(level.Error, "recorded\n")); err != nil {
		t.Fatalf("Write() error = %v, want nil", err)
	}

	if recorder.Len() != 1 {
		t.Errorf("Len() = %d, want 1", recorder.Len())
	}
}

//////
// Defensive copies.
//////

func TestRecorder_CaptureTimeCopies(t *testing.T) {
	recorder, o := Recorder(level.Trace)

	mutable := fields.Fields{"fkey": "original"}

	m := message.New(level.Info, "hello\n")

	m.SetFields(mutable)

	if err := o.Write(m); err != nil {
		t.Fatalf("Write() error = %v, want nil", err)
	}

	// Mutating the caller's map AFTER the write must not affect the store.
	mutable["fkey"] = "mutated"

	if records := recorder.Messages(); records[0].Fields["fkey"] != "original" {
		t.Errorf(`Fields["fkey"] = %v, want "original" - capture must deep copy`,
			records[0].Fields["fkey"])
	}
}

func TestRecorder_MessagesReturnsDefensiveCopies(t *testing.T) {
	recorder, o := Recorder(level.Trace)

	m := message.New(level.Info, "hello\n")

	m.SetFields(fields.Fields{"fkey": "fval"})
	m.AddTags("tag-a")

	if err := o.Write(m); err != nil {
		t.Fatalf("Write() error = %v, want nil", err)
	}

	// Mutate everything mutable on the returned records.
	tampered := recorder.Messages()

	tampered[0].Fields["fkey"] = "tampered"
	tampered[0].Tags[0] = "tampered"
	tampered[0].ProcessorsNames = append(tampered[0].ProcessorsNames, "tampered")
	tampered[0].ProcessedContent = "tampered"

	fresh := recorder.Messages()

	if fresh[0].Fields["fkey"] != "fval" {
		t.Errorf(`Fields["fkey"] = %v, want "fval" - the store was mutated`, fresh[0].Fields["fkey"])
	}

	if fresh[0].Tags[0] != "tag-a" {
		t.Errorf("Tags[0] = %q, want %q - the store was mutated", fresh[0].Tags[0], "tag-a")
	}

	if len(fresh[0].ProcessorsNames) != 0 {
		t.Errorf("ProcessorsNames = %v, want [] - the store was mutated", fresh[0].ProcessorsNames)
	}

	if fresh[0].ProcessedContent != "hello\n" {
		t.Errorf("ProcessedContent = %q, want %q", fresh[0].ProcessedContent, "hello\n")
	}
}

//////
// Reset, and Len.
//////

func TestRecorder_ResetAndLen(t *testing.T) {
	recorder, o := Recorder(level.Trace)

	for i := range 3 {
		if err := o.Write(message.New(level.Info, fmt.Sprintf("m%d\n", i))); err != nil {
			t.Fatalf("Write() error = %v, want nil", err)
		}
	}

	if recorder.Len() != 3 {
		t.Fatalf("Len() = %d, want 3", recorder.Len())
	}

	recorder.Reset()

	if recorder.Len() != 0 {
		t.Errorf("Len() after Reset = %d, want 0", recorder.Len())
	}

	if records := recorder.Messages(); len(records) != 0 {
		t.Errorf("Messages() after Reset = %v, want empty", records)
	}
}

//////
// Robustness.
//////

func TestRecorder_DirectWriterWritesAreNotRecorded(t *testing.T) {
	recorder, o := Recorder(level.Trace)

	// Writing to the exposed writer directly - outside the message
	// pipeline - has no message to snapshot: skipped, never a panic.
	if _, err := o.GetWriter().Write([]byte("raw bytes")); err != nil {
		t.Fatalf("Write() error = %v, want nil", err)
	}

	if recorder.Len() != 0 {
		t.Errorf("Len() = %d, want 0 - raw writes carry no message", recorder.Len())
	}
}

func TestRecorder_ConcurrentWriters(t *testing.T) {
	recorder, o := Recorder(level.Trace)

	const (
		writers           = 8
		messagesPerWriter = 25
	)

	var wg sync.WaitGroup

	for w := range writers {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for i := range messagesPerWriter {
				m := message.New(level.Info, fmt.Sprintf("w%d-m%d\n", w, i))

				m.SetFields(fields.Fields{"writer": w})

				if err := o.Write(m); err != nil {
					t.Errorf("Write() error = %v, want nil", err)
				}
			}
		}()
	}

	wg.Wait()

	if recorder.Len() != writers*messagesPerWriter {
		t.Errorf("Len() = %d, want %d", recorder.Len(), writers*messagesPerWriter)
	}
}
