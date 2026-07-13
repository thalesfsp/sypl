// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package output

import (
	"bytes"
	"errors"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/thalesfsp/sypl/v2/internal/sypltest"
	"github.com/thalesfsp/sypl/v2/level"
	"github.com/thalesfsp/sypl/v2/message"
	"github.com/thalesfsp/sypl/v2/processor"
	"github.com/thalesfsp/sypl/v2/shared"
)

// getProcessorByName returns the processor registered under `name` - nil
// if absent. The v2 replacement for the removed `IOutput.GetProcessor`.
func getProcessorByName(o IOutput, name string) processor.IProcessor {
	for _, p := range o.GetProcessors() {
		if strings.EqualFold(p.GetName(), name) {
			return p
		}
	}

	return nil
}

//////
// Console, and StdErr.
//////

func TestConsole(t *testing.T) {
	o := Console(level.Debug, processor.Prefixer(sypltest.DefaultPrefixValue))

	if o.GetName() != "Console" {
		t.Errorf("GetName() = %q, want %q", o.GetName(), "Console")
	}

	if o.GetMaxLevel() != level.Debug {
		t.Errorf("GetMaxLevel() = %v, want %v", o.GetMaxLevel(), level.Debug)
	}

	if o.GetWriter() != os.Stdout {
		t.Error("Console should write to stdout")
	}

	if getProcessorByName(o, "Prefixer") == nil {
		t.Error("Console should carry the given processors")
	}
}

func TestStdErr(t *testing.T) {
	o := StdErr()

	if o.GetName() != "StdErr" {
		t.Errorf("GetName() = %q, want %q", o.GetName(), "StdErr")
	}

	if o.GetMaxLevel() != level.Error {
		t.Errorf("GetMaxLevel() = %v, want %v", o.GetMaxLevel(), level.Error)
	}

	if o.GetWriter() != os.Stderr {
		t.Error("StdErr should write to stderr")
	}

	// StdErr auto-appends a level restrictor.
	if getProcessorByName(o, "PrintOnlyAtLevel") == nil {
		t.Fatal("StdErr should have the PrintOnlyAtLevel processor")
	}

	// Behavior: only Fatal, and Error pass. Redirect the writes to a
	// buffer - the real stderr must stay clean.
	var buf bytes.Buffer

	o.GetBuiltinLogger().SetOutput(&buf)

	if err := o.Write(message.New(level.Error, "error message")); err != nil {
		t.Fatalf("Write() error = %v, want nil", err)
	}

	if !strings.Contains(buf.String(), "error message") {
		t.Errorf("Error-level message should be written, buffer: %q", buf.String())
	}

	buf.Reset()

	if err := o.Write(message.New(level.Info, "info message")); err != nil {
		t.Fatalf("Write() error = %v, want nil", err)
	}

	if buf.Len() != 0 {
		t.Errorf("Info-level message should be muted, buffer: %q", buf.String())
	}
}

//////
// File-based outputs.
//////

func TestFileBased(t *testing.T) {
	path := filepath.Join(t.TempDir(), "filebased.log")

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, shared.DefaultFileMode)
	if err != nil {
		t.Fatalf("Failed to create the test file: %v", err)
	}

	defer f.Close()

	o := FileBased("MyFile", level.Trace, f, processor.Prefixer(sypltest.DefaultPrefixValue))

	if o.GetName() != "MyFile" {
		t.Errorf("GetName() = %q, want %q", o.GetName(), "MyFile")
	}

	if err := o.Write(message.New(level.Info, sypltest.DefaultContentOutput)); err != nil {
		t.Fatalf("Write() error = %v, want nil", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read the log file: %v", err)
	}

	want := sypltest.DefaultPrefixValue + sypltest.DefaultContentOutput

	if string(content) != want {
		t.Errorf("File content = %q, want %q", string(content), want)
	}
}

func TestFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file.log")

	o := File("File", path, level.Trace)

	if o.GetName() != "File" {
		t.Errorf("GetName() = %q, want %q", o.GetName(), "File")
	}

	if err := o.Write(message.New(level.Info, sypltest.DefaultContentOutput)); err != nil {
		t.Fatalf("Write() error = %v, want nil", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read the log file: %v", err)
	}

	if string(content) != sypltest.DefaultContentOutput {
		t.Errorf("File content = %q, want %q", string(content), sypltest.DefaultContentOutput)
	}
}

