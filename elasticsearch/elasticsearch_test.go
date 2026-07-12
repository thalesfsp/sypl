// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package elasticsearch

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
)

//////
// Test helpers.
//////

// infoBody is a minimal Elasticsearch Info (ping) response - the v8 client
// validates it on the first request.
const infoBody = `{
	"name": "fake-node",
	"cluster_name": "fake-cluster",
	"cluster_uuid": "abc123",
	"version": {"number": "8.16.0", "build_flavor": "default"},
	"tagline": "You Know, for Search"
}`

// capturedRequest records what the fake Elasticsearch server received.
type capturedRequest struct {
	Method string
	Path   string
	Body   string
}

// requestRecorder concurrency-safely accumulates captured requests.
type requestRecorder struct {
	mu       sync.Mutex
	requests []capturedRequest
}

func (rr *requestRecorder) add(r capturedRequest) {
	rr.mu.Lock()
	defer rr.mu.Unlock()

	rr.requests = append(rr.requests, r)
}

func (rr *requestRecorder) all() []capturedRequest {
	rr.mu.Lock()
	defer rr.mu.Unlock()

	out := make([]capturedRequest, len(rr.requests))
	copy(out, rr.requests)

	return out
}

// newFakeES starts a fake Elasticsearch server. It answers the Info ping
// ("/") with a valid product response - including the mandatory
// "X-Elastic-Product" header - and delegates any other path to `handler`.
func newFakeES(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		w.Header().Set("Content-Type", "application/json")

		if r.URL.Path == "/" {
			fmt.Fprint(w, infoBody)

			return
		}

		if handler != nil {
			handler(w, r)

			return
		}

		fmt.Fprint(w, `{"result":"created"}`)
	}))

	t.Cleanup(srv.Close)

	return srv
}

// newTestES creates an `ElasticSearch` against the fake server, exercising
// the `New` factory - Info ping included.
func newTestES(t *testing.T, indexName string, handler http.HandlerFunc) *ElasticSearch {
	t.Helper()

	srv := newFakeES(t, handler)

	return New(indexName, Config{Addresses: []string{srv.URL}})
}

//////
// Factories.
//////

func TestNew(t *testing.T) {
	es := newTestES(t, "test-index", nil)

	if es.Client == nil {
		t.Fatal("New() should set a non-nil client")
	}

	if es.DynamicIndex == nil {
		t.Fatal("New() should set a non-nil DynamicIndex")
	}

	if got := es.DynamicIndex(); got != "test-index" {
		t.Errorf("DynamicIndex() = %q, want %q", got, "test-index")
	}
}

func TestNewWithDynamicIndex(t *testing.T) {
	srv := newFakeES(t, nil)

	// The index name must be evaluated at index time - not at creation
	// time.
	current := "idx-1"

	es := NewWithDynamicIndex(
		func() string { return current },
		Config{Addresses: []string{srv.URL}},
	)

	if es.Client == nil {
		t.Fatal("NewWithDynamicIndex() should set a non-nil client")
	}

	if got := es.DynamicIndex(); got != "idx-1" {
		t.Errorf("DynamicIndex() = %q, want %q", got, "idx-1")
	}

	current = "idx-2"

	if got := es.DynamicIndex(); got != "idx-2" {
		t.Errorf("DynamicIndex() after change = %q, want %q", got, "idx-2")
	}
}

