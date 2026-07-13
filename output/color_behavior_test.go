// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package output

import (
	"strings"
	"testing"

	fatihcolor "github.com/fatih/color"
	"github.com/thalesfsp/sypl/v2/color"
	"github.com/thalesfsp/sypl/v2/level"
	"github.com/thalesfsp/sypl/v2/message"
	"github.com/thalesfsp/sypl/v2/processor"
	"github.com/thalesfsp/sypl/v2/safebuffer"
)

// ansiEscapePrefix starts every ANSI SGR color sequence.
const ansiEscapePrefix = "\x1b["

// Verifies - and documents - Sypl's NO_COLOR, and non-tty behavior.
//
// FINDINGS:
//
// Sypl does NOT bypass fatih/color's detection: the `color` package builds
// its palette with `color.New(...).SprintFunc()`, and the colorize
// processors apply those functions - no raw ANSI is ever injected. Every
// print re-checks `fatihcolor.NoColor` (via `isNoColorSet`), so whatever
// the detection decided applies to each write.
//
// `fatihcolor.NoColor` itself is computed ONCE - at package init - as:
// NO_COLOR set (any non-empty value), or TERM=dumb, or stdout NOT a
// terminal. Consequences:
//   - A process started with NO_COLOR=1 writes uncolored text - the
//     standard https://no-color.org contract - with no Sypl involvement.
//   - Setting NO_COLOR mid-process (e.g. t.Setenv, os.Setenv) does NOT
//     retrigger detection: it's an init-time snapshot. In tests, output is
//     uncolored anyway - `go test` pipes stdout, so the non-tty branch of
//     the SAME detection already disabled color. Either road - NO_COLOR,
//     or non-tty - leads to the uncolored output asserted here.
//   - A per-color override (`EnableColor()`) beats the global detection -
//     used below as the negative control proving the assertion mechanism
//     actually detects ANSI sequences.
func TestConsole_NoColorAndNonTTYProduceUncoloredOutput(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	// Belt, and braces: in a `go test` run stdout is a pipe - the non-tty
	// branch of fatih/color's init-time detection must have fired.
	if !fatihcolor.NoColor {
		t.Fatal("Expected fatih/color to have disabled color: stdout isn't a terminal")
	}

	// A Console output with a color processor attached - writes are
	// captured in a buffer; the real stdout stays clean.
	o := Console(level.Trace, processor.ColorizeBasedOnLevel(
		map[level.Level]color.Color{level.Info: color.Red},
	))

	var buf safebuffer.Buffer

	o.GetBuiltinLogger().SetOutput(&buf)

	if err := o.Write(message.New(level.Info, "colored candidate\n")); err != nil {
		t.Fatalf("Write() error = %v, want nil", err)
	}

	if !strings.Contains(buf.String(), "colored candidate") {
		t.Fatalf("Buffer = %q, want the message text", buf.String())
	}

	if strings.Contains(buf.String(), ansiEscapePrefix) {
		t.Errorf("Buffer = %q contains ANSI escapes - NO_COLOR/non-tty was bypassed", buf.String())
	}
}

// Negative control: a color FORCED via EnableColor() overrides the global
// detection - ANSI escapes MUST appear, proving the assertion above fails
// for the right reason (fatih/color's gating), not because Sypl strips
// color, or the detector can't see ANSI.
func TestConsole_ForcedColorStillColorsOutput(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	forcedRed := fatihcolor.New(fatihcolor.FgRed)

	forcedRed.EnableColor()

	o := Console(level.Trace, processor.ColorizeBasedOnLevel(
		map[level.Level]color.Color{level.Info: forcedRed.SprintFunc()},
	))

	var buf safebuffer.Buffer

	o.GetBuiltinLogger().SetOutput(&buf)

	if err := o.Write(message.New(level.Info, "forced\n")); err != nil {
		t.Fatalf("Write() error = %v, want nil", err)
	}

	if !strings.Contains(buf.String(), ansiEscapePrefix) {
		t.Errorf("Buffer = %q, want ANSI escapes - the forced color must colorize", buf.String())
	}
}
