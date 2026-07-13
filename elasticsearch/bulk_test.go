// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package elasticsearch

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elastic/go-elasticsearch/v8/esutil"
)

//////
// Test helpers.
//////

// bulkOKHandler answers a _bulk request reporting every item as created.
func bulkOKHandler(w http.ResponseWriter, r *http.Request) {
	var body bytes.Buffer

	_, _ = body.ReadFrom(r.Body)

	// NDJSON: one action line + one source line per item.
	lines := strings.Count(strings.TrimSuffix(body.String(), "\n"), "\n") + 1

	items := []string{}

	for range lines / 2 {
		items = append(items, `{"index":{"_index":"idx","status":201,"result":"created"}}`)
	}

	fmt.Fprintf(w, `{"took":1,"errors":false,"items":[%s]}`, strings.Join(items, ","))
}

// newTestBulk creates an `ElasticSearchBulk` against a fake server,
// exercising the `NewBulk` factory - Info ping included. Single worker, so
// batching is deterministic.
func newTestBulk(
	t *testing.T,
	handler http.HandlerFunc,
	opts ...BulkOption,
) (*ElasticSearchBulk, *requestRecorder) {
	t.Helper()

	recorder := &requestRecorder{}

	srv := newFakeES(t, func(w http.ResponseWriter, r *http.Request) {
		var body bytes.Buffer

		_, _ = body.ReadFrom(r.Body)

		recorder.add(capturedRequest{Method: r.Method, Path: r.URL.Path, Body: body.String()})

		if handler != nil {
			// The recorder consumed the body - hand the handler a copy.
			r.Body = nopReadCloser{bytes.NewReader([]byte(body.String()))}

			handler(w, r)

			return
		}

		r.Body = nopReadCloser{bytes.NewReader([]byte(body.String()))}

		bulkOKHandler(w, r)
	})

	es := NewBulk(
		"test-index",
		Config{Addresses: []string{srv.URL}},
		append([]BulkOption{BulkWithNumWorkers(1)}, opts...)...,
	)

	return es, recorder
}

// nopReadCloser wraps a reader into an io.ReadCloser.
type nopReadCloser struct{ *bytes.Reader }

func (nopReadCloser) Close() error { return nil }

// bulkErrorCollector concurrency-safely accumulates callback errors.
type bulkErrorCollector struct {
	mu     sync.Mutex
	errors []error
}

func (c *bulkErrorCollector) callback() func(error) {
	return func(err error) {
		c.mu.Lock()
		defer c.mu.Unlock()

		c.errors = append(c.errors, err)
	}
}

func (c *bulkErrorCollector) all() []error {
	c.mu.Lock()
	defer c.mu.Unlock()

	out := make([]error, len(c.errors))
	copy(out, c.errors)

	return out
}

// stubIndexer is a controllable esutil.BulkIndexer for error paths.
type stubIndexer struct {
	addErr   error
	closeErr error
}

func (s *stubIndexer) Add(context.Context, esutil.BulkIndexerItem) error { return s.addErr }
func (s *stubIndexer) Close(context.Context) error                       { return s.closeErr }
func (s *stubIndexer) Stats() esutil.BulkIndexerStats                    { return esutil.BulkIndexerStats{} }

//////
// Factories.
//////

func TestNewBulk(t *testing.T) {
	es, _ := newTestBulk(t, nil)

	defer es.Close()

	if es.Client == nil {
		t.Fatal("NewBulk() should set a non-nil client")
	}

	if es.DynamicIndex == nil {
		t.Fatal("NewBulk() should set a non-nil DynamicIndex")
	}

	if got := es.DynamicIndex(); got != "test-index" {
		t.Errorf("DynamicIndex() = %q, want %q", got, "test-index")
	}
}

func TestNewBulk_Options(t *testing.T) {
	collector := &bulkErrorCollector{}

	srv := newFakeES(t, nil)

	es := NewBulkWithDynamicIndex(
		func() string { return "idx-dyn" },
		Config{Addresses: []string{srv.URL}},
		BulkWithFlushBytes(1234),
		BulkWithFlushInterval(7*time.Second),
		BulkWithNumWorkers(3),
		BulkWithOnError(collector.callback()),
		BulkWithCloseTimeout(11*time.Second),
	)

	defer es.Close()

	if es.flushBytes != 1234 {
		t.Errorf("flushBytes = %d, want 1234", es.flushBytes)
	}

	if es.flushInterval != 7*time.Second {
		t.Errorf("flushInterval = %v, want 7s", es.flushInterval)
	}

	if es.numWorkers != 3 {
		t.Errorf("numWorkers = %d, want 3", es.numWorkers)
	}

	if es.onError == nil {
		t.Error("onError should be set")
	}

	if es.closeTimeout != 11*time.Second {
		t.Errorf("closeTimeout = %v, want 11s", es.closeTimeout)
	}

	if got := es.DynamicIndex(); got != "idx-dyn" {
		t.Errorf("DynamicIndex() = %q, want %q", got, "idx-dyn")
	}
}

