// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package debug

import (
	"testing"

	"github.com/thalesfsp/sypl/v2/level"
	"github.com/thalesfsp/sypl/v2/shared"
)

// TestEntryBoundaries verifies that each matcher only matches COMPLETE
// comma-delimited entries of the debug env var - never prefixes, or
// substrings of unrelated entries.
func TestEntryBoundaries(t *testing.T) {
	type args struct {
		componentName string
		outputName    string
	}

	tests := []struct {
		name        string
		args        args
		content     string
		wantLevel   level.Level
		wantMatcher Matcher
		wantOK      bool
	}{
		//////
		// Happy path.
		//////
		{
			name:        "Should work - bare level",
			args:        args{componentName: "comp", outputName: "out"},
			content:     "trace",
			wantLevel:   level.Trace,
			wantMatcher: L,
			wantOK:      true,
		},
		{
			name:        "Should work - output:level",
			args:        args{componentName: "comp", outputName: "console"},
			content:     "console:debug",
			wantLevel:   level.Debug,
			wantMatcher: OL,
			wantOK:      true,
		},
		{
			name:        "Should work - component:output:level",
			args:        args{componentName: "comp", outputName: "console"},
			content:     "comp:console:warn",
			wantLevel:   level.Warn,
			wantMatcher: COL,
			wantOK:      true,
		},
		{
			name:        "Should work - multi-entry, COL wins",
			args:        args{componentName: "comp", outputName: "console"},
			content:     "info,comp:console:debug,file:trace",
			wantLevel:   level.Debug,
			wantMatcher: COL,
			wantOK:      true,
		},
		{
			name:        "Should work - multi-entry, OL wins",
			args:        args{componentName: "comp", outputName: "file"},
			content:     "info,other:console:debug,file:trace",
			wantLevel:   level.Trace,
			wantMatcher: OL,
			wantOK:      true,
		},
		{
			name:        "Should work - multi-entry, falls back to bare level",
			args:        args{componentName: "comp", outputName: "out"},
			content:     "info,other:console:debug,file:trace",
			wantLevel:   level.Info,
			wantMatcher: L,
			wantOK:      true,
		},
		{
			name:        "Should work - bare level is case-insensitive",
			args:        args{componentName: "comp", outputName: "out"},
			content:     "TRACE",
			wantLevel:   level.Trace,
			wantMatcher: L,
			wantOK:      true,
		},

		//////
		// Prefix-leak negative controls.
		//////
		{
			name:        "Should fail - component prefixed with a level name (infosvc)",
			args:        args{componentName: "other", outputName: "file"},
			content:     "infosvc:console:trace",
			wantLevel:   level.None,
			wantMatcher: None,
			wantOK:      false,
		},
		{
			name:        "Should fail - component prefixed with a level name (errorhandler)",
			args:        args{componentName: "other", outputName: "file"},
			content:     "errorhandler:console:trace",
			wantLevel:   level.None,
			wantMatcher: None,
			wantOK:      false,
		},
		{
			name:        "Should fail - output name is a substring of another output",
			args:        args{componentName: "comp", outputName: "es"},
			content:     "es-backup:trace",
			wantLevel:   level.None,
			wantMatcher: None,
			wantOK:      false,
		},
		{
			name:        "Should fail - output entry scoped to a superstring output",
			args:        args{componentName: "comp", outputName: "console"},
			content:     "console2:trace",
			wantLevel:   level.None,
			wantMatcher: None,
			wantOK:      false,
		},
		{
			name:        "Should fail - component entry scoped to a superstring component",
			args:        args{componentName: "comp", outputName: "console"},
			content:     "comp2:console:trace",
			wantLevel:   level.None,
			wantMatcher: None,
			wantOK:      false,
		},

		//////
		// Documented semantics that must be preserved.
		//////
		{
			name:        "Should fail - bare level not at the beginning (order matters)",
			args:        args{componentName: "comp", outputName: "out"},
			content:     "other:console:debug,file:trace,info",
			wantLevel:   level.None,
			wantMatcher: None,
			wantOK:      false,
		},
		{
			name:        "Should fail - empty env var",
			args:        args{componentName: "comp", outputName: "out"},
			content:     "",
			wantLevel:   level.None,
			wantMatcher: None,
			wantOK:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(shared.LevelEnvVar, tt.content)

			d := New(tt.args.componentName, tt.args.outputName)

			lvl, m, ok := d.Level()

			if ok != tt.wantOK {
				t.Fatalf("OK; expected %+v got %+v", tt.wantOK, ok)
			}

			if m != tt.wantMatcher {
				t.Fatalf("Matcher; expected %+v got %+v", tt.wantMatcher, m)
			}

			if lvl != tt.wantLevel {
				t.Fatalf("Level; expected %+v got %+v", tt.wantLevel, lvl)
			}
		})
	}
}

// TestMatchersReturnCleanEntries verifies that the `Match*` methods return
// the matched entry without the anchoring commas, keeping the extracted
// level `MustFromString`-compatible.
func TestMatchersReturnCleanEntries(t *testing.T) {
	t.Setenv(shared.LevelEnvVar, "info,comp:console:debug,file:trace")

	d := New("comp", "console")

	if got := d.matchL(); got != "info" {
		t.Fatalf("matchL; expected %q got %q", "info", got)
	}

	if got := d.matchCOL(); got != "comp:console:debug" {
		t.Fatalf("matchCOL; expected %q got %q", "comp:console:debug", got)
	}

	d = New("comp", "file")

	if got := d.matchOL(); got != "file:trace" {
		t.Fatalf("matchOL; expected %q got %q", "file:trace", got)
	}
}

// TestNewCachesRegexes verifies that the compiled regexes are reused across
// calls for the same component/output pair, while the env var content is
// still read fresh on every call.
func TestNewCachesRegexes(t *testing.T) {
	t.Setenv(shared.LevelEnvVar, "info")

	d1 := New("cachedComp", "cachedOut")

	// Runtime env var changes must keep working: content is per-call.
	t.Setenv(shared.LevelEnvVar, "trace")

	d2 := New("cachedComp", "cachedOut")

	if d1.Levels != d2.Levels ||
		d1.OutputLevels != d2.OutputLevels ||
		d1.ComponentOutputLevels != d2.ComponentOutputLevels {
		t.Fatal("compiled regexes weren't reused for the same component/output pair")
	}

	if d1.Content != "info" || d2.Content != "trace" {
		t.Fatalf("env var content should be read per call; got %q, and %q",
			d1.Content, d2.Content)
	}

	// Different pairs must not share output/component-scoped regexes.
	d3 := New("cachedComp", "anotherOut")

	if d3.OutputLevels == d1.OutputLevels {
		t.Fatal("different output names shared the same OutputLevels regex")
	}
}
