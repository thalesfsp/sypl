// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

// End-to-end mocked pipeline tests for the reliability outputs: a real
// Sypl logger - fields/tags merge, concurrent dispatch - driving the
// async wrapper, the rotating file, the recorder, and the ElasticSearch
// bulk output.
//
// NOTE: External test package - importing sypl from `package output` would
// be an import cycle.
package output_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/thalesfsp/sypl"
	"github.com/thalesfsp/sypl/elasticsearch"
	"github.com/thalesfsp/sypl/fields"
	"github.com/thalesfsp/sypl/level"
	"github.com/thalesfsp/sypl/output"
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

//////
// ElasticSearchBulk.
//////

func TestE2E_ElasticSearchBulkThroughSypl(t *testing.T) {
	const esInfoBody = `{
		"name": "fake-node",
		"cluster_name": "fake-cluster",
		"cluster_uuid": "abc123",
		"version": {"number": "8.16.0", "build_flavor": "default"},
		"tagline": "You Know, for Search"
	}`

	var (
		mu     sync.Mutex
		bodies []string
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		w.Header().Set("Content-Type", "application/json")

		if r.URL.Path == "/" {
			fmt.Fprint(w, esInfoBody)

			return
		}

		body, _ := io.ReadAll(r.Body)

		mu.Lock()
		bodies = append(bodies, string(body))
		mu.Unlock()

		lines := strings.Count(strings.TrimSuffix(string(body), "\n"), "\n") + 1

		items := []string{}

		for range lines / 2 {
			items = append(items, `{"index":{"_index":"idx","status":201,"result":"created"}}`)
		}

		fmt.Fprintf(w, `{"took":1,"errors":false,"items":[%s]}`, strings.Join(items, ","))
	}))

	t.Cleanup(srv.Close)

	o := output.ElasticSearchBulk(
		"idx-e2e",
		output.ElasticSearchConfig{Addresses: []string{srv.URL}},
		level.Trace,
		[]output.ElasticSearchBulkOption{elasticsearch.BulkWithNumWorkers(1)},
	)

	l := sypl.New("e2e-bulk", o)

	const total = 10

	for i := range total {
		l.Infolnf("bulk-m%02d", i)
	}

	if err := flushCapability(t, o); err != nil {
		t.Fatalf("Flush() error = %v, want nil", err)
	}

	mu.Lock()
	all := strings.Join(bodies, "")
	mu.Unlock()

	// Every message was indexed - as single-line JSON documents, into the
	// right index.
	indexed := strings.Count(all, `"_index":"idx-e2e"`)

	if indexed != total {
		t.Fatalf("Indexed %d documents, want %d: %q", indexed, total, all)
	}

	for i := range total {
		if !strings.Contains(all, fmt.Sprintf(`"message":"bulk-m%02d"`, i)) {
			t.Errorf("Document bulk-m%02d wasn't indexed", i)
		}
	}

	if err := closeCapability(t, o); err != nil {
		t.Fatalf("Close() error = %v, want nil", err)
	}
}
