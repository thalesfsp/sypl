// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

// End-to-end mocked pipeline tests for the reliability outputs: a real
// Sypl logger - fields/tags merge, concurrent dispatch - driving the
// async wrapper, the rotating file, and the recorder.
//
// NOTE: External test package - importing sypl from `package output` would
// be an import cycle.
package output_test

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/thalesfsp/sypl/v2"
	"github.com/thalesfsp/sypl/v2/fields"
	"github.com/thalesfsp/sypl/v2/level"
	"github.com/thalesfsp/sypl/v2/output"
)

//////
// Test helpers.
//////

// flushCapability drains `o` via the frozen Flush contract.
func flushCapability(t *testing.T, o output.IOutput) error {
	t.Helper()

	f, ok := o.(interface{ Flush() error })

	if !ok {
		t.Fatalf("Output %q should implement Flush() error", o.GetName())
	}

	return f.Flush()
}

// closeCapability closes `o` via the frozen Close contract.
func closeCapability(t *testing.T, o output.IOutput) error {
	t.Helper()

	c, ok := o.(io.Closer)

	if !ok {
		t.Fatalf("Output %q should implement io.Closer", o.GetName())
	}

	return c.Close()
}

//////
// Async.
//////

func TestE2E_AsyncOutputThroughSypl(t *testing.T) {
	buf, inner := output.SafeBuffer(level.Trace)

	a := output.Async(inner)

	l := sypl.New("e2e-async", a)

	const total = 50

	for i := range total {
		l.Infolnf("m%02d", i)
	}

	if err := flushCapability(t, a); err != nil {
		t.Fatalf("Flush() error = %v, want nil", err)
	}

	// Every line is present after Flush.
	for i := range total {
		if !strings.Contains(buf.String(), fmt.Sprintf("m%02d\n", i)) {
			t.Fatalf("Buffer is missing m%02d: %q", i, buf.String())
		}
	}

	// Sypl-level dispatch works through the wrapper: name-based routing,
	// and SetMaxLevel reach the inner output.
	//
	// NOTE: Level gating runs at DRAIN time - inside the inner output's
	// Write - so the queue is flushed before the max level changes.
	l.PrintWithOptions(level.Info, "routed\n", sypl.WithOutputsNames("Buffer"))

	if err := flushCapability(t, a); err != nil {
		t.Fatalf("Flush() error = %v, want nil", err)
	}

	l.SetMaxLevel(level.Error)

	l.Infoln("must be muted")

	if err := flushCapability(t, a); err != nil {
		t.Fatalf("Flush() error = %v, want nil", err)
	}

	if !strings.Contains(buf.String(), "routed\n") {
		t.Error("Name-based routing should reach the wrapped output")
	}

	if strings.Contains(buf.String(), "must be muted") {
		t.Error("SetMaxLevel through the logger should reach the wrapped output")
	}

	if err := closeCapability(t, a); err != nil {
		t.Fatalf("Close() error = %v, want nil", err)
	}
}

//////
// RotatingFile.
//////

func TestE2E_RotatingFileThroughSypl(t *testing.T) {
	path := filepath.Join(t.TempDir(), "e2e.log")

	o, err := output.RotatingFile("RotatingFile", path, level.Trace, output.RotationConfig{
		MaxSizeBytes: 64,
	})
	if err != nil {
		t.Fatalf("RotatingFile() error = %v, want nil", err)
	}

	l := sypl.New("e2e-rotate", o)

	// Concurrent writers + Flush: rotation must be safe vs. Flush.
	const (
		writers           = 4
		messagesPerWriter = 20
	)

	var wg sync.WaitGroup

	for w := range writers {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for i := range messagesPerWriter {
				l.Infolnf("w%d-m%02d", w, i)

				if i%5 == 0 {
					if err := flushCapability(t, o); err != nil {
						t.Errorf("Flush() error = %v, want nil", err)
					}
				}
			}
		}()
	}

	wg.Wait()

	if err := closeCapability(t, o); err != nil {
		t.Fatalf("Close() error = %v, want nil", err)
	}

	// No message lost across the live file, and every backup.
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatalf("Failed listing the log dir: %v", err)
	}

	all := strings.Builder{}

	for _, entry := range entries {
		content, err := os.ReadFile(filepath.Join(filepath.Dir(path), entry.Name()))
		if err != nil {
			t.Fatalf("Failed reading %q: %v", entry.Name(), err)
		}

		all.Write(content)
	}

	// Rotation happened - otherwise the test proves nothing.
	if len(entries) < 2 {
		t.Errorf("Expected the live file plus backups, got %d files", len(entries))
	}

	for w := range writers {
		for i := range messagesPerWriter {
			if !strings.Contains(all.String(), fmt.Sprintf("w%d-m%02d\n", w, i)) {
				t.Fatalf("Message w%d-m%02d was lost across rotation", w, i)
			}
		}
	}
}

//////
// Recorder.
//////

func TestE2E_RecorderThroughSypl(t *testing.T) {
	recorder, o := output.Recorder(level.Trace)

	l := sypl.New("e2e-recorder", o)

	// Global fields merge with per-message fields; tags flow through.
	l.SetFields(fields.Fields{"global": "gval"})

	l.PrintWithOptions(
		level.Warn,
		"recorded\n",
		sypl.WithFields(fields.Fields{"local": "lval"}),
		sypl.WithTags("tag-a"),
	)

	if recorder.Len() != 1 {
		t.Fatalf("Len() = %d, want 1", recorder.Len())
	}

	record := recorder.Messages()[0]

	if record.Level != level.Warn {
		t.Errorf("Level = %v, want %v", record.Level, level.Warn)
	}

	if record.OriginalContent != "recorded\n" {
		t.Errorf("OriginalContent = %q, want %q", record.OriginalContent, "recorded\n")
	}

	if record.ProcessedContent != "recorded\n" {
		t.Errorf("ProcessedContent = %q, want %q", record.ProcessedContent, "recorded\n")
	}

	if record.Fields["global"] != "gval" || record.Fields["local"] != "lval" {
		t.Errorf("Fields = %v, want the global, and local fields merged", record.Fields)
	}

	if len(record.Tags) != 1 || record.Tags[0] != "tag-a" {
		t.Errorf("Tags = %v, want [tag-a]", record.Tags)
	}

	if record.OutputName != "Recorder" {
		t.Errorf("OutputName = %q, want %q", record.OutputName, "Recorder")
	}
}
