// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

// End-to-end mocked pipeline test: a real Sypl logger - fields/tags merge,
// concurrent dispatch - driving the ElasticSearch bulk output.
//
// NOTE: External test package - exercising the es module exactly like a
// consumer does.
package es_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/thalesfsp/sypl/es/v2"
	"github.com/thalesfsp/sypl/v2"
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

	o := es.BulkOutput(
		"idx-e2e",
		es.Config{Addresses: []string{srv.URL}},
		level.Trace,
		[]es.BulkOption{es.BulkWithNumWorkers(1)},
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
