// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package message

import (
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/thalesfsp/sypl/content"
	"github.com/thalesfsp/sypl/debug"
	"github.com/thalesfsp/sypl/fields"
	"github.com/thalesfsp/sypl/flag"
	"github.com/thalesfsp/sypl/level"
	"github.com/thalesfsp/sypl/status"
)

// String() must return the processed content.
func TestMessage_String(t *testing.T) {
	m := New(level.Info, "original")

	if got := fmt.Sprint(m); got != "original" {
		t.Fatalf("String() = %q, expected %q", got, "original")
	}

	m.GetContent().SetProcessed("processed")

	if got := fmt.Sprint(m); got != "processed" {
		t.Fatalf("String() after processing = %q, expected %q", got, "processed")
	}
}

// ContainTag: happy (present) and bad (absent) paths.
func TestMessage_ContainTag(t *testing.T) {
	m := New(level.Info, "x")

	m.AddTags("present")

	if !m.ContainTag("present") {
		t.Fatal("ContainTag(present) = false, expected true")
	}

	if m.ContainTag("absent") {
		t.Fatal("ContainTag(absent) = true, expected false")
	}
}

// GetTags must return the tags lexicographically sorted.
func TestMessage_GetTagsSorted(t *testing.T) {
	m := New(level.Info, "x")

	m.AddTags("zebra", "alpha", "middle")

	if got := m.GetTags(); !reflect.DeepEqual(got, []string{"alpha", "middle", "zebra"}) {
		t.Fatalf("GetTags = %v, expected sorted [alpha middle zebra]", got)
	}
}

// SetContent must replace the content.
func TestMessage_SetContent(t *testing.T) {
	m := New(level.Info, "old")

	m.SetContent(content.New("new"))

	if m.GetContent().GetOriginal() != "new" || m.GetContent().GetProcessed() != "new" {
		t.Fatalf("SetContent: original %q, processed %q, expected both %q",
			m.GetContent().GetOriginal(), m.GetContent().GetProcessed(), "new")
	}
}

// SetLevel must replace the level.
func TestMessage_SetLevel(t *testing.T) {
	m := New(level.Info, "x")

	m.SetLevel(level.Warn)

	if m.GetLevel() != level.Warn {
		t.Fatalf("SetLevel: level = %s, expected warn", m.GetLevel())
	}
}

// IsEmpty is based on the ORIGINAL content, trimming whitespace/control chars.
func TestMessage_IsEmpty(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{"empty", "", true},
		{"only whitespace and control chars", " \f\t\r\n ", true},
		{"non-empty", "x", false},
		{"non-empty with padding", "  x\n", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := New(level.Info, tt.content).IsEmpty(); got != tt.want {
				t.Fatalf("IsEmpty(%q) = %v, expected %v", tt.content, got, tt.want)
			}
		})
	}
}

//////
// util.go: generateID / generateUUID.
//////

