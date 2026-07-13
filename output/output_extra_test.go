// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package output

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"syscall"
	"testing"

	"github.com/thalesfsp/sypl/v2/debug"
	"github.com/thalesfsp/sypl/v2/flag"
	"github.com/thalesfsp/sypl/v2/formatter"
	"github.com/thalesfsp/sypl/v2/internal/builtin"
	"github.com/thalesfsp/sypl/v2/level"
	"github.com/thalesfsp/sypl/v2/message"
	"github.com/thalesfsp/sypl/v2/processor"
	"github.com/thalesfsp/sypl/v2/shared"
	"github.com/thalesfsp/sypl/v2/status"
)

//////
// Test helpers.
//////

// failingWriter always fails with the configured error.
type failingWriter struct {
	err error
}

func (f *failingWriter) Write(p []byte) (int, error) {
	return 0, f.err
}

// newBufferedOutput creates an output writing to an in-memory buffer.
func newBufferedOutput(maxLevel level.Level, processors ...processor.IProcessor) (*bytes.Buffer, IOutput) {
	var buf bytes.Buffer

	return &buf, New("TestOutput", maxLevel, &buf, processors...)
}

//////
// Write - flag, and level dispatch.
//////

func TestOutput_Write_Dispatch(t *testing.T) {
	tests := []struct {
		name      string
		maxLevel  level.Level
		msgLevel  level.Level
		flag      flag.Flag
		wantWrite bool
	}{
		{
			name:      "Should print - level within maxLevel",
			maxLevel:  level.Info,
			msgLevel:  level.Info,
			flag:      flag.None,
			wantWrite: true,
		},
		{
			name:      "Should not print - level above maxLevel",
			maxLevel:  level.Info,
			msgLevel:  level.Debug,
			flag:      flag.None,
			wantWrite: false,
		},
		{
			name:      "Should not print - level.None message",
			maxLevel:  level.Trace,
			msgLevel:  level.None,
			flag:      flag.None,
			wantWrite: false,
		},
		{
			name:      "Should print - Force bypasses maxLevel",
			maxLevel:  level.Info,
			msgLevel:  level.Trace,
			flag:      flag.Force,
			wantWrite: true,
		},
		{
			name:      "Should print - SkipAndForce bypasses maxLevel",
			maxLevel:  level.Info,
			msgLevel:  level.Trace,
			flag:      flag.SkipAndForce,
			wantWrite: true,
		},
		{
			name:      "Should not print - Mute",
			maxLevel:  level.Trace,
			msgLevel:  level.Info,
			flag:      flag.Mute,
			wantWrite: false,
		},
		{
			name:      "Should not print - SkipAndMute",
			maxLevel:  level.Trace,
			msgLevel:  level.Info,
			flag:      flag.SkipAndMute,
			wantWrite: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf, o := newBufferedOutput(tt.maxLevel)

			m := message.New(tt.msgLevel, shared.DefaultContentOutput)

			m.SetFlag(tt.flag)

			if err := o.Write(m); err != nil {
				t.Fatalf("Write() error = %v, want nil", err)
			}

			if tt.wantWrite && !strings.Contains(buf.String(), shared.DefaultContentOutput) {
				t.Errorf("Expected %q to be written, buffer: %q", shared.DefaultContentOutput, buf.String())
			}

			if !tt.wantWrite && buf.Len() != 0 {
				t.Errorf("Expected nothing to be written, buffer: %q", buf.String())
			}
		})
	}
}

func TestOutput_Write_SkipFlagsBypassProcessing(t *testing.T) {
	tests := []struct {
		name string
		flag flag.Flag
	}{
		{name: "Should not process, or format - Skip", flag: flag.Skip},
		{name: "Should not process, or format - SkipAndForce", flag: flag.SkipAndForce},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf, o := newBufferedOutput(level.Trace, processor.Prefixer(shared.DefaultPrefixValue))

			o.SetFormatter(formatter.JSON())

			m := message.New(level.Info, shared.DefaultContentOutput)

			m.SetFlag(tt.flag)

			if err := o.Write(m); err != nil {
				t.Fatalf("Write() error = %v, want nil", err)
			}

			// Raw content only: no prefix (processing skipped), and no JSON
			// (formatting skipped).
			if buf.String() != shared.DefaultContentOutput {
				t.Errorf("Expected raw content %q, got %q", shared.DefaultContentOutput, buf.String())
			}
		})
	}
}

