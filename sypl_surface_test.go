// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package sypl_test

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"

	"github.com/thalesfsp/sypl/v2"
	"github.com/thalesfsp/sypl/v2/fields"
	"github.com/thalesfsp/sypl/v2/flag"
	"github.com/thalesfsp/sypl/v2/formatter"
	"github.com/thalesfsp/sypl/v2/level"
	"github.com/thalesfsp/sypl/v2/message"
	"github.com/thalesfsp/sypl/v2/options"
	"github.com/thalesfsp/sypl/v2/output"
	"github.com/thalesfsp/sypl/v2/processor"
	"github.com/thalesfsp/sypl/v2/safebuffer"
	"github.com/thalesfsp/sypl/v2/shared"
	"github.com/thalesfsp/sypl/v2/status"
)

//////
// io.Writer implementation.
//////

// Write must print nothing at the default io.Writer level (none), print after
// SetDefaultIoWriterLevel, and always return 0, nil.
func TestSypl_Write(t *testing.T) {
	buf, o := output.SafeBuffer(level.Trace)

	l := sypl.New("writer", o)

	// Default io.Writer level is `none` - must not print.
	n, err := l.Write([]byte("hidden\n"))
	if n != 0 || err != nil {
		t.Fatalf("Write returned (%d, %v), expected (0, nil)", n, err)
	}

	if buf.String() != "" {
		t.Fatalf("Write at default (none) level printed: %q", buf.String())
	}

	if l.GetDefaultIoWriterLevel() != level.None {
		t.Fatalf("default io.Writer level = %s, expected none", l.GetDefaultIoWriterLevel())
	}

	// After raising the level, it must print.
	l.SetDefaultIoWriterLevel(level.Info)

	if l.GetDefaultIoWriterLevel() != level.Info {
		t.Fatalf("io.Writer level = %s, expected info", l.GetDefaultIoWriterLevel())
	}

	n, err = l.Write([]byte("visible\n"))
	if n != 0 || err != nil {
		t.Fatalf("Write returned (%d, %v), expected (0, nil)", n, err)
	}

	if buf.String() != "visible\n" {
		t.Fatalf("Write at info level printed %q, expected %q", buf.String(), "visible\n")
	}
}

//////
// PrintPretty / PrintlnPretty.
//////

type prettyData struct {
	Exported string

	unexported string
}

// PrintPretty must print the JSON version of the data structure, dropping
// unexported fields, and must NOT run any processor (flag.Skip).
func TestSypl_PrintPretty(t *testing.T) {
	buf, o := output.SafeBuffer(level.Trace, processor.Prefixer("MUSTNOTRUN-"))

	l := sypl.New("pretty", o)

	l.PrintPretty(level.Info, prettyData{Exported: "visibleValue", unexported: "hiddenValue"})

	got := buf.String()

	if !strings.Contains(got, `"Exported": "visibleValue"`) {
		t.Fatalf("PrintPretty missing exported field: %q", got)
	}

	if strings.Contains(got, "hiddenValue") || strings.Contains(got, "unexported") {
		t.Fatalf("PrintPretty leaked unexported field: %q", got)
	}

	if strings.Contains(got, "MUSTNOTRUN-") {
		t.Fatalf("PrintPretty processed the message (flag.Skip violated): %q", got)
	}
}

// PrintlnPretty behaves like PrintPretty, adding a new line to the end.
func TestSypl_PrintlnPretty(t *testing.T) {
	buf, o := output.SafeBuffer(level.Trace, processor.Prefixer("MUSTNOTRUN-"))

	l := sypl.New("pretty", o)

	l.PrintlnPretty(level.Info, prettyData{Exported: "visibleValue", unexported: "hiddenValue"})

	got := buf.String()

	if !strings.Contains(got, `"Exported": "visibleValue"`) {
		t.Fatalf("PrintlnPretty missing exported field: %q", got)
	}

	if strings.Contains(got, "hiddenValue") {
		t.Fatalf("PrintlnPretty leaked unexported field: %q", got)
	}

	if strings.Contains(got, "MUSTNOTRUN-") {
		t.Fatalf("PrintlnPretty processed the message (flag.Skip violated): %q", got)
	}

	// Prettify ends with "\n"; Sprintln appends another.
	if !strings.HasSuffix(got, "\n\n") {
		t.Fatalf("PrintlnPretty did not add a new line: %q", got)
	}
}

