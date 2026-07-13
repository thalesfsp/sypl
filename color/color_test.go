// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package color

import (
	"fmt"
	"strings"
	"testing"

	"github.com/acarl005/stripansi"
	fatihcolor "github.com/fatih/color"
	"github.com/thalesfsp/sypl/v2/shared"
)

// All built-in colors must satisfy the exported Color specification.
var (
	_ Color = Red
	_ Color = BoldRed
	_ Color = Green
	_ Color = BoldGreen
	_ Color = Yellow
	_ Color = BoldYellow
)

// enableColor forces color output on (tests don't run on a TTY, so
// fatih/color would otherwise disable itself), restoring the previous
// setting when the test finishes.
func enableColor(t *testing.T) {
	t.Helper()

	orig := fatihcolor.NoColor

	fatihcolor.NoColor = false

	t.Cleanup(func() { fatihcolor.NoColor = orig })
}

func colorTable() []struct {
	name     string
	color    Color
	ansiCode string
} {
	return []struct {
		name     string
		color    Color
		ansiCode string
	}{
		{name: "Red", color: Red, ansiCode: "31"},
		{name: "BoldRed", color: BoldRed, ansiCode: "31;1"},
		{name: "Green", color: Green, ansiCode: "32"},
		{name: "BoldGreen", color: BoldGreen, ansiCode: "32;1"},
		{name: "Yellow", color: Yellow, ansiCode: "33"},
		{name: "BoldYellow", color: BoldYellow, ansiCode: "33;1"},
	}
}

func TestColors_WrapWithANSICodes(t *testing.T) {
	enableColor(t)

	for _, tt := range colorTable() {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.color(shared.DefaultContentOutput)

			// Must open with the exact color code...
			wantPrefix := fmt.Sprintf("\x1b[%sm", tt.ansiCode)

			if !strings.HasPrefix(got, wantPrefix) {
				t.Errorf("%s(%q) = %q, want prefix %q", tt.name, shared.DefaultContentOutput, got, wantPrefix)
			}

			// ...and close with a reset sequence ("\x1b[0m", or an
			// attribute-specific reset such as "\x1b[0;22m" for bold).
			if !strings.Contains(got, shared.DefaultContentOutput+"\x1b[0") || !strings.HasSuffix(got, "m") {
				t.Errorf("%s(%q) = %q, want content followed by a reset sequence", tt.name, shared.DefaultContentOutput, got)
			}

			// Content must pass through untouched once codes are stripped.
			if stripped := stripansi.Strip(got); stripped != shared.DefaultContentOutput {
				t.Errorf("stripansi.Strip(%s(...)) = %q, want %q", tt.name, stripped, shared.DefaultContentOutput)
			}
		})
	}
}

func TestColors_EmptyString(t *testing.T) {
	enableColor(t)

	for _, tt := range colorTable() {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.color("")

			if stripped := stripansi.Strip(got); stripped != "" {
				t.Errorf("stripansi.Strip(%s(\"\")) = %q, want empty string", tt.name, stripped)
			}
		})
	}
}

func TestColors_MultipleArgs(t *testing.T) {
	enableColor(t)

	for _, tt := range colorTable() {
		t.Run(tt.name, func(t *testing.T) {
			// fmt.Sprint semantics: adjacent string operands aren't
			// space-separated.
			got := tt.color("a", "b")

			if stripped := stripansi.Strip(got); stripped != "ab" {
				t.Errorf("stripansi.Strip(%s(\"a\", \"b\")) = %q, want %q", tt.name, stripped, "ab")
			}
		})
	}
}

func TestColors_NoColorPassthrough(t *testing.T) {
	orig := fatihcolor.NoColor

	fatihcolor.NoColor = true

	t.Cleanup(func() { fatihcolor.NoColor = orig })

	for _, tt := range colorTable() {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.color(shared.DefaultContentOutput)

			if got != shared.DefaultContentOutput {
				t.Errorf("%s(...) with NoColor = %q, want %q (no ANSI codes)", tt.name, got, shared.DefaultContentOutput)
			}

			if strings.Contains(got, "\x1b[") {
				t.Errorf("%s(...) with NoColor contains ANSI escape: %q", tt.name, got)
			}
		})
	}
}
