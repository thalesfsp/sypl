// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package processor

import (
	"fmt"
	"testing"

	"github.com/thalesfsp/sypl/v2/color"
	"github.com/thalesfsp/sypl/v2/flag"
	"github.com/thalesfsp/sypl/v2/level"
	"github.com/thalesfsp/sypl/v2/message"
	"github.com/thalesfsp/sypl/v2/shared"
)

//////
// Test helpers.
//////

// fakeColor deterministically marks the content - tests must not depend on
// ANSI codes being enabled (they aren't, on a non-TTY).
func fakeColor(a ...interface{}) string {
	return "<C>" + fmt.Sprint(a...) + "</C>"
}

// runProcessor runs `p` against a fresh message with the given level, and
// content, failing the test on error.
func runProcessor(t *testing.T, p IProcessor, l level.Level, content string) message.IMessage {
	t.Helper()

	m := message.New(l, content)

	if err := p.Run(m); err != nil {
		t.Fatalf("Run failed: %s", err)
	}

	return m
}

//////
// ChangeFirstCharCase.
//////

func TestChangeFirstCharCase(t *testing.T) {
	tests := []struct {
		name    string
		casing  Casing
		content string
		want    string
	}{
		{
			name:    "Should work - uppercase",
			casing:  Uppercase,
			content: "hello world",
			want:    "Hello world",
		},
		{
			name:    "Should work - lowercase",
			casing:  Lowercase,
			content: "HELLO WORLD",
			want:    "hELLO WORLD",
		},
		{
			name:    "Should work - single char",
			casing:  Uppercase,
			content: "a",
			want:    "A",
		},
		{
			name:    "Should work - multibyte char isn't corrupted",
			casing:  Uppercase,
			content: "école",
			want:    "École",
		},
		{
			name:    "Should work - multibyte char lowercase",
			casing:  Lowercase,
			content: "École",
			want:    "école",
		},
		{
			name:    "Should work - already the requested case",
			casing:  Uppercase,
			content: "Hello",
			want:    "Hello",
		},
		{
			name:    "Should do nothing - empty content",
			casing:  Uppercase,
			content: "",
			want:    "",
		},
		{
			name:    "Should do nothing - unknown casing",
			casing:  Casing("titlecase"),
			content: "hello",
			want:    "hello",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := runProcessor(t, ChangeFirstCharCase(tt.casing), level.Info, tt.content)

			if got := m.GetContent().GetProcessed(); got != tt.want {
				t.Errorf("ChangeFirstCharCase() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestChangeFirstCharCase_EmptyProcessedContent(t *testing.T) {
	// Non-empty original, but the processed content was emptied by an
	// earlier processor - nothing to change, and no panic.
	m := message.New(level.Info, shared.DefaultContentOutput)

	m.GetContent().SetProcessed("")

	if err := ChangeFirstCharCase(Uppercase).Run(m); err != nil {
		t.Fatalf("Run failed: %s", err)
	}

	if got := m.GetContent().GetProcessed(); got != "" {
		t.Errorf("Processed content = %q, want empty", got)
	}
}

//////
// Colorizers.
//////

func TestColorizeBasedOnLevel(t *testing.T) {
	tests := []struct {
		name  string
		level level.Level
		want  string
	}{
		{
			name:  "Should colorize - level matches",
			level: level.Info,
			want:  "<C>" + shared.DefaultContentOutput + "</C>",
		},
		{
			name:  "Should not colorize - level doesn't match",
			level: level.Warn,
			want:  shared.DefaultContentOutput,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := ColorizeBasedOnLevel(map[level.Level]color.Color{level.Info: fakeColor})

			m := runProcessor(t, p, tt.level, shared.DefaultContentOutput)

			if got := m.GetContent().GetProcessed(); got != tt.want {
				t.Errorf("ColorizeBasedOnLevel() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestColorizeBasedOnWord(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "Should colorize - word present",
			content: "an error occurred",
			want:    "<C>an error occurred</C>",
		},
		{
			name:    "Should not colorize - word absent",
			content: "all good",
			want:    "all good",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := ColorizeBasedOnWord(map[string]color.Color{"error": fakeColor})

			m := runProcessor(t, p, level.Info, tt.content)

			if got := m.GetContent().GetProcessed(); got != tt.want {
				t.Errorf("ColorizeBasedOnWord() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDecolourizer(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "Should strip ANSI codes",
			content: "\x1b[31mred\x1b[0m",
			want:    "red",
		},
		{
			name:    "Should pass through - no ANSI codes",
			content: shared.DefaultContentOutput,
			want:    shared.DefaultContentOutput,
		},
		{
			name:    "Should pass through - empty content",
			content: "",
			want:    "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := runProcessor(t, Decolourizer(), level.Info, tt.content)

			if got := m.GetContent().GetProcessed(); got != tt.want {
				t.Errorf("Decolourizer() = %q, want %q", got, tt.want)
			}
		})
	}
}

//////
// ErrorSimulator, and Flagger.
//////

func TestErrorSimulator(t *testing.T) {
	p := ErrorSimulator("simulated failure")

	if p.GetName() != "ErrorSimulator" {
		t.Errorf("GetName() = %q, want %q", p.GetName(), "ErrorSimulator")
	}

	m := message.New(level.Info, shared.DefaultContentOutput)

	err := p.Run(m)
	if err == nil {
		t.Fatal("ErrorSimulator should always fail")
	}

	if err.Error() != "simulated failure" {
		t.Errorf("Error = %q, want %q", err.Error(), "simulated failure")
	}
}

func TestFlagger(t *testing.T) {
	tests := []struct {
		name string
		flag flag.Flag
	}{
		{name: "Should set Force", flag: flag.Force},
		{name: "Should set Mute", flag: flag.Mute},
		{name: "Should set None", flag: flag.None},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := runProcessor(t, Flagger(tt.flag), level.Info, shared.DefaultContentOutput)

			if got := m.GetFlag(); got != tt.flag {
				t.Errorf("Flagger() flag = %v, want %v", got, tt.flag)
			}
		})
	}
}

//////
// Level-based flaggers.
//////

func TestForceBasedOnLevel(t *testing.T) {
	tests := []struct {
		name   string
		levels []level.Level
		level  level.Level
		want   flag.Flag
	}{
		{
			name:   "Should force - level in set",
			levels: []level.Level{level.Debug, level.Trace},
			level:  level.Debug,
			want:   flag.Force,
		},
		{
			name:   "Should not force - level not in set",
			levels: []level.Level{level.Debug, level.Trace},
			level:  level.Info,
			want:   flag.None,
		},
		{
			name:   "Should not force - empty set",
			levels: []level.Level{},
			level:  level.Info,
			want:   flag.None,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := runProcessor(t, ForceBasedOnLevel(tt.levels...), tt.level, shared.DefaultContentOutput)

			if got := m.GetFlag(); got != tt.want {
				t.Errorf("ForceBasedOnLevel() flag = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMuteBasedOnLevel(t *testing.T) {
	tests := []struct {
		name   string
		levels []level.Level
		level  level.Level
		want   flag.Flag
	}{
		{
			name:   "Should mute - level in set",
			levels: []level.Level{level.Debug, level.Trace},
			level:  level.Trace,
			want:   flag.Mute,
		},
		{
			name:   "Should not mute - level not in set",
			levels: []level.Level{level.Debug, level.Trace},
			level:  level.Error,
			want:   flag.None,
		},
		{
			name:   "Should not mute - empty set",
			levels: []level.Level{},
			level:  level.Error,
			want:   flag.None,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := runProcessor(t, MuteBasedOnLevel(tt.levels...), tt.level, shared.DefaultContentOutput)

			if got := m.GetFlag(); got != tt.want {
				t.Errorf("MuteBasedOnLevel() flag = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPrintOnlyAtLevel_EmptySet(t *testing.T) {
	// An empty set means no level is allowed - everything is muted.
	m := runProcessor(t, PrintOnlyAtLevel(), level.Info, shared.DefaultContentOutput)

	if got := m.GetFlag(); got != flag.Mute {
		t.Errorf("PrintOnlyAtLevel() flag = %v, want %v", got, flag.Mute)
	}
}

//////
// Prefix masks.
//////

func TestPrefixBasedOnMask(t *testing.T) {
	m := message.New(level.Info, shared.DefaultContentOutput)

	m.SetComponentName(shared.DefaultComponentNameOutput)

	if err := PrefixBasedOnMask(shared.DefaultTimestampFormat).Run(m); err != nil {
		t.Fatalf("Run failed: %s", err)
	}

	want := generateDefaultPrefix(
		m.GetTimestamp().Format(shared.DefaultTimestampFormat),
		shared.DefaultComponentNameOutput,
		level.Info,
	) + shared.DefaultContentOutput

	if got := m.GetContent().GetProcessed(); got != want {
		t.Errorf("PrefixBasedOnMask() = %q, want %q", got, want)
	}
}

func TestPrefixBasedOnMaskExceptForLevels(t *testing.T) {
	tests := []struct {
		name       string
		exceptions []level.Level
		level      level.Level
		wantPrefix bool
	}{
		{
			name:       "Should not prefix - level is an exception",
			exceptions: []level.Level{level.Info},
			level:      level.Info,
			wantPrefix: false,
		},
		{
			name:       "Should prefix - level isn't an exception",
			exceptions: []level.Level{level.Info},
			level:      level.Warn,
			wantPrefix: true,
		},
		{
			name:       "Should prefix - empty exceptions",
			exceptions: []level.Level{},
			level:      level.Info,
			wantPrefix: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := message.New(tt.level, shared.DefaultContentOutput)

			m.SetComponentName(shared.DefaultComponentNameOutput)

			p := PrefixBasedOnMaskExceptForLevels(shared.DefaultTimestampFormat, tt.exceptions...)

			if err := p.Run(m); err != nil {
				t.Fatalf("Run failed: %s", err)
			}

			want := shared.DefaultContentOutput

			if tt.wantPrefix {
				want = generateDefaultPrefix(
					m.GetTimestamp().Format(shared.DefaultTimestampFormat),
					shared.DefaultComponentNameOutput,
					tt.level,
				) + shared.DefaultContentOutput
			}

			if got := m.GetContent().GetProcessed(); got != want {
				t.Errorf("PrefixBasedOnMaskExceptForLevels() = %q, want %q", got, want)
			}
		})
	}
}

//////
// Tag-based processors.
//////

func TestPrintOnlyIfTagged(t *testing.T) {
	tests := []struct {
		name string
		tags []string
		want flag.Flag
	}{
		{
			name: "Should not mute - message has the tag",
			tags: []string{"audit"},
			want: flag.None,
		},
		{
			name: "Should not mute - message has the tag among others",
			tags: []string{"other", "audit"},
			want: flag.None,
		},
		{
			name: "Should mute - message doesn't have the tag",
			tags: []string{"other"},
			want: flag.Mute,
		},
		{
			name: "Should mute - message has no tags",
			tags: nil,
			want: flag.Mute,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := message.New(level.Info, shared.DefaultContentOutput)

			m.AddTags(tt.tags...)

			if err := PrintOnlyIfTagged("audit").Run(m); err != nil {
				t.Fatalf("Run failed: %s", err)
			}

			if got := m.GetFlag(); got != tt.want {
				t.Errorf("PrintOnlyIfTagged() flag = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPrintOnlyIfNotTaggedWith(t *testing.T) {
	tests := []struct {
		name        string
		denylist    []string
		messageTags []string
		want        flag.Flag
	}{
		{
			name:        "Should mute - message has a denied tag",
			denylist:    []string{"internal", "secret"},
			messageTags: []string{"secret"},
			want:        flag.Mute,
		},
		{
			name:        "Should not mute - message has no denied tag",
			denylist:    []string{"internal", "secret"},
			messageTags: []string{"public"},
			want:        flag.None,
		},
		{
			name:        "Should not mute - message has no tags",
			denylist:    []string{"internal"},
			messageTags: nil,
			want:        flag.None,
		},
		{
			name:        "Should not mute - empty denylist",
			denylist:    nil,
			messageTags: []string{"anything"},
			want:        flag.None,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := message.New(level.Info, shared.DefaultContentOutput)

			m.AddTags(tt.messageTags...)

			if err := PrintOnlyIfNotTaggedWith(tt.denylist...).Run(m); err != nil {
				t.Fatalf("Run failed: %s", err)
			}

			if got := m.GetFlag(); got != tt.want {
				t.Errorf("PrintOnlyIfNotTaggedWith() flag = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTagger(t *testing.T) {
	tests := []struct {
		name string
		tags []string
	}{
		{name: "Should add one tag", tags: []string{"alpha"}},
		{name: "Should add multiple tags", tags: []string{"alpha", "beta"}},
		{name: "Should add no tags", tags: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := runProcessor(t, Tagger(tt.tags...), level.Info, shared.DefaultContentOutput)

			for _, tag := range tt.tags {
				if !m.ContainTag(tag) {
					t.Errorf("Message should contain tag %q, has %v", tag, m.GetTags())
				}
			}

			if len(m.GetTags()) != len(tt.tags) {
				t.Errorf("GetTags() = %v, want %d tags", m.GetTags(), len(tt.tags))
			}
		})
	}
}

//////
// processor.go remaining surface.
//////

func TestProcessor_NameAndString(t *testing.T) {
	p := New("Original", func(m message.IMessage) error { return nil })

	if p.GetName() != "Original" {
		t.Errorf("GetName() = %q, want %q", p.GetName(), "Original")
	}

	if fmt.Sprint(p) != "Original" {
		t.Errorf("String() = %q, want %q", fmt.Sprint(p), "Original")
	}
}
