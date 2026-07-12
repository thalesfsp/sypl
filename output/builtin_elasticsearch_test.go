// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package output

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/thalesfsp/sypl/flag"
	"github.com/thalesfsp/sypl/level"
	"github.com/thalesfsp/sypl/message"
	"github.com/thalesfsp/sypl/processor"
)

//////
// Test helpers.
//////

// esInfoBody is a minimal Elasticsearch Info (ping) response - the v8
// client validates it on the first request.
const esInfoBody = `{
	"name": "fake-node",
	"cluster_name": "fake-cluster",
	"cluster_uuid": "abc123",
	"version": {"number": "8.16.0", "build_flavor": "default"},
	"tagline": "You Know, for Search"
}`

// esRecorder captures the index name, and body of every document-index
// request the fake Elasticsearch server receives.
type esRecorder struct {
	mu       sync.Mutex
	indexed  []string
	payloads []string
}

func (er *esRecorder) add(index, payload string) {
	er.mu.Lock()
	defer er.mu.Unlock()

	er.indexed = append(er.indexed, index)
	er.payloads = append(er.payloads, payload)
}

func (er *esRecorder) indexes() []string {
	er.mu.Lock()
	defer er.mu.Unlock()

	out := make([]string, len(er.indexed))
	copy(out, er.indexed)

	return out
}

func (er *esRecorder) bodies() []string {
	er.mu.Lock()
	defer er.mu.Unlock()

	out := make([]string, len(er.payloads))
	copy(out, er.payloads)

	return out
}

func (er *esRecorder) reset() {
	er.mu.Lock()
	defer er.mu.Unlock()

	er.indexed = nil
	er.payloads = nil
}

// newFakeESServer starts a fake Elasticsearch answering the Info ping, and
// recording index requests - always reporting the document as created.
func newFakeESServer(t *testing.T) (*httptest.Server, *esRecorder) {
	t.Helper()

	recorder := &esRecorder{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		w.Header().Set("Content-Type", "application/json")

		if r.URL.Path == "/" {
			fmt.Fprint(w, esInfoBody)

			return
		}

		// Document index requests look like /{index}/_doc[/{id}].
		body, _ := io.ReadAll(r.Body)

		segments := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")

		recorder.add(segments[0], string(body))

		fmt.Fprint(w, `{"result":"created"}`)
	}))

	t.Cleanup(srv.Close)

	return srv, recorder
}

//////
// NewElasticSearchTagMapItem.
//////

func TestNewElasticSearchTagMapItem(t *testing.T) {
	item := NewElasticSearchTagMapItem(level.Debug, func() string { return "idx-item" })

	if item.Level != level.Debug {
		t.Errorf("Level = %v, want %v", item.Level, level.Debug)
	}

	if item.DynamicIndexFunc == nil {
		t.Fatal("DynamicIndexFunc should be set")
	}

	if got := item.DynamicIndexFunc(); got != "idx-item" {
		t.Errorf("DynamicIndexFunc() = %q, want %q", got, "idx-item")
	}
}

//////
// ElasticSearch.
//////

func TestElasticSearchOutput(t *testing.T) {
	srv, recorder := newFakeESServer(t)

	o := ElasticSearch("idx-plain", ElasticSearchConfig{
		Addresses: []string{srv.URL},
	}, level.Trace)

	if o.GetName() != "ElasticSearch" {
		t.Errorf("GetName() = %q, want %q", o.GetName(), "ElasticSearch")
	}

	if o.GetMaxLevel() != level.Trace {
		t.Errorf("GetMaxLevel() = %v, want %v", o.GetMaxLevel(), level.Trace)
	}

	// Data is JSON-formatted by default.
	if o.GetFormatter() == nil || o.GetFormatter().GetName() != "JSONPretty" {
		t.Errorf("GetFormatter() = %v, want the JSONPretty formatter", o.GetFormatter())
	}

	if err := o.Write(message.New(level.Info, "es message")); err != nil {
		t.Fatalf("Write() error = %v, want nil", err)
	}

	indexes := recorder.indexes()

	if len(indexes) != 1 || indexes[0] != "idx-plain" {
		t.Fatalf("Indexed into %v, want [idx-plain]", indexes)
	}

	// The indexed document is the JSON-formatted message.
	parsed := map[string]interface{}{}

	if err := json.Unmarshal([]byte(recorder.bodies()[0]), &parsed); err != nil {
		t.Fatalf("Indexed body isn't valid JSON: %v", err)
	}

	if parsed["message"] != "es message" {
		t.Errorf(`Indexed body message = %v, want "es message"`, parsed["message"])
	}
}