func TestOutput_Write_ProcessorsAreApplied(t *testing.T) {
	buf, o := newBufferedOutput(level.Trace,
		processor.Prefixer(shared.DefaultPrefixValue),
		processor.Suffixer(" - suffixed"),
	)

	m := message.New(level.Info, shared.DefaultContentOutput)

	if err := o.Write(m); err != nil {
		t.Fatalf("Write() error = %v, want nil", err)
	}

	want := shared.DefaultPrefixValue + shared.DefaultContentOutput + " - suffixed"

	if buf.String() != want {
		t.Errorf("Write() output = %q, want %q", buf.String(), want)
	}
}

func TestOutput_Write_MessageSelectedProcessors(t *testing.T) {
	buf, o := newBufferedOutput(level.Trace,
		processor.Prefixer(shared.DefaultPrefixValue),
		processor.Suffixer(" - suffixed"),
	)

	m := message.New(level.Info, shared.DefaultContentOutput)

	// The message narrows processing down to a single processor.
	m.SetProcessorsNames([]string{"Suffixer"})

	if err := o.Write(m); err != nil {
		t.Fatalf("Write() error = %v, want nil", err)
	}

	want := shared.DefaultContentOutput + " - suffixed"

	if buf.String() != want {
		t.Errorf("Write() output = %q, want %q", buf.String(), want)
	}
}

func TestOutput_Write_ProcessorErrorContinues(t *testing.T) {
	// A failing processor is logged, and the pipeline continues - the
	// message must still be processed by later processors, and printed.
	buf, o := newBufferedOutput(level.Trace,
		processor.ErrorSimulator("boom"),
		processor.Suffixer(" - suffixed"),
	)

	m := message.New(level.Info, shared.DefaultContentOutput)

	if err := o.Write(m); err != nil {
		t.Fatalf("Write() error = %v, want nil", err)
	}

	want := shared.DefaultContentOutput + " - suffixed"

	if buf.String() != want {
		t.Errorf("Write() output = %q, want %q", buf.String(), want)
	}
}

//////
// Write - formatter.
//////

func TestOutput_Write_WithFormatter(t *testing.T) {
	buf, o := newBufferedOutput(level.Trace)

	o.SetFormatter(formatter.JSON())

	m := message.New(level.Info, shared.DefaultContentOutput)

	if err := o.Write(m); err != nil {
		t.Fatalf("Write() error = %v, want nil", err)
	}

	parsed := map[string]interface{}{}

	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("Formatted output isn't valid JSON: %v.\nOutput: %s", err, buf.String())
	}

	if parsed["message"] != shared.DefaultContentOutput {
		t.Errorf(`message = %v, want %q`, parsed["message"], shared.DefaultContentOutput)
	}
}

func TestOutput_Write_FormatterErrorContinues(t *testing.T) {
	buf, o := newBufferedOutput(level.Trace)

	// A failing formatter is logged, and the write still happens - with
	// the unformatted content.
	o.SetFormatter(processor.New("BadFormatter", func(m message.IMessage) error {
		return errors.New("formatter boom")
	}))

	m := message.New(level.Info, shared.DefaultContentOutput)

	if err := o.Write(m); err != nil {
		t.Fatalf("Write() error = %v, want nil", err)
	}

	if buf.String() != shared.DefaultContentOutput {
		t.Errorf("Write() output = %q, want %q", buf.String(), shared.DefaultContentOutput)
	}
}

//////
// Write - SYPL_LEVEL env var override.
//////