// NewBulk mirrors New: it calls log.Fatalf when the indexer can't be
// created. The failure is injected through the test seam, and asserted by
// re-running this test binary as a subprocess.
func TestNewBulk_FatalPath(t *testing.T) {
	if _, ok := os.LookupEnv("SYPL_TEST_ES_BULK_FATAL_MODE"); ok {
		newBulkIndexer = func(esutil.BulkIndexerConfig) (esutil.BulkIndexer, error) {
			return nil, errors.New("indexer construction denied")
		}

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Elastic-Product", "Elasticsearch")
			w.Header().Set("Content-Type", "application/json")

			fmt.Fprint(w, infoBody)
		}))

		NewBulk("idx", Config{Addresses: []string{srv.URL}})

		// Only reached if NewBulk fails to exit; the sentinel below
		// reports it.
		srv.Close()

		os.Exit(42)
	}

	//nolint:gosec // Re-running the test binary itself.
	cmd := exec.Command(os.Args[0], "-test.run=TestNewBulk_FatalPath$")

	cmd.Env = append(os.Environ(), "SYPL_TEST_ES_BULK_FATAL_MODE=1")

	var stderr bytes.Buffer

	cmd.Stderr = &stderr

	err := cmd.Run()

	var exitErr *exec.ExitError

	if !errors.As(err, &exitErr) {
		t.Fatalf("Expected subprocess to exit with an error, got %v (stderr: %s)", err, stderr.String())
	}

	if code := exitErr.ExitCode(); code != 1 {
		t.Errorf("Expected exit code 1 (log.Fatalf), got %d (stderr: %s)", code, stderr.String())
	}

	if !strings.Contains(stderr.String(), "Error creating the ElasticSearch bulk indexer") {
		t.Errorf("Unexpected stderr: %q", stderr.String())
	}
}

//////
// Write - happy paths.
//////

func TestElasticSearchBulk_Write_BatchesIntoBulkRequests(t *testing.T) {
	es, recorder := newTestBulk(t, nil)

	docs := []string{
		`{"message":"one"}`,
		`{"message":"two"}`,
		`{"message":"three"}`,
	}

	for _, doc := range docs {
		n, err := es.Write([]byte(doc))
		if err != nil {
			t.Fatalf("Write() error = %v, want nil", err)
		}

		if n != len(doc) {
			t.Errorf("Write() = %d, want %d", n, len(doc))
		}
	}

	// Flush drains the indexer - everything lands in ONE _bulk request.
	if err := es.Flush(); err != nil {
		t.Fatalf("Flush() error = %v, want nil", err)
	}

	requests := recorder.all()

	if len(requests) != 1 {
		t.Fatalf("Expected 1 _bulk request, got %d: %+v", len(requests), requests)
	}

	if requests[0].Method != http.MethodPost || requests[0].Path != "/_bulk" {
		t.Errorf("Request = %s %s, want POST /_bulk", requests[0].Method, requests[0].Path)
	}

	// NDJSON shape: action line + source line per item.
	lines := strings.Split(strings.TrimSuffix(requests[0].Body, "\n"), "\n")

	if len(lines) != 6 {
		t.Fatalf("Expected 6 NDJSON lines, got %d: %q", len(lines), requests[0].Body)
	}

	for i, doc := range docs {
		action := lines[i*2]

		if !strings.Contains(action, `"index"`) || !strings.Contains(action, `"_index":"test-index"`) {
			t.Errorf("Action line %d = %q, want an index action into test-index", i, action)
		}

		if lines[i*2+1] != doc {
			t.Errorf("Source line %d = %q, want %q", i, lines[i*2+1], doc)
		}
	}
}