//////
// Serror family.
//////

// The Serror family must return an error carrying the NON-processed content,
// while the printed message IS processed.
func TestSypl_SerrorFamily(t *testing.T) {
	tests := []struct {
		name      string
		call      func(l *sypl.Sypl) error
		wantErr   string
		wantPrint string
	}{
		{
			name:      "Serror",
			call:      func(l *sypl.Sypl) error { return l.Serror("boom") },
			wantErr:   "boom",
			wantPrint: "PFX-boom",
		},
		{
			name:      "Serrorf",
			call:      func(l *sypl.Sypl) error { return l.Serrorf("code-%d", 7) },
			wantErr:   "code-7",
			wantPrint: "PFX-code-7",
		},
		{
			name:      "Serrorlnf",
			call:      func(l *sypl.Sypl) error { return l.Serrorlnf("code-%d", 7) },
			wantErr:   "code-7\n",
			wantPrint: "PFX-code-7\n",
		},
		{
			name:      "Serrorln",
			call:      func(l *sypl.Sypl) error { return l.Serrorln("boom") },
			wantErr:   "boom\n",
			wantPrint: "PFX-boom\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf, o := output.SafeBuffer(level.Trace, processor.Prefixer("PFX-"))

			l := sypl.New("serror", o)

			err := tt.call(l)
			if err == nil {
				t.Fatal("expected a non-nil error")
			}

			if err.Error() != tt.wantErr {
				t.Fatalf("error = %q, expected non-processed %q", err.Error(), tt.wantErr)
			}

			if buf.String() != tt.wantPrint {
				t.Fatalf("printed %q, expected processed %q", buf.String(), tt.wantPrint)
			}
		})
	}
}

//////
// Full leveled matrix.
//////

// Every leveled printer ({Error,Info,Warn,Debug,Trace} x {plain,f,ln,lnf})
// must print its content against an output @ Trace.
func TestSypl_LeveledMatrix(t *testing.T) {
	type variant struct {
		suffix string
		call   func(l *sypl.Sypl)
		want   string
	}

	matrix := map[string][]variant{
		"Error": {
			{"plain", func(l *sypl.Sypl) { l.Error("msg") }, "msg"},
			{"f", func(l *sypl.Sypl) { l.Errorf("%s-%d", "m", 1) }, "m-1"},
			{"ln", func(l *sypl.Sypl) { l.Errorln("msg") }, "msg\n"},
			{"lnf", func(l *sypl.Sypl) { l.Errorlnf("%s-%d", "m", 1) }, "m-1\n"},
		},
		"Info": {
			{"plain", func(l *sypl.Sypl) { l.Info("msg") }, "msg"},
			{"f", func(l *sypl.Sypl) { l.Infof("%s-%d", "m", 1) }, "m-1"},
			{"ln", func(l *sypl.Sypl) { l.Infoln("msg") }, "msg\n"},
			{"lnf", func(l *sypl.Sypl) { l.Infolnf("%s-%d", "m", 1) }, "m-1\n"},
		},
		"Warn": {
			{"plain", func(l *sypl.Sypl) { l.Warn("msg") }, "msg"},
			{"f", func(l *sypl.Sypl) { l.Warnf("%s-%d", "m", 1) }, "m-1"},
			{"ln", func(l *sypl.Sypl) { l.Warnln("msg") }, "msg\n"},
			{"lnf", func(l *sypl.Sypl) { l.Warnlnf("%s-%d", "m", 1) }, "m-1\n"},
		},
		"Debug": {
			{"plain", func(l *sypl.Sypl) { l.Debug("msg") }, "msg"},
			{"f", func(l *sypl.Sypl) { l.Debugf("%s-%d", "m", 1) }, "m-1"},
			{"ln", func(l *sypl.Sypl) { l.Debugln("msg") }, "msg\n"},
			{"lnf", func(l *sypl.Sypl) { l.Debuglnf("%s-%d", "m", 1) }, "m-1\n"},
		},
		"Trace": {
			{"plain", func(l *sypl.Sypl) { l.Trace("msg") }, "msg"},
			{"f", func(l *sypl.Sypl) { l.Tracef("%s-%d", "m", 1) }, "m-1"},
			{"ln", func(l *sypl.Sypl) { l.Traceln("msg") }, "msg\n"},
			{"lnf", func(l *sypl.Sypl) { l.Tracelnf("%s-%d", "m", 1) }, "m-1\n"},
		},
	}

	for lvl, variants := range matrix {
		for _, v := range variants {
			t.Run(lvl+"_"+v.suffix, func(t *testing.T) {
				buf, o := output.SafeBuffer(level.Trace)

				l := sypl.New("matrix", o)

				v.call(l)

				if buf.String() != v.want {
					t.Fatalf("got %q, expected %q", buf.String(), v.want)
				}
			})
		}
	}
}

