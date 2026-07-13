// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

// End-to-end mocked pipeline tests: logger -> global fields/tags merge ->
// processors -> formatter -> output writer.
package sypl_test

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/thalesfsp/sypl/v2"
	"github.com/thalesfsp/sypl/v2/fields"
	"github.com/thalesfsp/sypl/v2/flag"
	"github.com/thalesfsp/sypl/v2/formatter"
	"github.com/thalesfsp/sypl/v2/level"
	"github.com/thalesfsp/sypl/v2/message"
	"github.com/thalesfsp/sypl/v2/output"
	"github.com/thalesfsp/sypl/v2/processor"
)

// (a) Multi-output routing: a message targeted at one of three outputs must
// reach only that output.
func TestE2E_MultiOutputRouting(t *testing.T) {
	bufA, oA := output.SafeBuffer(level.Trace)
	oA.SetName("A")

	bufB, oB := output.SafeBuffer(level.Trace)
	oB.SetName("B")

	bufC, oC := output.SafeBuffer(level.Trace)
	oC.SetName("C")

	l := sypl.New("e2e-routing", oA, oB, oC)

	l.PrintWithOptions(level.Info, "routed\n", sypl.WithOutputsNames("B"))

	if bufA.String() != "" {
		t.Fatalf("output A received a message targeted at B: %q", bufA.String())
	}

	if bufB.String() != "routed\n" {
		t.Fatalf("output B got %q, expected %q", bufB.String(), "routed\n")
	}

	if bufC.String() != "" {
		t.Fatalf("output C received a message targeted at B: %q", bufC.String())
	}
}

// (b) Processor chain order: processors run in registration order, and their
// effects compose.
func TestE2E_ProcessorChainOrder(t *testing.T) {
	first := processor.New("First", func(m message.IMessage) error {
		m.GetContent().SetProcessed(m.GetContent().GetProcessed() + "|1")

		return nil
	})

	second := processor.New("Second", func(m message.IMessage) error {
		m.GetContent().SetProcessed(m.GetContent().GetProcessed() + "|2")

		return nil
	})

	buf, o := output.SafeBuffer(level.Trace, first, second)

	l := sypl.New("e2e-chain", o)

	l.Println(level.Info, "core")

	// Order-sensitive: "core|1|2" proves First ran before Second; the line
	// break is stripped before processing and restored after.
	if buf.String() != "core|1|2\n" {
		t.Fatalf("processor chain produced %q, expected %q", buf.String(), "core|1|2\n")
	}
}

// (c) JSON formatter end-to-end: the emitted line must be valid JSON carrying
// component, level, message, fields, and tags.
func TestE2E_JSONFormatter(t *testing.T) {
	buf, o := output.SafeBuffer(level.Trace)
	o.SetFormatter(formatter.JSON())

	l := sypl.New("e2e-json", o)

	l.PrintWithOptions(
		level.Info,
		"json-msg\n",
		sypl.WithFields(fields.Fields{"fkey": "fval"}),
		sypl.WithTags("tag-a"),
	)

	line := strings.TrimSuffix(buf.String(), "\n")

	decoded := map[string]interface{}{}
	if err := json.Unmarshal([]byte(line), &decoded); err != nil {
		t.Fatalf("output is not valid JSON: %v: %q", err, line)
	}

	if decoded["component"] != "e2e-json" {
		t.Fatalf("component = %v, expected e2e-json", decoded["component"])
	}

	if decoded["level"] != "info" {
		t.Fatalf("level = %v, expected info", decoded["level"])
	}

	if decoded["message"] != "json-msg" {
		t.Fatalf("message = %v, expected json-msg", decoded["message"])
	}

	if decoded["fkey"] != "fval" {
		t.Fatalf("field fkey = %v, expected fval", decoded["fkey"])
	}

	tags, ok := decoded["tags"].([]interface{})
	if !ok || len(tags) != 1 || tags[0] != "tag-a" {
		t.Fatalf("tags = %v, expected [tag-a]", decoded["tags"])
	}
}