func TestFile_CreatesMissingDirectories(t *testing.T) {
	// The parent directories don't exist - File must create them, and
	// retry.
	path := filepath.Join(t.TempDir(), "a", "b", "c.log")

	o := File("File", path, level.Trace)

	if err := o.Write(message.New(level.Info, sypltest.DefaultContentOutput)); err != nil {
		t.Fatalf("Write() error = %v, want nil", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read the log file: %v", err)
	}

	if string(content) != sypltest.DefaultContentOutput {
		t.Errorf("File content = %q, want %q", string(content), sypltest.DefaultContentOutput)
	}
}

func TestFile_DashWritesToStdout(t *testing.T) {
	o := File("DashOutput", "-", level.Debug)

	if o.GetName() != "DashOutput" {
		t.Errorf("GetName() = %q, want %q", o.GetName(), "DashOutput")
	}

	if o.GetWriter() != os.Stdout {
		t.Error(`File with the "-" path should write to stdout`)
	}

	if o.GetMaxLevel() != level.Debug {
		t.Errorf("GetMaxLevel() = %v, want %v", o.GetMaxLevel(), level.Debug)
	}
}

func TestFile_EmptyPathCreatesTempFile(t *testing.T) {
	// An empty path makes File create a UUID-named file in the OS temp
	// dir, logging the chosen path - capture the log to find, verify,
	// and clean it up.
	var logBuf bytes.Buffer

	log.SetOutput(&logBuf)

	defer log.SetOutput(os.Stderr)

	o := File("File", "", level.Trace)

	re := regexp.MustCompile(`Created/opened "([^"]+)"`)

	matches := re.FindStringSubmatch(logBuf.String())

	if len(matches) != 2 {
		t.Fatalf("Expected the created path to be logged, got %q", logBuf.String())
	}

	path := matches[1]

	t.Cleanup(func() { os.Remove(path) })

	if err := o.Write(message.New(level.Info, sypltest.DefaultContentOutput)); err != nil {
		t.Fatalf("Write() error = %v, want nil", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read the log file: %v", err)
	}

	if string(content) != sypltest.DefaultContentOutput {
		t.Errorf("File content = %q, want %q", string(content), sypltest.DefaultContentOutput)
	}
}

// File calls log.Fatalf on unrecoverable filesystem failures, so those
// paths are asserted by re-running this test binary as a subprocess, and
// checking it dies with exit code 1, and the expected message.
func TestFile_FatalPaths(t *testing.T) {
	// Subprocess mode: trigger the fatal path under test, and nothing
	// else. If `File` doesn't exit, exit with a sentinel code so the
	// parent fails the assertion.
	if mode, ok := os.LookupEnv("SYPL_TEST_FILE_FATAL_MODE"); ok {
		switch mode {
		case "isdir":
			// Opening a directory for writing fails with EISDIR - which
			// isn't "not exist", so there's no retry.
			dir, err := os.MkdirTemp("", "sypl-file-fatal")
			if err != nil {
				os.Exit(43)
			}

			// No cleanup: File is expected to log.Fatalf, which skips
			// defers anyway; the OS reclaims the temp dir.
			File("File", dir, level.Trace)
		case "mkdirfail":
			// The parent dir is read-only - the MkdirAll retry fails.
			parent, err := os.MkdirTemp("", "sypl-file-fatal")
			if err != nil {
				os.Exit(43)
			}

			if err := os.Chmod(parent, 0o555); err != nil {
				os.Exit(43)
			}

			File("File", filepath.Join(parent, "sub", "file.log"), level.Trace)
		}

		os.Exit(42)
	}

	if os.Geteuid() == 0 {
		t.Skip("Running as root - permission-based failures can't be simulated")
	}

	tests := []struct {
		name    string
		mode    string
		wantMsg string
	}{
		{
			name:    "Should exit - path is a directory",
			mode:    "isdir",
			wantMsg: "Failed to create/open",
		},
		{
			name:    "Should exit - directory can't be created",
			mode:    "mkdirfail",
			wantMsg: "Failed to create dir",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			//nolint:gosec // Re-running the test binary itself.
			cmd := exec.Command(os.Args[0], "-test.run=TestFile_FatalPaths$")

			cmd.Env = append(os.Environ(), "SYPL_TEST_FILE_FATAL_MODE="+tt.mode)

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
// SafeBuffer.
//////

func TestSafeBuffer(t *testing.T) {
	buf, o := SafeBuffer(level.Trace, processor.Prefixer(sypltest.DefaultPrefixValue))

	if o.GetName() != "Buffer" {
		t.Errorf("GetName() = %q, want %q", o.GetName(), "Buffer")
	}

	if o.GetWriter() != buf {
		t.Error("SafeBuffer should expose the same buffer it writes to")
	}

	if err := o.Write(message.New(level.Info, sypltest.DefaultContentOutput)); err != nil {
		t.Fatalf("Write() error = %v, want nil", err)
	}

	want := sypltest.DefaultPrefixValue + sypltest.DefaultContentOutput

	if buf.String() != want {
		t.Errorf("Buffer = %q, want %q", buf.String(), want)
	}
}