// New calls log.Fatalf on failures, so the failure paths are asserted by
// re-running this test binary as a subprocess, and checking it dies with
// exit code 1, and the expected message.
func TestNew_FatalPaths(t *testing.T) {
	// Subprocess mode: call the fatal path under test, and nothing else. If
	// `New` doesn't exit, exit with a sentinel code so the parent fails the
	// assertion.
	if mode, ok := os.LookupEnv("SYPL_TEST_ES_FATAL_MODE"); ok {
		switch mode {
		case "badconfig":
			// Both Addresses and CloudID set - the client constructor
			// rejects it.
			New("idx", Config{
				Addresses: []string{"http://127.0.0.1:9200"},
				CloudID:   "name:Zm9vLmJhcjokYWJjJGRlZg==",
			})
		case "unreachable":
			// Nothing listens on port 1 - the Info ping fails.
			New("idx", Config{Addresses: []string{"http://127.0.0.1:1"}})
		case "badbody":
			// The Info ping succeeds - but its body dies mid-read
			// (declared Content-Length is larger than what is written), so
			// consuming it fails.
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-Elastic-Product", "Elasticsearch")
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Content-Length", "4096")

				_, _ = w.Write([]byte("{"))
			}))
			defer srv.Close()

			New("idx", Config{Addresses: []string{srv.URL}})
		}

		os.Exit(42)
	}

	tests := []struct {
		name    string
		mode    string
		wantMsg string
	}{
		{
			name:    "Should exit - invalid configuration",
			mode:    "badconfig",
			wantMsg: "Error creating the ElasticSearch client",
		},
		{
			name:    "Should exit - unreachable ElasticSearch",
			mode:    "unreachable",
			wantMsg: "Error pinging Elasticsearch",
		},
		{
			name:    "Should exit - broken ping response body",
			mode:    "badbody",
			wantMsg: "Error consuming the response body",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			//nolint:gosec // Re-running the test binary itself.
			cmd := exec.Command(os.Args[0], "-test.run=TestNew_FatalPaths$")

			cmd.Env = append(os.Environ(), "SYPL_TEST_ES_FATAL_MODE="+tt.mode)

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

			if !strings.Contains(stderr.String(), tt.wantMsg) {
				t.Errorf("Expected stderr to contain %q, got %q", tt.wantMsg, stderr.String())
			}
		})
	}
}

//////
// Write - happy paths.
//////