//////
// Max level management.
//////

// GetMaxLevel/SetMaxLevel must read/write the max level of ALL outputs.
func TestSypl_GetSetMaxLevel(t *testing.T) {
	_, oA := output.SafeBuffer(level.Info)
	oA.SetName("A")

	_, oB := output.SafeBuffer(level.Debug)
	oB.SetName("B")

	l := sypl.New("maxlevel", oA, oB)

	want := map[string]level.Level{"A": level.Info, "B": level.Debug}
	if got := l.GetMaxLevel(); !reflect.DeepEqual(got, want) {
		t.Fatalf("GetMaxLevel = %v, expected %v", got, want)
	}

	l.SetMaxLevel(level.Warn)

	want = map[string]level.Level{"A": level.Warn, "B": level.Warn}
	if got := l.GetMaxLevel(); !reflect.DeepEqual(got, want) {
		t.Fatalf("after SetMaxLevel, GetMaxLevel = %v, expected %v", got, want)
	}
}

// AnyMaxLevel must find levels set at output creation, and honor the
// SYPL_LEVEL env var for runtime changes.
func TestSypl_AnyMaxLevel(t *testing.T) {
	_, o := output.SafeBuffer(level.Info)

	l := sypl.New("anymaxlevel", o)

	if !l.AnyMaxLevel(level.Info) {
		t.Fatal("AnyMaxLevel(info) = false, expected true (output @ info)")
	}

	if l.AnyMaxLevel(level.Trace) {
		t.Fatal("AnyMaxLevel(trace) = true, expected false")
	}

	// Env var branch: no output @ trace, but SYPL_LEVEL says trace.
	t.Setenv(shared.LevelEnvVar, "trace")

	if !l.AnyMaxLevel(level.Trace) {
		t.Fatal("AnyMaxLevel(trace) = false, expected true via SYPL_LEVEL")
	}
}

//////
// Output management.
//////

// GetOutput must be case-insensitive, and return nil when not found.
func TestSypl_GetOutput(t *testing.T) {
	_, o := output.SafeBuffer(level.Info)

	l := sypl.New("getoutput", o)

	if got := l.GetOutput("Buffer"); got == nil {
		t.Fatal("GetOutput(Buffer) = nil, expected the output")
	}

	// Case-insensitive.
	if got := l.GetOutput("bUfFeR"); got == nil {
		t.Fatal("GetOutput(bUfFeR) = nil, expected case-insensitive match")
	}

	if got := l.GetOutput("ghost"); got != nil {
		t.Fatalf("GetOutput(ghost) = %v, expected nil", got)
	}
}