//////
// ElasticSearchWithDynamicIndex.
//////

func TestElasticSearchWithDynamicIndexOutput(t *testing.T) {
	srv, recorder := newFakeESServer(t)

	day := "2026-07-11"

	o := ElasticSearchWithDynamicIndex(
		func() string { return "idx-" + day },
		ElasticSearchConfig{Addresses: []string{srv.URL}},
		level.Trace,
	)

	if o.GetName() != "ElasticSearchWithDynamicIndex-idx-2026-07-11" {
		t.Errorf("GetName() = %q, want %q", o.GetName(), "ElasticSearchWithDynamicIndex-idx-2026-07-11")
	}

	if err := o.Write(message.New(level.Info, "first")); err != nil {
		t.Fatalf("Write() error = %v, want nil", err)
	}

	// The index name is evaluated at index time - a change must be
	// reflected on the next write.
	day = "2026-07-12"

	if err := o.Write(message.New(level.Info, "second")); err != nil {
		t.Fatalf("Write() error = %v, want nil", err)
	}

	indexes := recorder.indexes()

	if len(indexes) != 2 || indexes[0] != "idx-2026-07-11" || indexes[1] != "idx-2026-07-12" {
		t.Errorf("Indexed into %v, want [idx-2026-07-11 idx-2026-07-12]", indexes)
	}
}

//////
// ElasticSearchWithTagMap.
//////

func tagMapForTest(withCatchAll bool) ElasticSearchTagMap {
	tagMap := ElasticSearchTagMap{
		"alpha": NewElasticSearchTagMapItem(level.Trace, func() string { return "idx-alpha" }),
		"beta":  NewElasticSearchTagMapItem(level.Trace, func() string { return "idx-beta" }),
	}

	if withCatchAll {
		tagMap["*"] = NewElasticSearchTagMapItem(level.Trace, func() string { return "idx-catchall" })
	}

	return tagMap
}

// findOutput returns the output with the given name - nil if absent.
func findOutput(outputs []IOutput, name string) IOutput {
	for _, o := range outputs {
		if o.GetName() == name {
			return o
		}
	}

	return nil
}

func TestElasticSearchWithTagMapOutput_Naming(t *testing.T) {
	srv, _ := newFakeESServer(t)

	outputs := ElasticSearchWithTagMap(
		tagMapForTest(true),
		ElasticSearchConfig{Addresses: []string{srv.URL}},
	)

	if len(outputs) != 3 {
		t.Fatalf("Expected 3 outputs, got %d", len(outputs))
	}

	for _, name := range []string{
		"ElasticSearchWithTagMap-alpha",
		"ElasticSearchWithTagMap-beta",
		"ElasticSearchWithTagMap-*",
	} {
		o := findOutput(outputs, name)

		if o == nil {
			t.Errorf("Missing output %q", name)

			continue
		}

		if o.GetFormatter() == nil || o.GetFormatter().GetName() != "JSONPretty" {
			t.Errorf("Output %q should have the JSONPretty formatter", name)
		}
	}

	// Tagged outputs filter by tag; the catch-all filters by NOT having
	// any mapped tag.
	if findOutput(outputs, "ElasticSearchWithTagMap-alpha").GetProcessor("PrintOnlyIfTagged") == nil {
		t.Error("Tagged output should have the PrintOnlyIfTagged processor")
	}

	if findOutput(outputs, "ElasticSearchWithTagMap-*").GetProcessor("PrintOnlyIfNotTaggedWith") == nil {
		t.Error("Catch-all output should have the PrintOnlyIfNotTaggedWith processor")
	}
}