// (d) Muted end-to-end: a level above the output's max level, and an
// explicitly muted message, must both produce no output.
func TestE2E_MutedSilent(t *testing.T) {
	buf, o := output.SafeBuffer(level.Info)

	l := sypl.New("e2e-muted", o)

	// Above max level.
	l.Debugln("above-max-level")

	if buf.String() != "" {
		t.Fatalf("message above max level was printed: %q", buf.String())
	}

	// Explicit mute at an allowed level.
	l.PrintWithOptions(level.Info, "muted\n", sypl.WithFlag(flag.Mute))

	if buf.String() != "" {
		t.Fatalf("muted message was printed: %q", buf.String())
	}
}

// (e) Forced end-to-end: the Force flag must print despite the level being
// above the output's max level.
func TestE2E_ForcedPrints(t *testing.T) {
	buf, o := output.SafeBuffer(level.Error)

	l := sypl.New("e2e-forced", o)

	// Negative control: without Force, trace against an error-max output is
	// silent.
	l.Traceln("not-forced")

	if buf.String() != "" {
		t.Fatalf("non-forced trace message was printed: %q", buf.String())
	}

	l.PrintWithOptions(level.Trace, "forced\n", sypl.WithFlag(flag.Force))

	if buf.String() != "forced\n" {
		t.Fatalf("forced message printed %q, expected %q", buf.String(), "forced\n")
	}
}

// (f) Global fields + per-message fields: both must land in the output, and
// the per-message value must win on conflict.
func TestE2E_FieldsPrecedence(t *testing.T) {
	buf, o := output.SafeBuffer(level.Trace)
	o.SetFormatter(formatter.JSON())

	l := sypl.New("e2e-fields", o)
	l.SetFields(fields.Fields{"env": "global", "shared": "global-value"})

	l.PrintWithOptions(level.Info, "fields-msg\n", sypl.WithFields(fields.Fields{"shared": "message-value"}))

	line := strings.TrimSuffix(buf.String(), "\n")

	decoded := map[string]interface{}{}
	if err := json.Unmarshal([]byte(line), &decoded); err != nil {
		t.Fatalf("output is not valid JSON: %v: %q", err, line)
	}

	if decoded["env"] != "global" {
		t.Fatalf("global field env = %v, expected global", decoded["env"])
	}

	if decoded["shared"] != "message-value" {
		t.Fatalf("per-message field must win: shared = %v, expected message-value", decoded["shared"])
	}
}

// (g) Concurrent end-to-end: 50 goroutines x 20 messages into one output.
// Exactly 1000 intact lines must come out - no losses, no duplicates, no
// interleaving corruption. Run under -race.
func TestE2E_Concurrent(t *testing.T) {
	const (
		goroutines          = 50
		messagesPerRoutine  = 20
		expectedTotalOfRows = goroutines * messagesPerRoutine
	)

	buf, o := output.SafeBuffer(level.Trace)

	l := sypl.New("e2e-concurrent", o)

	var wg sync.WaitGroup

	wg.Add(goroutines)

	for g := range goroutines {
		go func(g int) {
			defer wg.Done()

			for j := range messagesPerRoutine {
				l.Printlnf(level.Info, "g%02d-m%02d", g, j)
			}
		}(g)
	}

	wg.Wait()

	got := buf.String()

	lines := strings.Split(strings.TrimSuffix(got, "\n"), "\n")

	if len(lines) != expectedTotalOfRows {
		t.Fatalf("got %d lines, expected %d", len(lines), expectedTotalOfRows)
	}

	// Every line must be intact (no interleaving corruption), and every
	// expected line must appear exactly once.
	lineRegex := regexp.MustCompile(`^g\d{2}-m\d{2}$`)

	counts := map[string]int{}

	for _, line := range lines {
		if !lineRegex.MatchString(line) {
			t.Fatalf("corrupted line: %q", line)
		}

		counts[line]++
	}

	for g := range goroutines {
		for j := range messagesPerRoutine {
			expected := fmt.Sprintf("g%02d-m%02d", g, j)

			if counts[expected] != 1 {
				t.Fatalf("line %q appeared %d times, expected exactly 1", expected, counts[expected])
			}
		}
	}
}