// SetOutputs must replace outputs by name, and silently ignore unknown names.
func TestSypl_SetOutputs(t *testing.T) {
	_, oA := output.SafeBuffer(level.Info)
	oA.SetName("A")

	_, oB := output.SafeBuffer(level.Info)
	oB.SetName("B")

	l := sypl.New("setoutputs", oA, oB)

	// Replace "A" with an output at a different max level.
	_, oA2 := output.SafeBuffer(level.Trace)
	oA2.SetName("A")

	l.SetOutputs(oA2)

	if got := l.GetOutput("A").GetMaxLevel(); got != level.Trace {
		t.Fatalf("SetOutputs did not replace output A: max level = %s", got)
	}

	if len(l.GetOutputs()) != 2 {
		t.Fatalf("SetOutputs changed the output count: %d", len(l.GetOutputs()))
	}

	// Unknown name: ignored, not added.
	_, oGhost := output.SafeBuffer(level.Info)
	oGhost.SetName("Ghost")

	l.SetOutputs(oGhost)

	if got := l.GetOutput("Ghost"); got != nil {
		t.Fatal("SetOutputs added an unknown output instead of ignoring it")
	}

	if len(l.GetOutputs()) != 2 {
		t.Fatalf("SetOutputs with unknown name changed the output count: %d", len(l.GetOutputs()))
	}
}

// GetOutputsNames/AddOutputs must reflect registered outputs.
func TestSypl_GetOutputsNamesAndAddOutputs(t *testing.T) {
	_, oA := output.SafeBuffer(level.Info)
	oA.SetName("A")

	l := sypl.New("names", oA)

	if got := l.GetOutputsNames(); !reflect.DeepEqual(got, []string{"A"}) {
		t.Fatalf("GetOutputsNames = %v, expected [A]", got)
	}

	_, oB := output.SafeBuffer(level.Info)
	oB.SetName("B")

	l.AddOutputs(oB)

	if got := l.GetOutputsNames(); !reflect.DeepEqual(got, []string{"A", "B"}) {
		t.Fatalf("after AddOutputs, GetOutputsNames = %v, expected [A B]", got)
	}
}

//////
// Meta getters/setters.
//////

func TestSypl_MetaGettersSetters(t *testing.T) {
	l := sypl.New("meta")

	// Name + String().
	if l.GetName() != "meta" || l.String() != "meta" {
		t.Fatalf("GetName/String = %q/%q, expected meta", l.GetName(), l.String())
	}

	l.SetName("renamed")

	if l.GetName() != "renamed" || l.String() != "renamed" {
		t.Fatalf("after SetName, GetName/String = %q/%q, expected renamed", l.GetName(), l.String())
	}

	// Status: factory default is enabled.
	if l.GetStatus() != status.Enabled {
		t.Fatalf("GetStatus = %s, expected enabled", l.GetStatus())
	}

	l.SetStatus(status.Disabled)

	if l.GetStatus() != status.Disabled {
		t.Fatalf("after SetStatus, GetStatus = %s, expected disabled", l.GetStatus())
	}

	// Fields.
	f := fields.Fields{"k": "v"}

	l.SetFields(f)

	if got := l.GetFields(); !reflect.DeepEqual(got, f) {
		t.Fatalf("GetFields = %v, expected %v", got, f)
	}

	// Tags: SetTags appends.
	l.SetTags("t1", "t2")

	if got := l.GetTags(); !reflect.DeepEqual(got, []string{"t1", "t2"}) {
		t.Fatalf("GetTags = %v, expected [t1 t2]", got)
	}

	l.SetTags("t3")

	if got := l.GetTags(); !reflect.DeepEqual(got, []string{"t1", "t2", "t3"}) {
		t.Fatalf("SetTags must append: GetTags = %v, expected [t1 t2 t3]", got)
	}
}

