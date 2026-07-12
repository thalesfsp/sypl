// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package builtin

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// Panic must write the message, then panic with it.
func TestPanic(t *testing.T) {
	var b bytes.Buffer

	l := NewBuiltin(&b, "", 0)

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected Panic to panic")
		}

		if r != "panic-msg" {
			t.Fatalf("recovered %v, expected panic-msg", r)
		}

		if b.String() != "panic-msg" {
			t.Fatalf("written %q, expected panic-msg", b.String())
		}
	}()

	l.Panic("panic-msg")
}

// Panicf must format, write, then panic with the formatted message.
func TestPanicf(t *testing.T) {
	var b bytes.Buffer

	l := NewBuiltin(&b, "", 0)

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected Panicf to panic")
		}

		if r != "panic-42" {
			t.Fatalf("recovered %v, expected panic-42", r)
		}

		if b.String() != "panic-42" {
			t.Fatalf("written %q, expected panic-42", b.String())
		}
	}()

	l.Panicf("panic-%d", 42)
}

// Panicln must append a new line, write, then panic with the message.
func TestPanicln(t *testing.T) {
	var b bytes.Buffer

	l := NewBuiltin(&b, "", 0)

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected Panicln to panic")
		}

		if r != "panic-ln\n" {
			t.Fatalf("recovered %q, expected %q", r, "panic-ln\n")
		}

		if b.String() != "panic-ln\n" {
			t.Fatalf("written %q, expected %q", b.String(), "panic-ln\n")
		}
	}()

	l.Panicln("panic-ln")
}

// OutputBuiltin with an unrecoverable call depth must fall back to "???":0
// instead of failing.
func TestOutputBuiltinUnknownCaller(t *testing.T) {
	var b bytes.Buffer

	l := NewBuiltin(&b, "", Lshortfile)

	if err := l.OutputBuiltin(9999, "deep"); err != nil {
		t.Fatal(err)
	}

	if got := b.String(); got != "???:0: deep" {
		t.Fatalf("got %q, expected %q", got, "???:0: deep")
	}
}

// SetOutput must redirect subsequent writes to the new writer.
func TestSetOutput(t *testing.T) {
	var before, after bytes.Buffer

	l := NewBuiltin(&before, "", 0)

	l.Print("first")

	l.SetOutput(&after)

	l.Print("second")

	if before.String() != "first" {
		t.Fatalf("original writer got %q, expected only %q", before.String(), "first")
	}

	if after.String() != "second" {
		t.Fatalf("new writer got %q, expected %q", after.String(), "second")
	}
}

// Every Fatal variant must write the message and exit with os.Exit(1)
// (subprocess re-exec pattern - os.Exit cannot be intercepted in-process).
func TestFatalVariantsSubprocess(t *testing.T) {
	variant := os.Getenv("SYPL_BUILTIN_TEST_FATAL")
	if variant != "" {
		l := NewBuiltin(os.Stdout, "", 0)

		switch variant {
		case "fatal":
			l.Fatal("fatal-plain")
		case "fatalf":
			l.Fatalf("fatal-%d", 42)
		case "fatalln":
			l.Fatalln("fatal-ln")
		}

		// Unreachable: every branch above exits 1.
		os.Exit(42)
	}

	tests := []struct {
		variant string
		want    string
	}{
		{"fatal", "fatal-plain"},
		{"fatalf", "fatal-42"},
		{"fatalln", "fatal-ln\n"},
	}

	for _, tt := range tests {
		t.Run(tt.variant, func(t *testing.T) {
			//nolint:gosec
			cmd := exec.Command(os.Args[0], "-test.run=TestFatalVariantsSubprocess$")
			cmd.Env = append(os.Environ(), "SYPL_BUILTIN_TEST_FATAL="+tt.variant)

			out, err := cmd.Output()

			var ee *exec.ExitError
			if !errors.As(err, &ee) {
				t.Fatalf("expected the child to exit(1) via Fatal, got: %v", err)
			}

			if ee.ExitCode() != 1 {
				t.Fatalf("child exit code = %d, expected 1 (os.Exit(1))", ee.ExitCode())
			}

			if !strings.Contains(string(out), tt.want) {
				t.Fatalf("child stdout %q missing %q", string(out), tt.want)
			}
		})
	}
}