func TestElasticSearchWithTagMapOutput_Routing(t *testing.T) {
	srv, recorder := newFakeESServer(t)

	outputs := ElasticSearchWithTagMap(
		tagMapForTest(true),
		ElasticSearchConfig{Addresses: []string{srv.URL}},
	)

	// Emulates the logger: every output gets its own copy of the event.
	writeToAll := func(t *testing.T, tags ...string) {
		t.Helper()

		for _, o := range outputs {
			m := message.New(level.Info, "routed message")

			m.AddTags(tags...)

			if err := o.Write(m); err != nil {
				t.Fatalf("Write() to %q error = %v, want nil", o.GetName(), err)
			}
		}
	}

	tests := []struct {
		name        string
		tags        []string
		wantIndexes map[string]bool
	}{
		{
			name:        "Should route - tagged message goes only to its index",
			tags:        []string{"alpha"},
			wantIndexes: map[string]bool{"idx-alpha": true},
		},
		{
			name:        "Should route - multi-tagged message goes to every matching index",
			tags:        []string{"alpha", "beta"},
			wantIndexes: map[string]bool{"idx-alpha": true, "idx-beta": true},
		},
		{
			name:        "Should route - untagged message goes only to the catch-all",
			tags:        nil,
			wantIndexes: map[string]bool{"idx-catchall": true},
		},
		{
			name:        "Should route - unmapped tag goes only to the catch-all",
			tags:        []string{"gamma"},
			wantIndexes: map[string]bool{"idx-catchall": true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder.reset()

			writeToAll(t, tt.tags...)

			got := map[string]bool{}

			for _, index := range recorder.indexes() {
				got[index] = true
			}

			if len(got) != len(tt.wantIndexes) {
				t.Fatalf("Indexed into %v, want %v", recorder.indexes(), tt.wantIndexes)
			}

			for index := range tt.wantIndexes {
				if !got[index] {
					t.Errorf("Missing index %q, indexed into %v", index, recorder.indexes())
				}
			}
		})
	}
}

// The processors slice passed by the caller must never be aliased - with
// spare capacity, `append` would otherwise make every output share (and
// overwrite) the same backing array, wiring the LAST tag's filter into
// EVERY output.
func TestElasticSearchWithTagMapOutput_NoProcessorsAliasing(t *testing.T) {
	srv, _ := newFakeESServer(t)

	// Spare capacity is the trigger: len 1, cap 8.
	processors := make([]processor.IProcessor, 1, 8)

	processors[0] = processor.Prefixer("shared-prefix: ")

	outputs := ElasticSearchWithTagMap(
		tagMapForTest(false),
		ElasticSearchConfig{Addresses: []string{srv.URL}},
		processors...,
	)

	if len(outputs) != 2 {
		t.Fatalf("Expected 2 outputs, got %d", len(outputs))
	}

	// The caller's slice must be untouched.
	if len(processors) != 1 || processors[0].GetName() != "Prefixer" {
		t.Errorf("The caller's processors slice was mutated: %v", processors)
	}

	// Each output must hold ITS OWN tag filter: its filter must pass a
	// message carrying the output's own tag, and mute one carrying only
	// the other output's tag.
	for _, tag := range []string{"alpha", "beta"} {
		o := findOutput(outputs, "ElasticSearchWithTagMap-"+tag)

		if o == nil {
			t.Fatalf("Missing output for tag %q", tag)
		}

		// The shared processor must also be there.
		if o.GetProcessor("Prefixer") == nil {
			t.Errorf("Output %q should carry the shared Prefixer", o.GetName())
		}

		filter := o.GetProcessor("PrintOnlyIfTagged")

		if filter == nil {
			t.Fatalf("Output %q is missing its tag filter", o.GetName())
		}

		// Own tag: not muted.
		own := message.New(level.Info, "own")

		own.AddTags(tag)

		if err := filter.Run(own); err != nil {
			t.Fatalf("Run failed: %s", err)
		}

		if own.GetFlag() == flag.Mute {
			t.Errorf("Output %q muted a message with its OWN tag %q - filters are aliased",
				o.GetName(), tag)
		}

		// Other tag only: muted.
		otherTag := "alpha"

		if tag == "alpha" {
			otherTag = "beta"
		}

		other := message.New(level.Info, "other")

		other.AddTags(otherTag)

		if err := filter.Run(other); err != nil {
			t.Fatalf("Run failed: %s", err)
		}

		if other.GetFlag() != flag.Mute {
			t.Errorf("Output %q should mute a message tagged only %q", o.GetName(), otherTag)
		}
	}
}