func TestOutput_Write_EnvVarLevelOverride(t *testing.T) {
	tests := []struct {
		name      string
		envValue  string
		wantWrite bool
	}{
		{
			name: "Should print - env var raises maxLevel",

			// maxLevel is Info, message is Debug - only printable because
			// the env var overrides to debug.
			envValue:  "debug",
			wantWrite: true,
		},
		{
			name: "Should not print - env var doesn't match any level",

			// The env var is set, but useless - the debug capability is
			// consulted, matches nothing, and maxLevel stays Info.
			envValue:  "not-a-level",
			wantWrite: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(shared.LevelEnvVar, tt.envValue)

			buf, o := newBufferedOutput(level.Info)

			m := message.New(level.Debug, shared.DefaultContentOutput)

			// In the real pipeline (sypl.go), the debug matchers are set
			// on the message before it reaches the output.
			m.SetDebugEnvVarRegexes(debug.New("testComponent", "TestOutput"))

			if err := o.Write(m); err != nil {
				t.Fatalf("Write() error = %v, want nil", err)
			}

			if tt.wantWrite && !strings.Contains(buf.String(), shared.DefaultContentOutput) {
				t.Errorf("Expected message to be written, buffer: %q", buf.String())
			}

			if !tt.wantWrite && buf.Len() != 0 {
				t.Errorf("Expected nothing to be written, buffer: %q", buf.String())
			}
		})
	}
}

//////
// Write - writer error paths.
//////

func TestOutput_Write_WriterErrors(t *testing.T) {
	tests := []struct {
		name      string
		writerErr error
		wantErr   bool
	}{
		{
			name:      "Should swallow - broken pipe (EPIPE)",
			writerErr: syscall.EPIPE,
			wantErr:   false,
		},
		{
			name:      "Should swallow - wrapped EPIPE",
			writerErr: fmt.Errorf("wrapped: %w", syscall.EPIPE),
			wantErr:   false,
		},
		{
			name:      "Should swallow - closed writer (os.ErrClosed)",
			writerErr: os.ErrClosed,
			wantErr:   false,
		},
		{
			name:      "Should swallow - wrapped os.ErrClosed",
			writerErr: fmt.Errorf("wrapped: %w", os.ErrClosed),
			wantErr:   false,
		},
		{
			name:      "Should fail - arbitrary write error",
			writerErr: errors.New("disk full"),
			wantErr:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := New("TestOutput", level.Trace, &failingWriter{err: tt.writerErr})

			m := message.New(level.Info, shared.DefaultContentOutput)

			err := o.Write(m)

			if !tt.wantErr {
				if err != nil {
					t.Fatalf("Write() error = %v, want nil", err)
				}

				return
			}

			if err == nil {
				t.Fatal("Write() should fail")
			}

			// The error is wrapped with the output name.
			if !strings.Contains(err.Error(), `output: "TestOutput"`) {
				t.Errorf("Error %q should mention the output name", err.Error())
			}

			if !strings.Contains(err.Error(), "disk full") {
				t.Errorf("Error %q should wrap the writer error", err.Error())
			}
		})
	}
}

func TestOutput_Write_WriterErrorWithForceFlag(t *testing.T) {
	// The Force branch has its own write call - its error path must
	// behave the same.
	o := New("TestOutput", level.Info, &failingWriter{err: errors.New("disk full")})

	m := message.New(level.Trace, shared.DefaultContentOutput)

	m.SetFlag(flag.Force)

	err := o.Write(m)
	if err == nil {
		t.Fatal("Write() should fail")
	}

	if !strings.Contains(err.Error(), "disk full") {
		t.Errorf("Error %q should wrap the writer error", err.Error())
	}
}

//////
// Getters, and setters.
//////

func TestOutput_GettersAndSetters(t *testing.T) {
	var buf bytes.Buffer

	o := New("TestOutput", level.Info, &buf, processor.Prefixer(shared.DefaultPrefixValue))

	// Name, and String.
	if o.GetName() != "TestOutput" {
		t.Errorf("GetName() = %q, want %q", o.GetName(), "TestOutput")
	}

	if fmt.Sprint(o) != "TestOutput" {
		t.Errorf("String() = %q, want %q", fmt.Sprint(o), "TestOutput")
	}

	o.SetName("Renamed")

	if o.GetName() != "Renamed" {
		t.Errorf("GetName() after SetName = %q, want %q", o.GetName(), "Renamed")
	}

	// Status.
	if o.GetStatus() != status.Enabled {
		t.Errorf("GetStatus() = %v, want %v", o.GetStatus(), status.Enabled)
	}

	o.SetStatus(status.Disabled)

	if o.GetStatus() != status.Disabled {
		t.Errorf("GetStatus() after SetStatus = %v, want %v", o.GetStatus(), status.Disabled)
	}

	// Max level.
	if o.GetMaxLevel() != level.Info {
		t.Errorf("GetMaxLevel() = %v, want %v", o.GetMaxLevel(), level.Info)
	}

	o.SetMaxLevel(level.Trace)

	if o.GetMaxLevel() != level.Trace {
		t.Errorf("GetMaxLevel() after SetMaxLevel = %v, want %v", o.GetMaxLevel(), level.Trace)
	}

	// Writer.
	if o.GetWriter() != &buf {
		t.Error("GetWriter() should return the constructor writer")
	}

	var otherBuf bytes.Buffer

	o.SetWriter(&otherBuf)

	if o.GetWriter() != &otherBuf {
		t.Error("GetWriter() should return the writer set via SetWriter")
	}

	// Formatter.
	if o.GetFormatter() != nil {
		t.Error("GetFormatter() should be nil by default")
	}

	o.SetFormatter(formatter.JSON())

	if o.GetFormatter() == nil || o.GetFormatter().GetName() != "JSON" {
		t.Errorf("GetFormatter() = %v, want the JSON formatter", o.GetFormatter())
	}
}