func TestElasticSearchBulk_Write_DocumentID(t *testing.T) {
	tests := []struct {
		name       string
		data       string
		wantInBody string
		notInBody  string
	}{
		{
			name:       "Should work - string id becomes the document ID",
			data:       `{"id":"doc-123","message":"hello"}`,
			wantInBody: `"_id":"doc-123"`,
		},
		{
			name:      "Should work - numeric id is skipped, not a panic",
			data:      `{"id":123,"message":"hello"}`,
			notInBody: `"_id"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			es, recorder := newTestBulk(t, nil)

			if _, err := es.Write([]byte(tt.data)); err != nil {
				t.Fatalf("Write() error = %v, want nil", err)
			}

			if err := es.Close(); err != nil {
				t.Fatalf("Close() error = %v, want nil", err)
			}

			requests := recorder.all()

			if len(requests) != 1 {
				t.Fatalf("Expected 1 _bulk request, got %d", len(requests))
			}

			if tt.wantInBody != "" && !strings.Contains(requests[0].Body, tt.wantInBody) {
				t.Errorf("Body %q should contain %q", requests[0].Body, tt.wantInBody)
			}

			if tt.notInBody != "" && strings.Contains(requests[0].Body, tt.notInBody) {
				t.Errorf("Body %q should NOT contain %q", requests[0].Body, tt.notInBody)
			}
		})
	}
}

func TestElasticSearchBulk_DynamicIndexRouting(t *testing.T) {
	recorder := &requestRecorder{}

	srv := newFakeES(t, func(w http.ResponseWriter, r *http.Request) {
		var body bytes.Buffer

		_, _ = body.ReadFrom(r.Body)

		recorder.add(capturedRequest{Method: r.Method, Path: r.URL.Path, Body: body.String()})

		r.Body = nopReadCloser{bytes.NewReader([]byte(body.String()))}

		bulkOKHandler(w, r)
	})

	day := "2026-07-12"

	es := NewBulkWithDynamicIndex(
		func() string { return "idx-" + day },
		Config{Addresses: []string{srv.URL}},
		BulkWithNumWorkers(1),
	)

	if _, err := es.Write([]byte(`{"message":"first"}`)); err != nil {
		t.Fatalf("Write() error = %v, want nil", err)
	}

	// The index name is evaluated at WRITE time.
	day = "2026-07-13"

	if _, err := es.Write([]byte(`{"message":"second"}`)); err != nil {
		t.Fatalf("Write() error = %v, want nil", err)
	}

	if err := es.Close(); err != nil {
		t.Fatalf("Close() error = %v, want nil", err)
	}

	requests := recorder.all()

	if len(requests) != 1 {
		t.Fatalf("Expected 1 _bulk request, got %d", len(requests))
	}

	if !strings.Contains(requests[0].Body, `"_index":"idx-2026-07-12"`) ||
		!strings.Contains(requests[0].Body, `"_index":"idx-2026-07-13"`) {
		t.Errorf("Body %q should route each item to its own index", requests[0].Body)
	}
}

func TestElasticSearchBulk_FlushBytesAutoFlushes(t *testing.T) {
	flushed := make(chan struct{}, 8)

	es, _ := newTestBulk(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case flushed <- struct{}{}:
		default:
		}

		bulkOKHandler(w, r)
	}, BulkWithFlushBytes(1))

	defer es.Close()

	if _, err := es.Write([]byte(`{"message":"auto"}`)); err != nil {
		t.Fatalf("Write() error = %v, want nil", err)
	}

	// No explicit Flush: the tiny FlushBytes threshold flushes by itself.
	select {
	case <-flushed:
	case <-time.After(2 * time.Second):
		t.Fatal("The indexer should auto-flush on the FlushBytes threshold")
	}
}

func TestElasticSearchBulk_Write_TrimsTrailingLinebreaks(t *testing.T) {
	es, recorder := newTestBulk(t, nil)

	// Sypl restores the message's trailing linebreak AFTER formatting: the
	// payload arrives as `{...}\n`. Verbatim, it would inject a blank line
	// into the NDJSON body - malformed for the _bulk protocol.
	doc := `{"message":"newline"}` + "\r\n"

	n, err := es.Write([]byte(doc))
	if err != nil {
		t.Fatalf("Write() error = %v, want nil", err)
	}

	// The full input is reported as consumed - io.Writer contract.
	if n != len(doc) {
		t.Errorf("Write() = %d, want %d", n, len(doc))
	}

	if err := es.Close(); err != nil {
		t.Fatalf("Close() error = %v, want nil", err)
	}

	requests := recorder.all()

	if len(requests) != 1 {
		t.Fatalf("Expected 1 _bulk request, got %d", len(requests))
	}

	lines := strings.Split(strings.TrimSuffix(requests[0].Body, "\n"), "\n")

	if len(lines) != 2 {
		t.Fatalf("Expected 2 NDJSON lines - no blank lines, got %d: %q", len(lines), requests[0].Body)
	}

	if lines[1] != `{"message":"newline"}` {
		t.Errorf("Source line = %q, want the trimmed document", lines[1])
	}
}

//////
// Write - bad paths.
//////

func TestElasticSearchBulk_Write_NonJSONData(t *testing.T) {
	es, recorder := newTestBulk(t, nil)

	defer es.Close()

	n, err := es.Write([]byte("not-json"))
	if err == nil {
		t.Fatal("Write() with non-JSON data should fail")
	}

	if n != 0 {
		t.Errorf("Write() = %d, want 0", n)
	}

	if !strings.Contains(err.Error(), "failed parsing the response body") {
		t.Errorf("Unexpected error: %v", err)
	}

	if got := len(recorder.all()); got != 0 {
		t.Errorf("Expected no _bulk requests, got %d", got)
	}
}

func TestElasticSearchBulk_Write_UninitializedIndexer(t *testing.T) {
	es := &ElasticSearchBulk{
		DynamicIndex: func() string { return "test-index" },
	}

	n, err := es.Write([]byte(`{"message":"hello"}`))
	if err == nil {
		t.Fatal("Write() with a nil indexer should fail - not panic")
	}

	if n != 0 {
		t.Errorf("Write() = %d, want 0", n)
	}

	if !strings.Contains(err.Error(), "bulk indexer isn't initialized") {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestElasticSearchBulk_Write_AddError(t *testing.T) {
	es, _ := newTestBulk(t, nil)

	defer es.Close()

	// Swap in a failing indexer - the Add error must surface.
	es.mu.Lock()
	es.indexer = &stubIndexer{addErr: errors.New("add rejected")}
	es.mu.Unlock()

	n, err := es.Write([]byte(`{"message":"hello"}`))
	if err == nil || !strings.Contains(err.Error(), "add rejected") {
		t.Fatalf("Write() error = %v, want the Add failure", err)
	}

	if n != 0 {
		t.Errorf("Write() = %d, want 0", n)
	}
}

func TestElasticSearchBulk_PerItemFailuresReachCallback(t *testing.T) {
	collector := &bulkErrorCollector{}

	// A partial _bulk failure: item 1 created, item 2 rejected.
	es, _ := newTestBulk(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"took":1,"errors":true,"items":[`+
			`{"index":{"_index":"idx","status":201,"result":"created"}},`+
			`{"index":{"_index":"idx","_id":"doc-2","status":400,`+
			`"error":{"type":"mapper_parsing_exception","reason":"failed to parse field"}}}`+
			`]}`)
	}, BulkWithOnError(collector.callback()))

	if _, err := es.Write([]byte(`{"message":"good"}`)); err != nil {
		t.Fatalf("Write() error = %v, want nil", err)
	}

	if _, err := es.Write([]byte(`{"id":"doc-2","message":"bad"}`)); err != nil {
		t.Fatalf("Write() error = %v, want nil", err)
	}

	if err := es.Close(); err != nil {
		t.Fatalf("Close() error = %v, want nil", err)
	}

	callbackErrors := collector.all()

	if len(callbackErrors) != 1 {
		t.Fatalf("Expected 1 callback error, got %d: %v", len(callbackErrors), callbackErrors)
	}

	msg := callbackErrors[0].Error()

	for _, want := range []string{"doc-2", "mapper_parsing_exception", "failed to parse field"} {
		if !strings.Contains(msg, want) {
			t.Errorf("Callback error %q should contain %q", msg, want)
		}
	}
}