// generateUUID must produce a well-formed UUIDv4, and must survive a failing
// random source (error branch) without panicking.
func TestGenerateUUID_WellFormedAndErrorPath(t *testing.T) {
	uuidRegex := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[0-9a-f]{4}-[0-9a-f]{12}$`)

	got := generateUUID()

	if !uuidRegex.MatchString(got) {
		t.Fatalf("generateUUID() = %q, expected a well-formed UUIDv4", got)
	}

	// Two calls must differ (random-based).
	if generateUUID() == generateUUID() {
		t.Fatal("generateUUID() returned the same value twice")
	}

	// Error branch: a failing random source must not panic; the zero UUID is
	// returned, still 36 chars long.
	uuid.SetRand(failingReader{})

	defer uuid.SetRand(nil)

	gotErr := generateUUID()

	if len(gotErr) != 36 {
		t.Fatalf("generateUUID() on rand failure = %q, expected a 36-char UUID string", gotErr)
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) {
	return 0, errors.New("rand is broken")
}

// generateID: deterministic for same content, distinct for different content,
// well-formed (40 hex chars, SHA-1), and whitespace-trimmed.
func TestGenerateID_Properties(t *testing.T) {
	hexRegex := regexp.MustCompile(`^[0-9a-f]{40}$`)

	idA1 := generateID("content-a")
	idA2 := generateID("content-a")
	idB := generateID("content-b")

	if !hexRegex.MatchString(idA1) {
		t.Fatalf("generateID() = %q, expected 40 hex chars", idA1)
	}

	if idA1 != idA2 {
		t.Fatalf("generateID not deterministic: %q != %q", idA1, idA2)
	}

	if idA1 == idB {
		t.Fatal("generateID returned the same hash for different content")
	}

	// Content is trimmed before hashing.
	if generateID("content-a\n") != idA1 {
		t.Fatal("generateID must trim trailing control chars before hashing")
	}
}

//////
// lineBreaker edge cases.
//////

// Content of only "\n": Strip leaves empty processed content, Restore brings
// the line break back.
func TestLineBreaker_OnlyNewline(t *testing.T) {
	m := New(level.Info, "\n")

	m.Strip()

	if got := m.GetContent().GetProcessed(); got != "" {
		t.Fatalf("Strip(%q): processed = %q, expected empty", "\n", got)
	}

	if !reflect.DeepEqual(m.getLineBreaker().ControlChars, []string{"\n"}) {
		t.Fatalf("ControlChars = %v, expected [\\n]", m.getLineBreaker().ControlChars)
	}

	m.Restore()

	if got := m.GetContent().GetProcessed(); got != "\n" {
		t.Fatalf("Restore: processed = %q, expected %q", got, "\n")
	}
}

// Empty content: Strip/Restore are no-ops.
func TestLineBreaker_EmptyContent(t *testing.T) {
	m := New(level.Info, "")

	m.Strip()

	if got := m.GetContent().GetProcessed(); got != "" {
		t.Fatalf("Strip(empty): processed = %q, expected empty", got)
	}

	if len(m.getLineBreaker().ControlChars) != 0 {
		t.Fatalf("ControlChars = %v, expected none", m.getLineBreaker().ControlChars)
	}

	m.Restore()

	if got := m.GetContent().GetProcessed(); got != "" {
		t.Fatalf("Restore(empty): processed = %q, expected empty", got)
	}
}

// A disabled lineBreaker turns Strip and Restore into no-ops.
func TestLineBreaker_Disabled(t *testing.T) {
	m := New(level.Info, "x\n")

	m.getLineBreaker().Status = status.Disabled

	m.Strip()

	if got := m.GetContent().GetProcessed(); got != "x\n" {
		t.Fatalf("Strip with disabled lineBreaker changed content: %q", got)
	}

	if len(m.getLineBreaker().ControlChars) != 0 {
		t.Fatalf("Strip with disabled lineBreaker accumulated chars: %v", m.getLineBreaker().ControlChars)
	}

	m.Restore()

	if got := m.GetContent().GetProcessed(); got != "x\n" {
		t.Fatalf("Restore with disabled lineBreaker changed content: %q", got)
	}
}

//////
// Copy round-trip fidelity.
//////

// Copy must reproduce every field, deep-copying tags and fields so the copy
// is independent of the original.
func TestCopy_RoundTripFidelity(t *testing.T) {
	ts := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	d := debug.New("comp", "out1")

	m := New(level.Warn, "body\r\n")
	m.SetComponentName("comp")
	m.SetContentBasedHashID("hash-123")
	m.SetDebugEnvVarRegexes(d)
	m.SetFields(fields.Fields{"k1": "v1", "k2": 2})
	m.SetFlag(flag.Force)
	m.SetID("id-1")
	m.SetOutputName("out1")
	m.SetOutputsNames([]string{"o1", "o2"})
	m.SetProcessorName("p1")
	m.SetProcessorsNames([]string{"p1", "p2"})
	m.SetTimestamp(ts)
	m.GetMessage().Tags = []string{"t1", "t2"}
	m.AddTags("t1", "t2")

	// Populate the lineBreaker state too.
	m.Strip()

	c := Copy(m)

	if c.GetLevel() != level.Warn {
		t.Fatalf("level = %s, expected warn", c.GetLevel())
	}

	if c.GetContent().GetOriginal() != "body\r\n" {
		t.Fatalf("original content = %q, expected %q", c.GetContent().GetOriginal(), "body\r\n")
	}

	if c.GetComponentName() != "comp" {
		t.Fatalf("component = %q, expected comp", c.GetComponentName())
	}

	if c.GetContentBasedHashID() != "hash-123" {
		t.Fatalf("content-based hash = %q, expected hash-123", c.GetContentBasedHashID())
	}

	if c.GetDebugEnvVarRegexes() != d {
		t.Fatal("debug regexes not copied")
	}

	if !reflect.DeepEqual(c.GetFields(), fields.Fields{"k1": "v1", "k2": 2}) {
		t.Fatalf("fields = %v", c.GetFields())
	}

	if c.GetFlag() != flag.Force {
		t.Fatalf("flag = %v, expected Force", c.GetFlag())
	}

	if c.GetID() != "id-1" {
		t.Fatalf("id = %q, expected id-1", c.GetID())
	}

	if c.GetOutputName() != "out1" {
		t.Fatalf("output name = %q, expected out1", c.GetOutputName())
	}

	if !reflect.DeepEqual(c.GetOutputsNames(), []string{"o1", "o2"}) {
		t.Fatalf("outputs names = %v", c.GetOutputsNames())
	}

	if c.GetProcessorName() != "p1" {
		t.Fatalf("processor name = %q, expected p1", c.GetProcessorName())
	}

	if !reflect.DeepEqual(c.GetProcessorsNames(), []string{"p1", "p2"}) {
		t.Fatalf("processors names = %v", c.GetProcessorsNames())
	}

	if !c.GetTimestamp().Equal(ts) {
		t.Fatalf("timestamp = %v, expected %v", c.GetTimestamp(), ts)
	}

	if !reflect.DeepEqual(c.GetMessage().Tags, []string{"t1", "t2"}) {
		t.Fatalf("options tags = %v", c.GetMessage().Tags)
	}

	if !reflect.DeepEqual(c.GetTags(), []string{"t1", "t2"}) {
		t.Fatalf("tags = %v", c.GetTags())
	}

	// lineBreaker state (control chars stripped from "body\r\n": "\n" then
	// "\r") must carry over as an independent copy.
	if !reflect.DeepEqual(c.getLineBreaker().ControlChars, m.getLineBreaker().ControlChars) {
		t.Fatalf("lineBreaker control chars = %v, expected %v",
			c.getLineBreaker().ControlChars, m.getLineBreaker().ControlChars)
	}

	if c.getLineBreaker() == m.getLineBreaker() {
		t.Fatal("lineBreaker is shared between copy and original")
	}

	// Independence: mutating the copy must not leak into the original.
	c.AddTags("copy-only")

	if m.ContainTag("copy-only") {
		t.Fatal("tags map is shared between copy and original")
	}

	c.GetFields()["copy-only"] = true

	if _, leaked := m.GetFields()["copy-only"]; leaked {
		t.Fatal("fields map is shared between copy and original")
	}

	c.GetMessage().Tags[0] = "mutated"

	if m.GetMessage().Tags[0] != "t1" {
		t.Fatal("options tags slice is shared between copy and original")
	}
}
