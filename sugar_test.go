// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package sypl_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/thalesfsp/sypl/v2"
	"github.com/thalesfsp/sypl/v2/formatter"
	"github.com/thalesfsp/sypl/v2/level"
	"github.com/thalesfsp/sypl/v2/output"
	"github.com/thalesfsp/sypl/v2/safebuffer"
)

// sugarLogger builds a Trace-capped, JSON-formatted logger over a SafeBuffer
// - the fields land as top-level JSON keys, asserted end-to-end.
func sugarLogger(name string) (*safebuffer.Buffer, *sypl.Sypl) {
	buf, o := output.SafeBuffer(level.Trace)
	o.SetFormatter(formatter.JSON())

	return buf, sypl.New(name, o)
}

// sugarLine decodes the single JSON line in buf.
func sugarLine(t *testing.T, buf *safebuffer.Buffer) map[string]interface{} {
	t.Helper()

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")

	decoded := map[string]interface{}{}
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &decoded); err != nil {
		t.Fatalf("output is not valid JSON: %v: %q", err, buf.String())
	}

	return decoded
}

// Happy path: pairs become fields, message and level are intact.
func TestSugar_HappyPairs(t *testing.T) {
	buf, l := sugarLogger("sugar-happy")

	l.Infow("user logged in", "user", "thales", "attempt", 2)

	decoded := sugarLine(t, buf)

	if decoded["message"] != "user logged in" {
		t.Fatalf("message = %v, want %q", decoded["message"], "user logged in")
	}

	if decoded["level"] != "info" {
		t.Fatalf("level = %v, want info", decoded["level"])
	}

	if decoded["user"] != "thales" {
		t.Fatalf("field user = %v, want thales", decoded["user"])
	}

	// JSON numbers decode as float64.
	if decoded["attempt"] != float64(2) {
		t.Fatalf("field attempt = %v, want 2", decoded["attempt"])
	}
}

// Every leveled variant must log at ITS level.
func TestSugar_LeveledVariants(t *testing.T) {
	buf, l := sugarLogger("sugar-levels")

	tests := []struct {
		call  func()
		level string
	}{
		{func() { l.Tracew("m", "k", "v") }, "trace"},
		{func() { l.Debugw("m", "k", "v") }, "debug"},
		{func() { l.Infow("m", "k", "v") }, "info"},
		{func() { l.Warnw("m", "k", "v") }, "warn"},
		{func() { l.Errorw("m", "k", "v") }, "error"},
		{func() { l.Logw(level.Warn, "m", "k", "v") }, "warn"},
	}

	for _, tt := range tests {
		buf.Reset()

		tt.call()

		decoded := sugarLine(t, buf)

		if decoded["level"] != tt.level {
			t.Fatalf("level = %v, want %s", decoded["level"], tt.level)
		}

		if decoded["k"] != "v" {
			t.Fatalf("%s: field k = %v, want v", tt.level, decoded["k"])
		}
	}
}

// An odd trailing key still logs - its value becomes "(MISSING)". Never
// panics.
func TestSugar_OddTrailingKey(t *testing.T) {
	buf, l := sugarLogger("sugar-odd")

	l.Infow("odd pairs", "complete", "yes", "dangling")

	decoded := sugarLine(t, buf)

	if decoded["complete"] != "yes" {
		t.Fatalf("field complete = %v, want yes", decoded["complete"])
	}

	if decoded["dangling"] != "(MISSING)" {
		t.Fatalf("dangling key = %v, want (MISSING)", decoded["dangling"])
	}
}

// A non-string key becomes "!BADKEY<idx>" carrying the offending element -
// consuming ONE element, so the following pair still parses. Never panics.
func TestSugar_NonStringKey(t *testing.T) {
	buf, l := sugarLogger("sugar-badkey")

	l.Infow("bad key", 42, "after", "ok")

	decoded := sugarLine(t, buf)

	if decoded["!BADKEY0"] != float64(42) {
		t.Fatalf("!BADKEY0 = %v, want 42", decoded["!BADKEY0"])
	}

	if decoded["after"] != "ok" {
		t.Fatalf("field after = %v, want ok (recovery after bad key failed)", decoded["after"])
	}

	// A trailing non-string element is a bad key too.
	buf.Reset()

	l.Infow("trailing bad key", "k", "v", true)

	decoded = sugarLine(t, buf)

	if decoded["!BADKEY2"] != true {
		t.Fatalf("!BADKEY2 = %v, want true", decoded["!BADKEY2"])
	}
}

// Empty key-values: plain message, no extra fields, no panic.
func TestSugar_EmptyKeyValues(t *testing.T) {
	buf, l := sugarLogger("sugar-empty")

	l.Infow("no fields")

	decoded := sugarLine(t, buf)

	if decoded["message"] != "no fields" {
		t.Fatalf("message = %v, want %q", decoded["message"], "no fields")
	}

	for k := range decoded {
		if strings.HasPrefix(k, "!BADKEY") {
			t.Fatalf("unexpected synthetic field %q", k)
		}
	}
}

// The sugar respects the fast gate: a gated level never enters the pipeline.
func TestSugar_RespectsFastGate(t *testing.T) {
	t.Setenv("SYPL_LEVEL", "")
	t.Setenv("SYPL_FILTER", "")

	buf, o := output.SafeBuffer(level.Info)
	o.SetFormatter(formatter.JSON())

	l := sypl.New("sugar-gate", o).SetFastGate(true)

	l.Debugw("gated", "k", "v")

	if buf.String() != "" {
		t.Fatalf("gated Debugw produced output: %q", buf.String())
	}

	l.Infow("allowed", "k", "v")

	if !strings.Contains(buf.String(), "allowed") {
		t.Fatalf("allowed Infow lost: %q", buf.String())
	}
}

// Fatalw logs the message WITH fields, then exits 1 - asserted via
// subprocess re-exec.
func TestSugar_FatalwExits(t *testing.T) {
	if os.Getenv("SYPL_TEST_SUGAR_FATALW") == "1" {
		l := sypl.New("sugar-fatal", output.New("StdErr", level.Trace, os.Stderr).
			SetFormatter(formatter.JSON()))

		l.Fatalw("fatal with fields", "cause", "test")

		os.Exit(42) // Sentinel: Fatalw did not exit.
	}

	//nolint:gosec // Re-running the test binary itself.
	cmd := exec.Command(os.Args[0], "-test.run=TestSugar_FatalwExits$")

	cmd.Env = append(os.Environ(), "SYPL_TEST_SUGAR_FATALW=1")

	var stderr bytes.Buffer

	cmd.Stderr = &stderr

	err := cmd.Run()

	var exitErr *exec.ExitError

	if !errors.As(err, &exitErr) {
		t.Fatalf("expected subprocess to exit with an error, got %v (stderr: %s)", err, stderr.String())
	}

	if code := exitErr.ExitCode(); code != 1 {
		t.Errorf("expected exit code 1 (Fatalw), got %d (stderr: %s)", code, stderr.String())
	}

	if !strings.Contains(stderr.String(), "fatal with fields") ||
		!strings.Contains(stderr.String(), `"cause":"test"`) {
		t.Errorf("Fatalw did not log message+fields before exit, stderr: %s", stderr.String())
	}
}