func TestElasticSearchBulk_ServerDownReachesCallback(t *testing.T) {
	collector := &bulkErrorCollector{}

	recorder := &requestRecorder{}

	srv := newFakeES(t, func(w http.ResponseWriter, r *http.Request) {
		recorder.add(capturedRequest{Method: r.Method, Path: r.URL.Path})
	})

	es := NewBulk(
		"test-index",
		Config{Addresses: []string{srv.URL}},
		BulkWithNumWorkers(1),
		BulkWithOnError(collector.callback()),
	)

	// Kill the server - the flush can't be performed anymore.
	srv.Close()

	if _, err := es.Write([]byte(`{"message":"hello"}`)); err != nil {
		t.Fatalf("Write() error = %v, want nil - the failure surfaces at flush", err)
	}

	if err := es.Close(); err != nil {
		t.Fatalf("Close() error = %v, want nil", err)
	}

	callbackErrors := collector.all()

	if len(callbackErrors) == 0 {
		t.Fatal("Expected the flush failure to reach the callback")
	}

	if !strings.Contains(callbackErrors[0].Error(), "flush") {
		t.Errorf("Callback error = %v, want a flush failure", callbackErrors[0])
	}
}

//////
// Flush, and Close.
//////

func TestElasticSearchBulk_FlushSwapsTheIndexerAndKeepsWorking(t *testing.T) {
	es, recorder := newTestBulk(t, nil)

	if _, err := es.Write([]byte(`{"message":"first"}`)); err != nil {
		t.Fatalf("Write() error = %v, want nil", err)
	}

	if err := es.Flush(); err != nil {
		t.Fatalf("Flush() error = %v, want nil", err)
	}

	// The output keeps accepting writes after a flush.
	if _, err := es.Write([]byte(`{"message":"second"}`)); err != nil {
		t.Fatalf("Write() after Flush error = %v, want nil", err)
	}

	if err := es.Flush(); err != nil {
		t.Fatalf("Second Flush() error = %v, want nil", err)
	}

	requests := recorder.all()

	if len(requests) != 2 {
		t.Fatalf("Expected 2 _bulk requests, got %d", len(requests))
	}

	if err := es.Close(); err != nil {
		t.Fatalf("Close() error = %v, want nil", err)
	}
}