//////
// Child logger.
//////

// New(name) on a logger must create a child inheriting outputs, fields, tags,
// status, and the default io.Writer level - and the child must log.
func TestSypl_ChildLogger(t *testing.T) {
	buf, o := output.SafeBuffer(level.Trace)

	parent := sypl.New("parent", o)
	parent.SetFields(fields.Fields{"pk": "pv"})
	parent.SetTags("ptag")
	parent.SetStatus(status.Disabled)
	parent.SetDefaultIoWriterLevel(level.Debug)

	child := parent.New("child")

	if child.GetName() != "child" {
		t.Fatalf("child name = %q, expected child", child.GetName())
	}

	if !reflect.DeepEqual(child.GetOutputs(), parent.GetOutputs()) {
		t.Fatal("child did not inherit the outputs")
	}

	if !reflect.DeepEqual(child.GetFields(), fields.Fields{"pk": "pv"}) {
		t.Fatalf("child fields = %v, expected map[pk:pv]", child.GetFields())
	}

	if !reflect.DeepEqual(child.GetTags(), []string{"ptag"}) {
		t.Fatalf("child tags = %v, expected [ptag]", child.GetTags())
	}

	if child.GetStatus() != status.Disabled {
		t.Fatalf("child status = %s, expected disabled", child.GetStatus())
	}

	if child.GetDefaultIoWriterLevel() != level.Debug {
		t.Fatalf("child io.Writer level = %s, expected debug", child.GetDefaultIoWriterLevel())
	}

	// Logging via the child must reach the inherited output.
	child.Infoln("from-child")

	if !strings.Contains(buf.String(), "from-child") {
		t.Fatalf("child logging did not reach the inherited output: %q", buf.String())
	}
}

//////
// PrintMessagesToOutputs.
//////

// Each message goes only to its named output; unknown outputs drop the
// message silently.
func TestSypl_PrintMessagesToOutputs(t *testing.T) {
	bufA, oA := output.SafeBuffer(level.Trace)
	oA.SetName("A")

	bufB, oB := output.SafeBuffer(level.Trace)
	oB.SetName("B")

	l := sypl.New("routing", oA, oB)

	l.PrintMessagesToOutputs(
		sypl.MessageToOutput{Content: "to-a\n", Level: level.Info, OutputName: "A"},
		sypl.MessageToOutput{Content: "to-b\n", Level: level.Info, OutputName: "B"},
		sypl.MessageToOutput{Content: "to-ghost\n", Level: level.Info, OutputName: "Ghost"},
	)

	if bufA.String() != "to-a\n" {
		t.Fatalf("output A got %q, expected %q", bufA.String(), "to-a\n")
	}

	if bufB.String() != "to-b\n" {
		t.Fatalf("output B got %q, expected %q", bufB.String(), "to-b\n")
	}
}

