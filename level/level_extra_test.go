// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package level

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"

	"github.com/thalesfsp/sypl/shared"
)

func TestLevel_String(t *testing.T) {
	tests := []struct {
		name string
		l    Level
		want string
	}{
		{name: "None", l: None, want: "none"},
		{name: "Fatal", l: Fatal, want: "fatal"},
		{name: "Error", l: Error, want: "error"},
		{name: "Info", l: Info, want: "info"},
		{name: "Warn", l: Warn, want: "warn"},
		{name: "Debug", l: Debug, want: "debug"},
		{name: "Trace", l: Trace, want: "trace"},
		{name: "Out-of-range - negative", l: Level(-1), want: "Unknown"},
		{name: "Out-of-range - Trace + 1", l: Trace + 1, want: "Unknown"},
		{name: "Out-of-range - big positive", l: Level(100), want: "Unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.l.String(); got != tt.want {
				t.Errorf("Level.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFromInt_Extra(t *testing.T) {
	type args struct {
		level int
	}
	tests := []struct {
		name       string
		args       args
		want       Level
		wantString string
	}{
		{
			name:       "Should work - lower boundary",
			args:       args{level: 0},
			want:       None,
			wantString: "none",
		},
		{
			name:       "Should work - upper boundary",
			args:       args{level: 6},
			want:       Trace,
			wantString: "trace",
		},
		{
			name:       "Should work - negative maps to Unknown",
			args:       args{level: -1},
			want:       Level(-1),
			wantString: "Unknown",
		},
		{
			name:       "Should work - too big maps to Unknown",
			args:       args{level: 100},
			want:       Level(100),
			wantString: "Unknown",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FromInt(tt.args.level)

			if got != tt.want {
				t.Errorf("FromInt() = %v, want %v", got, tt.want)
			}

			if got.String() != tt.wantString {
				t.Errorf("FromInt().String() = %v, want %v", got.String(), tt.wantString)
			}
		})
	}
}

func TestFromString_AllLevels(t *testing.T) {
	tests := []struct {
		name  string
		level string
		want  Level
	}{
		{name: "none - lower", level: "none", want: None},
		{name: "fatal - lower", level: "fatal", want: Fatal},
		{name: "error - lower", level: "error", want: Error},
		{name: "info - lower", level: "info", want: Info},
		{name: "warn - lower", level: "warn", want: Warn},
		{name: "debug - lower", level: "debug", want: Debug},
		{name: "trace - lower", level: "trace", want: Trace},
		{name: "TRACE - upper", level: "TRACE", want: Trace},
		{name: "DeBuG - mixed", level: "DeBuG", want: Debug},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FromString(tt.level)
			if err != nil {
				t.Fatalf("FromString(%q) unexpected error: %v", tt.level, err)
			}

			if got != tt.want {
				t.Errorf("FromString(%q) = %v, want %v", tt.level, got, tt.want)
			}
		})
	}
}

func TestFromString_Garbage(t *testing.T) {
	tests := []struct {
		name  string
		level string
	}{
		{name: "garbage word", level: "garbage"},
		{name: "whitespace only", level: "   "},
		{name: "level with trailing space", level: "info "},
		{name: "numeric", level: "3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FromString(tt.level)

			if got != None {
				t.Errorf("FromString(%q) = %v, want %v", tt.level, got, None)
			}

			if !errors.Is(err, ErrInvalidLevel) {
				t.Errorf("FromString(%q) error = %v, want ErrInvalidLevel", tt.level, err)
			}
		})
	}
}

// MustFromString exits the process (log.Fatalf) on failure, so the failure
// paths are asserted by re-running this test binary as a subprocess, and
// checking it dies with exit code 1, and the expected message.
func TestMustFromString_Fatal(t *testing.T) {
	// Subprocess mode: call the function under test, and nothing else. If
	// MustFromString doesn't exit, exit with a sentinel code so the parent
	// fails the assertion.
	if levelArg, ok := os.LookupEnv("SYPL_TEST_MUSTFROMSTRING_LEVEL"); ok {
		MustFromString(levelArg)

		os.Exit(42)
	}

	tests := []struct {
		name    string
		level   string
		wantMsg string
	}{
		{
			name:    "Should exit - empty string",
			level:   "",
			wantMsg: "No level specified",
		},
		{
			name:    "Should exit - garbage",
			level:   "not-a-level",
			wantMsg: "Invalid level: not-a-level",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			//nolint:gosec // Re-running the test binary itself.
			cmd := exec.Command(os.Args[0], "-test.run=TestMustFromString_Fatal$")

			cmd.Env = append(os.Environ(), "SYPL_TEST_MUSTFROMSTRING_LEVEL="+tt.level)

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

			if !strings.Contains(stderr.String(), shared.ErrorPrefix) {
				t.Errorf("Expected stderr to contain %q, got %q", shared.ErrorPrefix, stderr.String())
			}
		})
	}
}

func TestLevelsToString_Extra(t *testing.T) {
	type args struct {
		levels []Level
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "Should work - empty slice",
			args: args{levels: []Level{}},
			want: "",
		},
		{
			name: "Should work - nil slice",
			args: args{levels: nil},
			want: "",
		},
		{
			name: "Should work - single",
			args: args{levels: []Level{Trace}},
			want: "trace",
		},
		{
			name: "Should work - all levels, in order",
			args: args{levels: []Level{None, Fatal, Error, Info, Warn, Debug, Trace}},
			want: "none,fatal,error,info,warn,debug,trace",
		},
		{
			name: "Should work - mixed valid, and out-of-range",
			args: args{levels: []Level{Info, Level(-1)}},
			want: "info,Unknown",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := LevelsToString(tt.args.levels); got != tt.want {
				t.Errorf("LevelsToString() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLevelsNames_Completeness(t *testing.T) {
	want := []string{"none", "fatal", "error", "info", "warn", "debug", "trace"}

	got := LevelsNames()

	if !reflect.DeepEqual(got, want) {
		t.Errorf("LevelsNames() = %v, want %v", got, want)
	}

	// Every name must round-trip through FromString back to its Level.
	for i, name := range got {
		l, err := FromString(name)
		if err != nil {
			t.Errorf("FromString(%q) unexpected error: %v", name, err)

			continue
		}

		if l != Level(i) {
			t.Errorf("FromString(%q) = %v, want %v", name, l, Level(i))
		}
	}
}