func TestElasticSearchBulk_FlushErrorSurfacesAndRecovers(t *testing.T) {
	es, _ := newTestBulk(t, nil)

	es.mu.Lock()
	es.indexer = &stubIndexer{closeErr: errors.New("drain failed")}
	es.mu.Unlock()

	if err := es.Flush(); err == nil || !strings.Contains(err.Error(), "drain failed") {
		t.Fatalf("Flush() error = %v, want the drain failure", err)
	}

	// A fresh indexer was swapped in - the output still works.
	if _, err := es.Write([]byte(`{"message":"recovered"}`)); err != nil {
		t.Fatalf("Write() after failed Flush error = %v, want nil", err)
	}

	if err := es.Close(); err != nil {
		t.Fatalf("Close() error = %v, want nil", err)
	}
}

func TestElasticSearchBulk_CloseDrainsIsIdempotentAndGuardsWrites(t *testing.T) {
	es, recorder := newTestBulk(t, nil)

	if _, err := es.Write([]byte(`{"message":"pending"}`)); err != nil {
		t.Fatalf("Write() error = %v, want nil", err)
	}

	if err := es.Close(); err != nil {
		t.Fatalf("Close() error = %v, want nil", err)
	}

	// Close drained the pending item.
	if got := len(recorder.all()); got != 1 {
		t.Errorf("Expected 1 _bulk request after Close, got %d", got)
	}

	// Idempotent.
	if err := es.Close(); err != nil {
		t.Fatalf("Second Close() error = %v, want nil", err)
	}

	// Write after Close: typed error, no panic - esutil's Add would panic
	// on a closed indexer.
	n, err := es.Write([]byte(`{"message":"late"}`))

	if !errors.Is(err, ErrBulkClosed) {
		t.Errorf("Write() after Close error = %v, want ErrBulkClosed", err)
	}

	if n != 0 {
		t.Errorf("Write() = %d, want 0", n)
	}

	// Flush after Close: documented no-op.
	if err := es.Flush(); err != nil {
		t.Errorf("Flush() after Close error = %v, want nil", err)
	}
}

func TestElasticSearchBulk_CloseErrorSurfaces(t *testing.T) {
	es, _ := newTestBulk(t, nil)

	es.mu.Lock()
	old := es.indexer
	es.indexer = &stubIndexer{closeErr: errors.New("close failed")}
	es.mu.Unlock()

	if err := es.Close(); err == nil || !strings.Contains(err.Error(), "close failed") {
		t.Errorf("Close() error = %v, want the close failure", err)
	}

	// Release the real indexer's goroutines.
	_ = old.Close(context.Background())
}