// Options must be merged into every message: flag, fields, tags, and
// outputs/processors names. An unknown output name drops silently.
func TestSypl_PrintMessagesToOutputsWithOptions(t *testing.T) {
	// Force flag: message @ debug against an output @ info must print.
	bufA, oA := output.SafeBuffer(level.Info)
	oA.SetName("A")

	l := sypl.New("optrouting", oA)

	l.PrintMessagesToOutputsWithOptions(
		&options.Options{Flag: flag.Force},
		sypl.MessageToOutput{Content: "forced\n", Level: level.Debug, OutputName: "A"},
	)

	if bufA.String() != "forced\n" {
		t.Fatalf("forced message not printed: %q", bufA.String())
	}

	// Fields + tags + processors names merged - observed via JSON formatter.
	bufB, oB := output.SafeBuffer(level.Trace, processor.Prefixer("SELECTED-"), processor.Suffixer("-MUSTNOTRUN"))
	oB.SetName("B")
	oB.SetFormatter(formatter.JSON())

	lb := sypl.New("optmerge", oB)

	lb.PrintMessagesToOutputsWithOptions(
		&options.Options{
			Fields:          fields.Fields{"fkey": "fval"},
			ProcessorsNames: []string{"Prefixer"},
			Tags:            []string{"opt-tag"},
		},
		sypl.MessageToOutput{Content: "payload\n", Level: level.Info, OutputName: "B"},
	)

	got := bufB.String()

	if !strings.Contains(got, `"fkey":"fval"`) {
		t.Fatalf("options fields not merged: %q", got)
	}

	if !strings.Contains(got, "opt-tag") {
		t.Fatalf("options tags not merged: %q", got)
	}

	if !strings.Contains(got, "SELECTED-payload") {
		t.Fatalf("selected processor did not run: %q", got)
	}

	if strings.Contains(got, "-MUSTNOTRUN") {
		t.Fatalf("non-selected processor ran: %q", got)
	}

	// Options OutputsNames take precedence over the per-message output name.
	bufC, oC := output.SafeBuffer(level.Trace)
	oC.SetName("C")

	bufD, oD := output.SafeBuffer(level.Trace)
	oD.SetName("D")

	lc := sypl.New("optprecedence", oC, oD)

	lc.PrintMessagesToOutputsWithOptions(
		&options.Options{OutputsNames: []string{"D"}},
		sypl.MessageToOutput{Content: "redirected\n", Level: level.Info, OutputName: "C"},
	)

	if bufC.String() != "" {
		t.Fatalf("output C got %q, expected options OutputsNames to redirect", bufC.String())
	}

	if bufD.String() != "redirected\n" {
		t.Fatalf("output D got %q, expected %q", bufD.String(), "redirected\n")
	}

	// Unknown output name: dropped silently.
	bufE, oE := output.SafeBuffer(level.Trace)
	oE.SetName("E")

	le := sypl.New("optdrop", oE)

	le.PrintMessagesToOutputsWithOptions(
		&options.Options{},
		sypl.MessageToOutput{Content: "dropped\n", Level: level.Info, OutputName: "Ghost"},
	)

	if bufE.String() != "" {
		t.Fatalf("message to unknown output was printed: %q", bufE.String())
	}
}

//////
// PrintNewLine / PrintlnWithOptions / PrintMessage.
//////

// PrintNewLine must always print, even against an output @ none.
func TestSypl_PrintNewLine(t *testing.T) {
	buf, o := output.SafeBuffer(level.None)

	l := sypl.New("newline", o)

	l.PrintNewLine()

	if buf.String() != "\n" {
		t.Fatalf("PrintNewLine printed %q, expected %q", buf.String(), "\n")
	}
}

func TestSypl_PrintlnWithOptions(t *testing.T) {
	buf, o := output.SafeBuffer(level.Trace)

	l := sypl.New("printlnwithoptions", o)

	l.PrintlnWithOptions(level.Info, "hello", sypl.WithTags("t"))

	if buf.String() != "hello\n" {
		t.Fatalf("PrintlnWithOptions printed %q, expected %q", buf.String(), "hello\n")
	}
}

// PrintMessage must process every provided message.
func TestSypl_PrintMessageMultiple(t *testing.T) {
	buf, o := output.SafeBuffer(level.Trace)

	l := sypl.New("multi", o)

	l.PrintMessage(
		message.New(level.Info, "m1\n"),
		message.New(level.Warn, "m2\n"),
	)

	got := buf.String()

	if !strings.Contains(got, "m1\n") || !strings.Contains(got, "m2\n") {
		t.Fatalf("PrintMessage dropped a message: %q", got)
	}
}

//////
// SYPL_FILTER.
//////

