// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package builtin

import (
	"bytes"
	"errors"
	"regexp"
	"testing"
)

// errorWriter always fails, allowing to exercise the write-error path.
type errorWriter struct {
	err error
}

func (w errorWriter) Write(p []byte) (int, error) {
	return 0, w.err
}

// TestOutputBuiltinNoForcedNewline verifies the package's raison d'être: the
// fork must NOT force-append a newline to the message.
//
// See this https://github.com/golang/go/issues/16564 for more info.
func TestOutputBuiltinNoForcedNewline(t *testing.T) {
	var b bytes.Buffer

	l := NewBuiltin(&b, "", 0)

	if err := l.OutputBuiltin(DefaultCallDepth, "no newline"); err != nil {
		t.Fatal(err)
	}

	if got := b.String(); got != "no newline" {
		t.Errorf("expected %q got %q", "no newline", got)
	}

	// Print must not append a newline either.
	b.Reset()

	l.Print("still no newline")

	if got := b.String(); got != "still no newline" {
		t.Errorf("expected %q got %q", "still no newline", got)
	}
}

// TestOutputBuiltinEmptyString verifies the empty-content edge case: nothing
// is written - no newline, no padding - unless a prefix is set.
func TestOutputBuiltinEmptyString(t *testing.T) {
	var b bytes.Buffer

	l := NewBuiltin(&b, "", 0)

	if err := l.OutputBuiltin(DefaultCallDepth, ""); err != nil {
		t.Fatal(err)
	}

	if got := b.String(); got != "" {
		t.Errorf("expected empty output got %q", got)
	}

	// With a prefix, only the prefix is written.
	b.Reset()
	l.SetPrefix("pfx:")

	if err := l.OutputBuiltin(DefaultCallDepth, ""); err != nil {
		t.Fatal(err)
	}

	if got := b.String(); got != "pfx:" {
		t.Errorf("expected %q got %q", "pfx:", got)
	}
}

// TestOutputBuiltinWithFlagsAndPrefix verifies header composition: prefix
// before the header, and - with Lmsgprefix - between the header and the
// message.
func TestOutputBuiltinWithFlagsAndPrefix(t *testing.T) {
	var b bytes.Buffer

	l := NewBuiltin(&b, "pfx ", Ldate|Ltime)

	if err := l.OutputBuiltin(DefaultCallDepth, "msg"); err != nil {
		t.Fatal(err)
	}

	pattern := "^pfx " + Rdate + " " + Rtime + " msg$"
	if matched, err := regexp.MatchString(pattern, b.String()); err != nil || !matched {
		t.Errorf("output %q should match %q (err: %v)", b.String(), pattern, err)
	}

	// Lmsgprefix moves the prefix to just before the message.
	b.Reset()
	l.SetFlags(Ldate | Ltime | Lmsgprefix)

	if err := l.OutputBuiltin(DefaultCallDepth, "msg"); err != nil {
		t.Fatal(err)
	}

	pattern = "^" + Rdate + " " + Rtime + " pfx msg$"
	if matched, err := regexp.MatchString(pattern, b.String()); err != nil || !matched {
		t.Errorf("output %q should match %q (err: %v)", b.String(), pattern, err)
	}
}

// TestOutputBuiltinShortfile verifies the caller file/line header.
func TestOutputBuiltinShortfile(t *testing.T) {
	var b bytes.Buffer

	l := NewBuiltin(&b, "", Lshortfile)

	// Calldepth 1 points at this call site.
	if err := l.OutputBuiltin(1, "msg"); err != nil {
		t.Fatal(err)
	}

	pattern := `^builtin_surface_test\.go:[0-9]+: msg$`
	if matched, err := regexp.MatchString(pattern, b.String()); err != nil || !matched {
		t.Errorf("output %q should match %q (err: %v)", b.String(), pattern, err)
	}
}

// TestOutputBuiltinWriteError verifies the bad path: a writer failure is
// propagated to the caller.
func TestOutputBuiltinWriteError(t *testing.T) {
	errWrite := errors.New("write failed")

	l := NewBuiltin(errorWriter{err: errWrite}, "", 0)

	if err := l.OutputBuiltin(DefaultCallDepth, "msg"); !errors.Is(err, errWrite) {
		t.Errorf("expected %v got %v", errWrite, err)
	}
}

// TestSetOutputSwitchesWriter verifies SetOutput redirects subsequent writes,
// and Writer reports the current destination.
func TestSetOutputSwitchesWriter(t *testing.T) {
	var b1, b2 bytes.Buffer

	l := NewBuiltin(&b1, "", 0)

	l.Print("first")

	l.SetOutput(&b2)

	if l.Writer() != &b2 {
		t.Error("Writer should report the new destination")
	}

	l.Print("second")

	if b1.String() != "first" {
		t.Errorf("expected %q got %q", "first", b1.String())
	}

	if b2.String() != "second" {
		t.Errorf("expected %q got %q", "second", b2.String())
	}
}