func TestOutput_ProcessorManagement(t *testing.T) {
	var buf bytes.Buffer

	o := New("TestOutput", level.Info, &buf, processor.Prefixer(shared.DefaultPrefixValue))

	// GetProcessor - case-insensitive hit, and miss.
	if o.GetProcessor("prefixer") == nil {
		t.Error(`GetProcessor("prefixer") should find "Prefixer" (case-insensitive)`)
	}

	if o.GetProcessor("nonexistent") != nil {
		t.Error(`GetProcessor("nonexistent") should be nil`)
	}

	// GetProcessorsNames.
	names := o.GetProcessorsNames()

	if len(names) != 1 || names[0] != "Prefixer" {
		t.Errorf("GetProcessorsNames() = %v, want [Prefixer]", names)
	}

	// AddProcessors appends.
	o.AddProcessors(processor.Suffixer(" - suffix"))

	if got := len(o.GetProcessors()); got != 2 {
		t.Errorf("GetProcessors() after AddProcessors = %d processors, want 2", got)
	}

	// SetProcessors replaces by name...
	replacement := processor.New("Prefixer", func(m message.IMessage) error {
		m.GetContent().SetProcessed("replaced: " + m.GetContent().GetProcessed())

		return nil
	})

	o.SetProcessors(replacement)

	m := message.New(level.Info, shared.DefaultContentOutput)

	if err := o.GetProcessor("Prefixer").Run(m); err != nil {
		t.Fatalf("Run failed: %s", err)
	}

	if !strings.HasPrefix(m.GetContent().GetProcessed(), "replaced: ") {
		t.Errorf("SetProcessors should have replaced the Prefixer, content: %q",
			m.GetContent().GetProcessed())
	}

	if got := len(o.GetProcessors()); got != 2 {
		t.Errorf("SetProcessors should replace - not append. Got %d processors, want 2", got)
	}

	// ...and ignores names that aren't registered.
	o.SetProcessors(processor.New("Unknown", func(m message.IMessage) error { return nil }))

	if got := len(o.GetProcessors()); got != 2 {
		t.Errorf("SetProcessors with an unknown name should be a no-op. Got %d processors, want 2", got)
	}

	if o.GetProcessor("Unknown") != nil {
		t.Error(`GetProcessor("Unknown") should be nil - SetProcessors must not append`)
	}
}

func TestOutput_SetBuiltinLogger(t *testing.T) {
	buf, o := newBufferedOutput(level.Trace)

	var redirected bytes.Buffer

	bl := builtin.NewBuiltin(&redirected, "", 0)

	if o.SetBuiltinLogger(bl); o.GetBuiltinLogger() != bl {
		t.Fatal("GetBuiltinLogger() should return the logger set via SetBuiltinLogger")
	}

	m := message.New(level.Info, shared.DefaultContentOutput)

	if err := o.Write(m); err != nil {
		t.Fatalf("Write() error = %v, want nil", err)
	}

	// The write must land on the new logger's writer - not the original.
	if redirected.String() != shared.DefaultContentOutput {
		t.Errorf("Redirected buffer = %q, want %q", redirected.String(), shared.DefaultContentOutput)
	}

	if buf.Len() != 0 {
		t.Errorf("Original buffer should be empty, got %q", buf.String())
	}
}