func TestElasticSearch_Write(t *testing.T) {
	tests := []struct {
		name     string
		response string
	}{
		{
			name:     "Should work - created",
			response: `{"result":"created"}`,
		},
		{
			name:     "Should work - updated",
			response: `{"result":"updated"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			es := newTestES(t, "test-index", func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprint(w, tt.response)
			})

			data := []byte(`{"message":"hello"}`)

			n, err := es.Write(data)
			if err != nil {
				t.Fatalf("Write() error = %v, want nil", err)
			}

			if n != len(data) {
				t.Errorf("Write() = %d, want %d", n, len(data))
			}
		})
	}
}

func TestElasticSearch_Write_DocumentID(t *testing.T) {
	tests := []struct {
		name       string
		data       string
		wantInPath string
		wantMethod string
	}{
		{
			name: "Should work - string id becomes the document ID",
			data: `{"id":"doc-123","message":"hello"}`,

			// esapi routes `IndexRequest` with a document ID as
			// PUT /{index}/_doc/{id}.
			wantInPath: "/test-index/_doc/doc-123",
			wantMethod: http.MethodPut,
		},
		{
			name: "Should work - numeric id is skipped, not a panic",
			data: `{"id":123,"message":"hello"}`,

			// Without a document ID, esapi routes as POST /{index}/_doc.
			wantInPath: "/test-index/_doc",
			wantMethod: http.MethodPost,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := &requestRecorder{}

			es := newTestES(t, "test-index", func(w http.ResponseWriter, r *http.Request) {
				recorder.add(capturedRequest{Method: r.Method, Path: r.URL.Path})

				fmt.Fprint(w, `{"result":"created"}`)
			})

			if _, err := es.Write([]byte(tt.data)); err != nil {
				t.Fatalf("Write() error = %v, want nil", err)
			}

			requests := recorder.all()

			if len(requests) != 1 {
				t.Fatalf("Expected 1 index request, got %d", len(requests))
			}

			if requests[0].Path != tt.wantInPath {
				t.Errorf("Request path = %q, want %q", requests[0].Path, tt.wantInPath)
			}

			if requests[0].Method != tt.wantMethod {
				t.Errorf("Request method = %q, want %q", requests[0].Method, tt.wantMethod)
			}
		})
	}
}

//////
// Write - bad paths.
//////

func TestElasticSearch_Write_NonJSONData(t *testing.T) {
	recorder := &requestRecorder{}

	es := newTestES(t, "test-index", func(w http.ResponseWriter, r *http.Request) {
		recorder.add(capturedRequest{Method: r.Method, Path: r.URL.Path})

		fmt.Fprint(w, `{"result":"created"}`)
	})

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

	// The data never left the process.
	if got := len(recorder.all()); got != 0 {
		t.Errorf("Expected no index requests, got %d", got)
	}
}

func TestElasticSearch_Write_UninitializedClient(t *testing.T) {
	es := &ElasticSearch{
		Client:       nil,
		DynamicIndex: func() string { return "test-index" },
	}

	n, err := es.Write([]byte(`{"message":"hello"}`))
	if err == nil {
		t.Fatal("Write() with a nil client should fail - not panic")
	}

	if n != 0 {
		t.Errorf("Write() = %d, want 0", n)
	}

	if !strings.Contains(err.Error(), "elasticsearch client isn't initialized") {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestElasticSearch_Write_RequestFailure(t *testing.T) {
	srv := newFakeES(t, nil)

	es := New("test-index", Config{Addresses: []string{srv.URL}})

	// Kill the server - the index request can't be performed anymore.
	srv.Close()

	n, err := es.Write([]byte(`{"message":"hello"}`))
	if err == nil {
		t.Fatal("Write() against a dead server should fail")
	}

	if n != 0 {
		t.Errorf("Write() = %d, want 0", n)
	}

	if !strings.Contains(err.Error(), "failed getting response") {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestElasticSearch_Write_ErrorResponses(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		response   string
		wantInErr  string
	}{
		{
			name:       "Should fail - 400 with reason",
			statusCode: http.StatusBadRequest,
			response:   `{"error":{"reason":"index is read-only"}}`,
			wantInErr:  "failed indexing document: index is read-only",
		},
		{
			name:       "Should fail - 500 with reason",
			statusCode: http.StatusInternalServerError,
			response:   `{"error":{"reason":"shard failure"}}`,
			wantInErr:  "failed indexing document: shard failure",
		},
		{
			name:       "Should fail - error isn't a map, fallback to raw body",
			statusCode: http.StatusBadRequest,
			response:   `{"error":"plain string error"}`,
			wantInErr:  "plain string error",
		},
		{
			name:       "Should fail - error map without a string reason, fallback",
			statusCode: http.StatusBadRequest,
			response:   `{"error":{"reason":123}}`,
			wantInErr:  "failed indexing document: map[reason:123]",
		},
		{
			name:       "Should fail - error body isn't JSON",
			statusCode: http.StatusBadRequest,
			response:   `boom`,
			wantInErr:  "failed parsing the response body",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			es := newTestES(t, "test-index", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)

				fmt.Fprint(w, tt.response)
			})

			n, err := es.Write([]byte(`{"message":"hello"}`))
			if err == nil {
				t.Fatal("Write() should fail")
			}

			if n != 0 {
				t.Errorf("Write() = %d, want 0", n)
			}

			if !strings.Contains(err.Error(), tt.wantInErr) {
				t.Errorf("Error %q should contain %q", err.Error(), tt.wantInErr)
			}
		})
	}
}

func TestElasticSearch_Write_UnexpectedSuccessResponses(t *testing.T) {
	tests := []struct {
		name      string
		response  string
		wantInErr string
	}{
		{
			name:      "Should fail - missing result",
			response:  `{"ok":true}`,
			wantInErr: `missing, or non-string "result"`,
		},
		{
			name:      "Should fail - non-string result, no panic",
			response:  `{"result":123}`,
			wantInErr: `missing, or non-string "result"`,
		},
		{
			name:      "Should fail - unexpected result value",
			response:  `{"result":"noop"}`,
			wantInErr: "unexpected result",
		},
		{
			name:      "Should fail - non-JSON success body",
			response:  `garbage`,
			wantInErr: "failed parsing the response body",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			es := newTestES(t, "test-index", func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprint(w, tt.response)
			})

			n, err := es.Write([]byte(`{"message":"hello"}`))
			if err == nil {
				t.Fatal("Write() should fail")
			}

			if n != 0 {
				t.Errorf("Write() = %d, want 0", n)
			}

			if !strings.Contains(err.Error(), tt.wantInErr) {
				t.Errorf("Error %q should contain %q", err.Error(), tt.wantInErr)
			}
		})
	}
}
