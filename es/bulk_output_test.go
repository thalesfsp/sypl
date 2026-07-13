// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package es

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/thalesfsp/sypl/v2/level"
	"github.com/thalesfsp/sypl/v2/message"
	"github.com/thalesfsp/sypl/v2/output"
	"github.com/thalesfsp/sypl/v2/processor"
)

//////
// Test helpers.
//////

// bulkRecorder captures the NDJSON payloads of every _bulk request the fake
// Elasticsearch server receives.
type bulkRecorder struct {
	mu     sync.Mutex
	bodies []string
}

func (br *bulkRecorder) add(body string) {
	br.mu.Lock()
	defer br.mu.Unlock()

	br.bodies = append(br.bodies, body)
}

func (br *bulkRecorder) all() []string {
	br.mu.Lock()
	defer br.mu.Unlock()

	out := make([]string, len(br.bodies))
	copy(out, br.bodies)

	return out
}

// items returns every indexed document - action, and source line pairs -
// across all captured _bulk requests.
func (br *bulkRecorder) items() [][2]string {
	out := [][2]string{}

	for _, body := range br.all() {
		lines := strings.Split(strings.TrimSuffix(body, "\n"), "\n")

		for i := 0; i+1 < len(lines); i += 2 {
			out = append(out, [2]string{lines[i], lines[i+1]})
		}
	}

	return out
}

// newFakeBulkESServer starts a fake Elasticsearch answering the Info ping,
// and recording _bulk requests - always reporting every item as created.
func newFakeBulkESServer(t *testing.T) (*httptest.Server, *bulkRecorder) {
	t.Helper()

	recorder := &bulkRecorder{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		w.Header().Set("Content-Type", "application/json")

		if r.URL.Path == "/" {
			fmt.Fprint(w, esInfoBody)

			return
		}

		body, _ := io.ReadAll(r.Body)

		recorder.add(string(body))

		lines := strings.Count(strings.TrimSuffix(string(body), "\n"), "\n") + 1

		items := []string{}

		for range lines / 2 {
			items = append(items, `{"index":{"_index":"idx","status":201,"result":"created"}}`)
		}

		fmt.Fprintf(w, `{"took":1,"errors":false,"items":[%s]}`, strings.Join(items, ","))
	}))

	t.Cleanup(srv.Close)

	return srv, recorder
}

// singleWorker keeps batching deterministic.
func singleWorker() []BulkOption {
	return []BulkOption{BulkWithNumWorkers(1)}
}

// flushOutput flushes `o` via the Flush capability, failing the test if the
// capability is missing.
func flushOutput(t *testing.T, o output.IOutput) error {
	t.Helper()

	f, ok := o.(interface{ Flush() error })

	if !ok {
		t.Fatal("The bulk output should implement Flush() error")
	}

	return f.Flush()
}

// closeOutput closes `o` via the Close capability, failing the test if the
// capability is missing.
func closeOutput(t *testing.T, o output.IOutput) error {
	t.Helper()

	c, ok := o.(interface{ Close() error })

	if !ok {
		t.Fatal("The bulk output should implement Close() error")
	}

	return c.Close()
}

//////
// ElasticSearchBulk.
//////

func TestElasticSearchBulkOutput(t *testing.T) {
	srv, recorder := newFakeBulkESServer(t)

	o := BulkOutput("idx-bulk", Config{
		Addresses: []string{srv.URL},
	}, level.Trace, singleWorker())

	if o.GetName() != "ElasticSearchBulk" {
		t.Errorf("GetName() = %q, want %q", o.GetName(), "ElasticSearchBulk")
	}

	if o.GetMaxLevel() != level.Trace {
		t.Errorf("GetMaxLevel() = %v, want %v", o.GetMaxLevel(), level.Trace)
	}

	// Data is JSON-formatted by default. NOTE: INLINE JSON - not
	// JSONPretty like the sync factory: the _bulk NDJSON protocol
	// requires each document on a single line.
	if o.GetFormatter() == nil || o.GetFormatter().GetName() != "JSON" {
		t.Errorf("GetFormatter() = %v, want the inline JSON formatter", o.GetFormatter())
	}

	if err := o.Write(message.New(level.Info, "bulk message")); err != nil {
		t.Fatalf("Write() error = %v, want nil", err)
	}

	if err := flushOutput(t, o); err != nil {
		t.Fatalf("Flush() error = %v, want nil", err)
	}

	items := recorder.items()

	if len(items) != 1 {
		t.Fatalf("Expected 1 indexed item, got %d", len(items))
	}

	if !strings.Contains(items[0][0], `"_index":"idx-bulk"`) {
		t.Errorf("Action line = %q, want it to index into idx-bulk", items[0][0])
	}

	// The indexed document is the JSON-formatted message.
	parsed := map[string]interface{}{}

	if err := json.Unmarshal([]byte(items[0][1]), &parsed); err != nil {
		t.Fatalf("Indexed body isn't valid JSON: %v", err)
	}

	if parsed["message"] != "bulk message" {
		t.Errorf(`Indexed body message = %v, want "bulk message"`, parsed["message"])
	}

	if err := closeOutput(t, o); err != nil {
		t.Fatalf("Close() error = %v, want nil", err)
	}
}

