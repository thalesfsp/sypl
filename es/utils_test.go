// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package elasticsearch

import (
	"io"
	"strings"
	"testing"

	"github.com/elastic/go-elasticsearch/v8/esapi"
)

func TestParseResponseBody(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr bool
		check   func(t *testing.T, got map[string]interface{})
	}{
		{
			name: "Should work - valid JSON object",
			body: `{"result":"created","count":2}`,
			check: func(t *testing.T, got map[string]interface{}) {
				t.Helper()

				if got["result"] != "created" {
					t.Errorf(`got["result"] = %v, want "created"`, got["result"])
				}

				// JSON numbers decode as float64.
				if got["count"] != float64(2) {
					t.Errorf(`got["count"] = %v, want 2`, got["count"])
				}
			},
		},
		{
			name: "Should work - empty JSON object",
			body: `{}`,
			check: func(t *testing.T, got map[string]interface{}) {
				t.Helper()

				if len(got) != 0 {
					t.Errorf("Expected empty map, got %+v", got)
				}
			},
		},
		{
			name:    "Should fail - invalid JSON",
			body:    `{invalid`,
			wantErr: true,
		},
		{
			name:    "Should fail - empty body",
			body:    ``,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseResponseBody(strings.NewReader(tt.body))

			if tt.wantErr {
				if err == nil {
					t.Fatal("parseResponseBody() should fail")
				}

				if !strings.Contains(err.Error(), "failed parsing the response body") {
					t.Errorf("Unexpected error: %v", err)
				}

				return
			}

			if err != nil {
				t.Fatalf("parseResponseBody() error = %v, want nil", err)
			}

			if tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}

func TestParseResponseBodyError(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		want    string
		wantErr bool
	}{
		{
			name: "Should work - error with a string reason",
			body: `{"error":{"reason":"index is read-only"}}`,
			want: "index is read-only",
		},
		{
			name: "Should work - error isn't a map, falls back to the whole body",
			body: `{"error":"plain string"}`,
			want: "map[error:plain string]",
		},
		{
			name: "Should work - no error key at all, falls back to the whole body",
			body: `{"something":"else"}`,
			want: "map[something:else]",
		},
		{
			name: "Should work - reason isn't a string, falls back to the error map",
			body: `{"error":{"reason":42}}`,
			want: "map[reason:42]",
		},
		{
			name:    "Should fail - body isn't JSON",
			body:    `boom`,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := &esapi.Response{
				StatusCode: 400,
				Body:       io.NopCloser(strings.NewReader(tt.body)),
			}

			got, err := parseResponseBodyError(res)

			if tt.wantErr {
				if err == nil {
					t.Fatal("parseResponseBodyError() should fail")
				}

				return
			}

			if err != nil {
				t.Fatalf("parseResponseBodyError() error = %v, want nil", err)
			}

			if got != tt.want {
				t.Errorf("parseResponseBodyError() = %q, want %q", got, tt.want)
			}
		})
	}
}