// SYPL_FILTER: exact, case-insensitive component-name matching. A filter
// entry must NOT match a component it is merely a prefix of.
func TestSypl_FilterEnvVar(t *testing.T) {
	tests := []struct {
		name       string
		filter     string
		component  string
		wantPrints bool
	}{
		{"name in list", "svc,api", "svc", true},
		{"second name in list", "svc,api", "api", true},
		{"name not in list", "svc,api", "nope", false},
		{"exact match only - prefix must not match", "svc", "svc-worker", false},
		{"case-insensitive", "SVC", "svc", true},
		{"list entries are trimmed", "svc , api", "api", true},
		{"empty filter prints", "", "anything", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(shared.FilterEnvVar, tt.filter)

			buf, o := output.SafeBuffer(level.Trace)

			l := sypl.New(tt.component, o)

			l.Infoln("filtered-content")

			printed := strings.Contains(buf.String(), "filtered-content")

			if printed != tt.wantPrints {
				t.Fatalf("filter %q, component %q: printed = %v, expected %v",
					tt.filter, tt.component, printed, tt.wantPrints)
			}
		})
	}
}

//////
// Fatal + nil-receiver paths (subprocess re-exec pattern).
//////

// A nil *Sypl must fail fast (log.Fatalf -> exit 1) with the documented
// error, not nil-panic.
func TestSypl_ProcessNilReceiver(t *testing.T) {
	if os.Getenv("SYPL_TEST_NIL_PROCESS") == "1" {
		var l *sypl.Sypl

		l.Info("boom")

		return
	}

	//nolint:gosec
	cmd := exec.Command(os.Args[0], "-test.run=TestSypl_ProcessNilReceiver$")
	cmd.Env = append(os.Environ(), "SYPL_TEST_NIL_PROCESS=1")

	var stderr strings.Builder
	cmd.Stderr = &stderr

	err := cmd.Run()

	var ee *exec.ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("expected the child to exit non-zero, got: %v", err)
	}

	if ee.ExitCode() != 1 {
		t.Fatalf("child exit code = %d, expected 1", ee.ExitCode())
	}

	if !strings.Contains(stderr.String(), "isn't initialized") {
		t.Fatalf("child stderr missing the initialization error: %q", stderr.String())
	}
}