func TestElasticSearchBulkOutput_LevelGating(t *testing.T) {
	srv, recorder := newFakeBulkESServer(t)

	o := BulkOutput("idx-bulk", Config{
		Addresses: []string{srv.URL},
	}, level.Info, singleWorker())

	// Above the max level: gated by the standard pipeline - never
	// enqueued.
	if err := o.Write(message.New(level.Debug, "muted")); err != nil {
		t.Fatalf("Write() error = %v, want nil", err)
	}

	if err := closeOutput(t, o); err != nil {
		t.Fatalf("Close() error = %v, want nil", err)
	}

	if items := recorder.items(); len(items) != 0 {
		t.Errorf("Expected no indexed items, got %d", len(items))
	}
}

//////
// ElasticSearchBulkWithDynamicIndex.
//////

func TestElasticSearchBulkWithDynamicIndexOutput(t *testing.T) {
	srv, recorder := newFakeBulkESServer(t)

	day := testDay1

	o := BulkOutputWithDynamicIndex(
		func() string { return "idx-" + day },
		Config{Addresses: []string{srv.URL}},
		level.Trace,
		singleWorker(),
	)

	if o.GetName() != "ElasticSearchBulkWithDynamicIndex-idx-2026-07-12" {
		t.Errorf("GetName() = %q, want %q",
			o.GetName(), "ElasticSearchBulkWithDynamicIndex-idx-2026-07-12")
	}

	if err := o.Write(message.New(level.Info, "first")); err != nil {
		t.Fatalf("Write() error = %v, want nil", err)
	}

	// The index name is evaluated at write time - a change must be
	// reflected on the next write.
	day = testDay2

	if err := o.Write(message.New(level.Info, "second")); err != nil {
		t.Fatalf("Write() error = %v, want nil", err)
	}

	if err := closeOutput(t, o); err != nil {
		t.Fatalf("Close() error = %v, want nil", err)
	}

	items := recorder.items()

	if len(items) != 2 {
		t.Fatalf("Expected 2 indexed items, got %d", len(items))
	}

	if !strings.Contains(items[0][0], `"_index":"idx-2026-07-12"`) {
		t.Errorf("Item 0 action = %q, want idx-2026-07-12", items[0][0])
	}

	if !strings.Contains(items[1][0], `"_index":"idx-2026-07-13"`) {
		t.Errorf("Item 1 action = %q, want idx-2026-07-13", items[1][0])
	}
}

//////
// Processor aliasing.
//////

// The processors slice passed by the caller must never be aliased - the
// sync factories' audit-established guarantee holds for the bulk ones too.
func TestElasticSearchBulkOutput_NoProcessorsAliasing(t *testing.T) {
	srv, _ := newFakeBulkESServer(t)

	// Spare capacity is the trigger: the passed slice (len 1) shares its
	// backing array with `backing` (len 2) - a sentinel occupies the very
	// slot a spare-capacity append would overwrite.
	backing := make([]processor.IProcessor, 2, 8)

	backing[0] = processor.Prefixer("shared-prefix: ")

	sentinel := processor.Prefixer("sentinel: ")

	backing[1] = sentinel

	o := BulkOutput("idx-bulk", Config{
		Addresses: []string{srv.URL},
	}, level.Trace, singleWorker(), backing[:1]...)

	defer func() { _ = closeOutput(t, o) }()

	// The output carries the caller's processor.
	if findProcessor(o, prefixerName) == nil {
		t.Error("The bulk output should carry the given processors")
	}

	// Growing the output's processors must not write into the caller's
	// backing array.
	o.AddProcessors(processor.Suffixer(" s"))

	if backing[0].GetName() != prefixerName {
		t.Errorf("The caller's processors slice was mutated: %v", backing)
	}

	if backing[1] != sentinel {
		t.Error("The output aliased the caller's backing array - the sentinel was overwritten")
	}
}
