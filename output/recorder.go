// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package output

import (
	"slices"
	"sync"
	"time"

	"github.com/thalesfsp/sypl/fields"
	"github.com/thalesfsp/sypl/level"
	"github.com/thalesfsp/sypl/message"
	"github.com/thalesfsp/sypl/processor"
)

//////
// Consts, vars, and types.
//////

// Record is a structured snapshot of a message the recorder output actually
// wrote - level gating, flags, and processors all applied.
type Record struct {
	// Fields is a copy of the message's structured fields.
	Fields fields.Fields

	// Level is the message's level.
	Level level.Level

	// OriginalContent is the message's content before processing.
	OriginalContent string

	// OutputName is the output the message was routed to.
	OutputName string

	// ProcessedContent is the exact text written - processors, and
	// formatter applied.
	ProcessedContent string

	// ProcessorsNames is a copy of the processors names applied.
	ProcessorsNames []string

	// Tags is a copy of the message's tags - lexicographically sorted.
	Tags []string

	// Timestamp is the message's creation time.
	Timestamp time.Time
}

// copyRecord deep copies a record's mutable parts.
func copyRecord(r Record) Record {
	r.Fields = fields.Copy(r.Fields, fields.Fields{})
	r.ProcessorsNames = slices.Clone(r.ProcessorsNames)
	r.Tags = slices.Clone(r.Tags)

	return r
}

// RecorderOutput is an `IOutput` capturing a structured snapshot of every
// message it writes - a test-assertion helper for Sypl consumers.
type RecorderOutput struct {
	*proxyOutput

	// writeMu serializes Write, guarding `pending` - the message currently
	// traversing the inner pipeline.
	writeMu sync.Mutex
	pending message.IMessage

	// recMu guards the captured records.
	recMu   sync.Mutex
	records []Record
}

//////
// Methods.
//////

// Write runs the message through the standard output pipeline - snapshotting
// it if, and only if, it's actually written.
func (r *RecorderOutput) Write(m message.IMessage) error {
	r.writeMu.Lock()
	defer r.writeMu.Unlock()

	r.pending = m

	defer func() { r.pending = nil }()

	return r.inner.Write(m)
}

// Messages returns a defensive copy of the captured records.
func (r *RecorderOutput) Messages() []Record {
	r.recMu.Lock()
	defer r.recMu.Unlock()

	out := make([]Record, len(r.records))

	for i, record := range r.records {
		out[i] = copyRecord(record)
	}

	return out
}

// Len returns how many records were captured.
func (r *RecorderOutput) Len() int {
	r.recMu.Lock()
	defer r.recMu.Unlock()

	return len(r.records)
}

// Reset discards the captured records.
func (r *RecorderOutput) Reset() {
	r.recMu.Lock()
	defer r.recMu.Unlock()

	r.records = nil
}

//////
// Helpers.
//////

// capture snapshots the pending message. `processed` is the exact text the
// pipeline wrote.
func (r *RecorderOutput) capture(processed string) {
	// Reading `pending` is safe: capture runs downstream of Write - on the
	// same goroutine - while `writeMu` is held.
	m := r.pending

	// A raw write to the exposed writer - outside the message pipeline -
	// carries no message: nothing to snapshot.
	if m == nil {
		return
	}

	record := Record{
		Fields:           fields.Copy(m.GetFields(), fields.Fields{}),
		Level:            m.GetLevel(),
		OriginalContent:  m.GetContent().GetOriginal(),
		OutputName:       m.GetOutputName(),
		ProcessedContent: processed,
		ProcessorsNames:  slices.Clone(m.GetProcessorsNames()),
		Tags:             m.GetTags(),
		Timestamp:        m.GetTimestamp(),
	}

	// Messages written directly - not routed by a logger - carry no
	// output name: fall back to the recorder's.
	if record.OutputName == "" {
		record.OutputName = r.GetName()
	}

	r.recMu.Lock()
	defer r.recMu.Unlock()

	r.records = append(r.records, record)
}

// recorderWriter snapshots everything the pipeline writes.
type recorderWriter struct {
	r *RecorderOutput
}

// Write conforms to the `io.Writer` interface.
func (w recorderWriter) Write(p []byte) (int, error) {
	w.r.capture(string(p))

	return len(p), nil
}

//////
// Factory.
//////

// Recorder is a built-in `output` - named `Recorder` - capturing a
// structured snapshot (level, original, and processed content, fields,
// tags, output, and processors names, timestamp) of every message it
// actually writes: level gating, flags, and processors are applied through
// the standard pipeline first. Designed for test assertions by Sypl
// consumers.
//
// It returns both the concrete recorder - for `Messages`, `Len`, and
// `Reset` - and the `IOutput` to register on the logger; they're the same
// instance. Thread-safe.
func Recorder(maxLevel level.Level, processors ...processor.IProcessor) (*RecorderOutput, IOutput) {
	r := &RecorderOutput{}

	r.proxyOutput = newProxyOutput(
		New("Recorder", maxLevel, recorderWriter{r: r}, processors...),
		r,
	)

	return r, r
}
