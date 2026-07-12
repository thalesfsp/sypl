// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package shared

import (
	"bytes"
	"log"
	"strings"
	"testing"

	"github.com/google/uuid"
)

type testData struct {
	Name  string
	Value int

	// Unexported fields must be dropped by the JSON encoder.
	hidden string //nolint:structcheck,unused
}

func TestPrettify(t *testing.T) {
	type args struct {
		data interface{}
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "Should work - struct - unexported fields dropped",
			args: args{
				data: testData{Name: "test", Value: 1, hidden: "secret"},
			},
			want: "{\n\t\"Name\": \"test\",\n\t\"Value\": 1\n}\n",
		},
		{
			name: "Should work - map",
			args: args{
				data: map[string]int{"a": 1},
			},
			want: "{\n\t\"a\": 1\n}\n",
		},
		{
			name: "Should work - nil",
			args: args{
				data: nil,
			},
			want: "null\n",
		},
		{
			name: "Should work - empty string",
			args: args{
				data: "",
			},
			want: "\"\"\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Prettify(tt.args.data); got != tt.want {
				t.Errorf("Prettify() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPrettify_NonSerializable(t *testing.T) {
	// The error path logs; capture it so the test both stays quiet, and
	// asserts the failure is reported.
	var logBuf bytes.Buffer

	origWriter := log.Writer()

	log.SetOutput(&logBuf)

	t.Cleanup(func() { log.SetOutput(origWriter) })

	if got := Prettify(make(chan int)); got != "" {
		t.Errorf("Prettify(chan) = %q, want empty string", got)
	}

	if !strings.Contains(logBuf.String(), ErrorPrefix) {
		t.Errorf("Expected log output to contain %q, got %q", ErrorPrefix, logBuf.String())
	}
}

func TestInline(t *testing.T) {
	type args struct {
		data interface{}
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "Should work - struct - single line",
			args: args{
				data: testData{Name: "test", Value: 1, hidden: "secret"},
			},
			want: "{\"Name\":\"test\",\"Value\":1}\n",
		},
		{
			name: "Should work - nil",
			args: args{
				data: nil,
			},
			want: "null\n",
		},
		{
			name: "Should work - slice",
			args: args{
				data: []int{1, 2, 3},
			},
			want: "[1,2,3]\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Inline(tt.args.data); got != tt.want {
				t.Errorf("Inline() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestInline_NonSerializable(t *testing.T) {
	var logBuf bytes.Buffer

	origWriter := log.Writer()

	log.SetOutput(&logBuf)

	t.Cleanup(func() { log.SetOutput(origWriter) })

	if got := Inline(func() {}); got != "" {
		t.Errorf("Inline(func) = %q, want empty string", got)
	}

	if !strings.Contains(logBuf.String(), ErrorPrefix) {
		t.Errorf("Expected log output to contain %q, got %q", ErrorPrefix, logBuf.String())
	}
}

func TestGenerateUUID(t *testing.T) {
	got := GenerateUUID()

	parsed, err := uuid.Parse(got)
	if err != nil {
		t.Fatalf("GenerateUUID() = %q, not a valid UUID: %v", got, err)
	}

	if parsed.Version() != 4 {
		t.Errorf("GenerateUUID() version = %d, want 4", parsed.Version())
	}

	if parsed.Variant() != uuid.RFC4122 {
		t.Errorf("GenerateUUID() variant = %v, want %v", parsed.Variant(), uuid.RFC4122)
	}
}

func TestGenerateUUID_Uniqueness(t *testing.T) {
	seen := map[string]bool{}

	for i := 0; i < 100; i++ {
		id := GenerateUUID()

		if seen[id] {
			t.Fatalf("GenerateUUID() returned duplicate: %q", id)
		}

		seen[id] = true
	}
}

// Guards the exported const values: they are part of the public API
// (env var names, prefixes matched by consumers).
func TestConstValues(t *testing.T) {
	tests := []struct {
		name string
		got  interface{}
		want interface{}
	}{
		{name: "ErrorPrefix", got: ErrorPrefix, want: "[sypl] [Error]"},
		{name: "WarnPrefix", got: WarnPrefix, want: "[sypl] [Warn]"},
		{name: "DefaultComponentNameOutput", got: DefaultComponentNameOutput, want: "componentNameTest"},
		{name: "DefaultContentOutput", got: DefaultContentOutput, want: "contentTest"},
		{name: "DefaultFileMode", got: DefaultFileMode, want: 0o644},
		{name: "DefaultPrefixValue", got: DefaultPrefixValue, want: "My Prefix - "},
		{name: "DefaultTimestampFormat", got: DefaultTimestampFormat, want: "2006"},
		{name: "FilterEnvVar", got: FilterEnvVar, want: "SYPL_FILTER"},
		{name: "LevelEnvVar", got: LevelEnvVar, want: "SYPL_LEVEL"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s = %v, want %v", tt.name, tt.got, tt.want)
			}
		})
	}
}