// Every Fatal variant must print the message and exit with os.Exit(1).
func TestSypl_FatalVariants(t *testing.T) {
	variant := os.Getenv("SYPL_TEST_FATAL_VARIANT")
	if variant != "" {
		l := sypl.New("fatal", output.New("stdout", level.Trace, os.Stdout))

		switch variant {
		case "fatal":
			l.Fatal("fatal-plain")
		case "fatalf":
			l.Fatalf("fatal-%d", 42)
		case "fatallnf":
			l.Fatallnf("fatal-%d", 42)
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
		{"fatallnf", "fatal-42\n"},
		{"fatalln", "fatal-ln\n"},
	}

	for _, tt := range tests {
		t.Run(tt.variant, func(t *testing.T) {
			//nolint:gosec
			cmd := exec.Command(os.Args[0], "-test.run=TestSypl_FatalVariants$")
			cmd.Env = append(os.Environ(), "SYPL_TEST_FATAL_VARIANT="+tt.variant)

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

//////
// Breakpoint.
//////

// Breakpoint must print the banner (PID + optional data), block until enter,
// then trace "Resuming".
func TestSypl_Breakpoint(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	origStdin := os.Stdin
	os.Stdin = r

	t.Cleanup(func() {
		os.Stdin = origStdin

		r.Close()
		w.Close()
	})

	buf, o := output.SafeBuffer(level.Trace)

	l := sypl.New("bp", o)

	// With data.
	if _, err := w.WriteString("\n"); err != nil {
		t.Fatal(err)
	}

	l.Breakpoint("bp-name", "data-1")

	got := buf.String()

	if !strings.Contains(got, fmt.Sprintf("Breakpoint: bp-name. PID: %d", os.Getpid())) {
		t.Fatalf("breakpoint banner missing name/PID: %q", got)
	}

	if !strings.Contains(got, "Data: data-1") {
		t.Fatalf("breakpoint banner missing data: %q", got)
	}

	if !strings.Contains(got, "Press enter to continue...") {
		t.Fatalf("breakpoint banner missing prompt: %q", got)
	}

	if !strings.Contains(got, "Resuming") {
		t.Fatalf("breakpoint did not resume: %q", got)
	}

	// Without data: no "Data:" section.
	buf.Reset()

	if _, err := w.WriteString("\n"); err != nil {
		t.Fatal(err)
	}

	l.Breakpoint("solo")

	got = buf.String()

	if strings.Contains(got, "Data:") {
		t.Fatalf("breakpoint without data printed a Data section: %q", got)
	}

	if !strings.Contains(got, "Resuming") {
		t.Fatalf("breakpoint without data did not resume: %q", got)
	}
}

// Breakpoint's read-error path: a closed stdin must log the failure and NOT
// print "Resuming".
func TestSypl_BreakpointReadError(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	origStdin := os.Stdin
	os.Stdin = r

	t.Cleanup(func() {
		os.Stdin = origStdin

		w.Close()
	})

	// Close the read end early: ReadString must fail.
	r.Close()

	buf, o := output.SafeBuffer(level.Trace)

	l := sypl.New("bp-err", o)

	l.Breakpoint("broken")

	got := buf.String()

	if !strings.Contains(got, "Failed to read input") {
		t.Fatalf("read-error path not logged: %q", got)
	}

	if strings.Contains(got, "Resuming") {
		t.Fatalf("read-error path must not resume: %q", got)
	}
}

//////
// NewDefault.
//////

// NewDefault must write non-error levels to the stdout-side output only, and
// error levels to the stderr-side output only (MuteBasedOnLevel wiring), with
// the given processors applied to both.
func TestSypl_NewDefault(t *testing.T) {
	l := sypl.NewDefault("nd", level.Info, processor.Prefixer("P-"))

	consoleOut := l.GetOutput("Console")
	stderrOut := l.GetOutput("StdErr")

	if consoleOut == nil || stderrOut == nil {
		t.Fatalf("NewDefault outputs = %v, expected Console and StdErr", l.GetOutputsNames())
	}

	// Redirect both outputs to buffers.
	var bufOut, bufErr safebuffer.Buffer

	consoleOut.SetWriter(&bufOut)
	consoleOut.GetBuiltinLogger().SetOutput(&bufOut)

	stderrOut.SetWriter(&bufErr)
	stderrOut.GetBuiltinLogger().SetOutput(&bufErr)

	l.Infoln("info-msg")
	l.Errorln("err-msg")

	stdout := bufOut.String()
	stderr := bufErr.String()

	// Info: stdout only.
	if !strings.Contains(stdout, "P-info-msg") {
		t.Fatalf("stdout-side output missing processed info message: %q", stdout)
	}

	if strings.Contains(stderr, "info-msg") {
		t.Fatalf("stderr-side output leaked an info message: %q", stderr)
	}

	// Error: stderr only (console muted via MuteBasedOnLevel).
	if !strings.Contains(stderr, "P-err-msg") {
		t.Fatalf("stderr-side output missing processed error message: %q", stderr)
	}

	if strings.Contains(stdout, "err-msg") {
		t.Fatalf("stdout-side output leaked an error message (MuteBasedOnLevel broken): %q", stdout)
	}

	// Text formatter is wired on both.
	if !strings.Contains(stdout, "component=nd") || !strings.Contains(stdout, "level=info") {
		t.Fatalf("stdout-side output not using the Text formatter: %q", stdout)
	}

	if !strings.Contains(stderr, "level=error") {
		t.Fatalf("stderr-side output not using the Text formatter: %q", stderr)
	}

	// Default io.Writer level is none.
	if l.GetDefaultIoWriterLevel() != level.None {
		t.Fatalf("NewDefault io.Writer level = %s, expected none", l.GetDefaultIoWriterLevel())
	}
}
