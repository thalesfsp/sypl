// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package sypl_test

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"

	"github.com/thalesfsp/sypl/v2"
	"github.com/thalesfsp/sypl/v2/debug"
	"github.com/thalesfsp/sypl/v2/elasticsearch"
	"github.com/thalesfsp/sypl/v2/fields"
	"github.com/thalesfsp/sypl/v2/flag"
	"github.com/thalesfsp/sypl/v2/formatter"
	"github.com/thalesfsp/sypl/v2/level"
	"github.com/thalesfsp/sypl/v2/message"
	"github.com/thalesfsp/sypl/v2/output"
	"github.com/thalesfsp/sypl/v2/processor"
	"github.com/thalesfsp/sypl/v2/shared"
)

// SkipAndMute is documented as "message will not be processed, neither
// printed" (flag/flag.go). A non-empty message flagged SkipAndMute must
// therefore produce no output.
func TestAudit_SkipAndMuteIsHonored(t *testing.T) {
	buf, o := output.SafeBuffer(level.Trace)

	l := sypl.New("testing", o)

	l.PrintWithOptions(level.Info, "hello", sypl.WithFlag(flag.SkipAndMute))

	if buf.String() != "" {
		t.Fatalf("SkipAndMute message was printed: %q", buf.String())
	}
}

// Strip removes trailing line breaks and Restore must re-append them in the
// original order: "x\r\n" must round-trip to "x\r\n", not "x\n\r".
func TestAudit_CRLFRoundTrip(t *testing.T) {
	m := message.New(level.Info, "Test 1\r\n")

	m.Strip()

	if m.GetContent().GetProcessed() != "Test 1" {
		t.Fatalf("Strip failed: %q", m.GetContent().GetProcessed())
	}

	m.Restore()

	if got := m.GetContent().GetProcessed(); got != "Test 1\r\n" {
		t.Fatalf("Restore reversed the CRLF: got %q, expected %q", got, "Test 1\r\n")
	}
}

// A message targeted at output "es-backup" must not also be written to an
// output named "es" (substring of the target).
func TestAudit_OutputDispatchExactMatch(t *testing.T) {
	bufES, oES := output.SafeBuffer(level.Trace)
	oES.SetName("es")

	bufBackup, oBackup := output.SafeBuffer(level.Trace)
	oBackup.SetName("es-backup")

	l := sypl.New("testing", oES, oBackup)

	l.PrintMessagesToOutputs(sypl.MessageToOutput{
		Content:    "hi\n",
		Level:      level.Info,
		OutputName: "es-backup",
	})

	if !strings.Contains(bufBackup.String(), "hi") {
		t.Fatalf(`targeted output "es-backup" did not receive the message`)
	}

	if bufES.String() != "" {
		t.Fatalf(`output "es" received a message targeted at "es-backup": %q`, bufES.String())
	}
}

// A processor named for a superstring must not trigger a processor whose name
// is a substring of it.
func TestAudit_ProcessorDispatchExactMatch(t *testing.T) {
	pre := processor.New("Pre", func(m message.IMessage) error {
		m.GetContent().SetProcessed("PRE-" + m.GetContent().GetProcessed())

		return nil
	})

	prefixer := processor.New("Prefixer", func(m message.IMessage) error {
		m.GetContent().SetProcessed("PREFIXER-" + m.GetContent().GetProcessed())

		return nil
	})

	buf, o := output.SafeBuffer(level.Trace, pre, prefixer)

	l := sypl.New("testing", o)

	l.PrintWithOptions(level.Info, "x\n", sypl.WithProcessorsNames("Prefixer"))

	if strings.Contains(buf.String(), "PRE-") {
		t.Fatalf(`processor "Pre" ran though only "Prefixer" was requested: %q`, buf.String())
	}

	if !strings.Contains(buf.String(), "PREFIXER-") {
		t.Fatalf(`requested processor "Prefixer" did not run: %q`, buf.String())
	}
}

// ChangeFirstCharCase must operate on the first rune, not the first byte.
func TestAudit_ChangeFirstCharCaseMultibyte(t *testing.T) {
	m := message.New(level.Info, "élan")
	m.GetContent().SetProcessed("élan")

	if err := processor.ChangeFirstCharCase(processor.Uppercase).Run(m); err != nil {
		t.Fatal(err)
	}

	if got := m.GetContent().GetProcessed(); got != "Élan" {
		t.Fatalf("multi-byte first char corrupted: got %q, expected %q", got, "Élan")
	}
}

// WithField after WithFields(nil) must neither panic nor drop the field.
func TestAudit_WithFieldNilGuard(t *testing.T) {
	m := message.New(level.Info, "x")

	m.SetFields(nil)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("WithField panicked on nil fields: %v", r)
		}
	}()

	sypl.WithField("k", "v")(m)

	if m.GetFields()["k"] != "v" {
		t.Fatalf("field lost: %+v", m.GetFields())
	}
}

// A SYPL_LEVEL entry scoped to component "infosvc" must not leak a global
// "info" level onto unrelated components ("info" is a prefix of "infosvc").
func TestAudit_DebugEnvVarNoPrefixLeak(t *testing.T) {
	t.Setenv(shared.LevelEnvVar, "infosvc:console:trace")

	d := debug.New("other", "file")

	if l, _, ok := d.Level(); ok {
		t.Fatalf("scoped entry leaked as global level %s", l)
	}
}

// Positive control for the fix: a bare level entry must keep working.
func TestAudit_DebugEnvVarBareLevelStillWorks(t *testing.T) {
	t.Setenv(shared.LevelEnvVar, "trace")

	d := debug.New("comp", "out")

	l, _, ok := d.Level()
	if !ok || l != level.Trace {
		t.Fatalf("bare level entry broken: level=%s ok=%v", l, ok)
	}
}

// message.Copy must deep-copy the fields map: mutating the copy's fields must
// not be visible in the original (copies are processed concurrently
// per-output).
func TestAudit_CopyDeepCopiesFields(t *testing.T) {
	m := message.New(level.Info, "x")
	m.SetFields(fields.Fields{"a": 1})

	c := message.Copy(m)

	c.GetFields()["b"] = 2

	if _, leaked := m.GetFields()["b"]; leaked {
		t.Fatal("message.Copy shares the fields map with the original")
	}
}

// The ES output must return an error, not panic, when the document "id" is
// not a string. The panic fires before any network call.
func TestAudit_ESWriteNonStringIDNoPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("ES Write panicked on non-string id: %v", r)
		}
	}()

	es := &elasticsearch.ElasticSearch{
		DynamicIndex: func() string { return "idx" },
	}

	// Client is nil: the call may error, but it must not panic on the
	// unchecked type assertion.
	//nolint:errcheck
	es.Write([]byte(`{"id":123}`))
}

// JSONPretty must register under its own name, not shadow JSON.
func TestAudit_JSONPrettyName(t *testing.T) {
	if formatter.JSONPretty().GetName() == formatter.JSON().GetName() {
		t.Fatalf("JSONPretty and JSON share the same name: %q", formatter.JSON().GetName())
	}
}

// Logging while reconfiguring (SetMaxLevel/SetTags/SetFields) is a normal
// runtime pattern and must be race-free. Run with -race.
func TestAudit_ReconfigureWhileLoggingRaceFree(t *testing.T) {
	_, o := output.SafeBuffer(level.Info)

	l := sypl.New("testing", o)

	var wg sync.WaitGroup

	wg.Add(2)

	go func() {
		defer wg.Done()

		for range 500 {
			l.Infoln("x")
		}
	}()

	go func() {
		defer wg.Done()

		for i := range 500 {
			l.SetMaxLevel(level.Debug)
			l.SetTags("t")
			l.SetFields(fields.Fields{"k": i})
		}
	}()

	wg.Wait()
}

// Two Fatal messages in one PrintMessage call must not data-race on the
// shared exit flag. The child process exits 1 (Fatal); under -race with
// halt_on_error it would exit 66 instead.
func TestAudit_DoubleFatalNoRace(t *testing.T) {
	if os.Getenv("SYPL_TEST_DOUBLE_FATAL") == "1" {
		_, o := output.SafeBuffer(level.Trace)

		l := sypl.New("testing", o)

		l.PrintMessage(
			message.New(level.Fatal, "a"),
			message.New(level.Fatal, "b"),
		)

		return
	}

	//nolint:gosec
	cmd := exec.Command(os.Args[0], "-test.run=TestAudit_DoubleFatalNoRace")
	cmd.Env = append(os.Environ(),
		"SYPL_TEST_DOUBLE_FATAL=1",
		"GORACE=halt_on_error=1 exitcode=66",
	)

	err := cmd.Run()

	var ee *exec.ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("expected the child to exit(1) via Fatal, got: %v", err)
	}

	if ee.ExitCode() == 66 {
		t.Fatal("data race detected in the double-Fatal path")
	}

	if ee.ExitCode() != 1 {
		t.Fatalf("unexpected child exit code: %d", ee.ExitCode())
	}
}
