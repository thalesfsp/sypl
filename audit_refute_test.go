// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package sypl_test

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/thalesfsp/sypl/v2"
	"github.com/thalesfsp/sypl/v2/fields"
	"github.com/thalesfsp/sypl/v2/level"
	"github.com/thalesfsp/sypl/v2/message"
	"github.com/thalesfsp/sypl/v2/output"
	"github.com/thalesfsp/sypl/v2/processor"
	"github.com/thalesfsp/sypl/v2/safebuffer"
	"github.com/thalesfsp/sypl/v2/shared"
)

//////
// F1 - Parent/child logger family container sharing.
//////

// Parent, and child loggers must not share mutable slice/map CONTAINERS.
// Each logger has its own mutex, so a shared backing array turns
// parent.SetTags racing child.SetTags into two unsynchronized appends into
// the same spare-capacity slot (a data race), and a shared fields map turns
// concurrent family mutations into concurrent map writes.
//
// The output ELEMENTS stay shared by design: "changes to internals, such as
// the state of outputs, and processors, are reflected cross all other
// loggers".
func TestAudit_FamilyReconfigureRaceFree(t *testing.T) {
	_, o := output.SafeBuffer(level.Trace)

	parent := sypl.New("audit-family", o)

	// Single-element appends leave spare capacity in the backing arrays
	// (len 3, cap 4), so - pre-fix - the parent's, and the child's next
	// appends target the SAME shared slot.
	parent.SetTags("t1")
	parent.SetTags("t2")
	parent.SetTags("t3")

	parent.AddOutputs(o)
	parent.AddOutputs(o)

	parent.SetFields(fields.Fields{"inherited": "v"})

	child := parent.New("audit-family-child")

	// Phase 1: the inherited fields map must not be shared parent<->child -
	// mutating each side's map concurrently must not be a concurrent map
	// write.
	var fwg sync.WaitGroup

	fwg.Add(2)

	go func() {
		defer fwg.Done()

		for j := range 100 {
			parent.GetFields()[fmt.Sprintf("p-%d", j)] = j
		}
	}()

	go func() {
		defer fwg.Done()

		for j := range 100 {
			child.GetFields()[fmt.Sprintf("c-%d", j)] = j
		}
	}()

	fwg.Wait()

	// Phase 2: concurrently reconfigure, log, and spawn grandchildren
	// across the family.
	var wg sync.WaitGroup

	for i := 0; i < 16; i++ {
		i := i

		wg.Add(2)

		go func() {
			defer wg.Done()

			parent.SetTags(fmt.Sprintf("p-%d", i))
			parent.AddOutputs(o)
			parent.SetFields(fields.Fields{"p": i})
			parent.Infoln("parent", i)
		}()

		go func() {
			defer wg.Done()

			child.SetTags(fmt.Sprintf("c-%d", i))
			child.AddOutputs(o)
			child.SetFields(fields.Fields{"c": i})
			child.Infoln("child", i)

			grandchild := child.New(fmt.Sprintf("audit-family-gc-%d", i))
			grandchild.Infoln("grandchild", i)
		}()
	}

	wg.Wait()

	// Containers are isolated: each logger accumulated exactly its own
	// appends on top of the inherited snapshot.
	if got := len(parent.GetTags()); got != 3+16 {
		t.Fatalf("parent tags = %d, expected %d", got, 3+16)
	}

	if got := len(child.GetTags()); got != 3+16 {
		t.Fatalf("child tags = %d, expected %d", got, 3+16)
	}

	for _, tag := range parent.GetTags() {
		if strings.HasPrefix(tag, "c-") {
			t.Fatalf("parent tags contain a child tag: %v", parent.GetTags())
		}
	}

	for _, tag := range child.GetTags() {
		if strings.HasPrefix(tag, "p-") {
			t.Fatalf("child tags contain a parent tag: %v", child.GetTags())
		}
	}

	// Output ELEMENTS stay shared (documented contract): reconfiguring an
	// inherited output through the child is visible through the parent.
	child.GetOutputs()[0].SetMaxLevel(level.Error)

	if parent.GetOutputs()[0].GetMaxLevel() != level.Error {
		t.Fatal("output elements must stay shared between parent, and child")
	}
}

// A caller-provided outputs slice with spare capacity must not be aliased by
// the factory: appending through the logger must not write into the caller's
// backing array, and two loggers built from the same slice must not share
// containers.
func TestAudit_FactoryOutputsSliceNotAliased(t *testing.T) {
	_, o := output.SafeBuffer(level.Trace)

	callerOutputs := make([]output.IOutput, 0, 8)
	callerOutputs = append(callerOutputs, o)

	l1 := sypl.New("audit-factory-1", callerOutputs...)
	l2 := sypl.New("audit-factory-2", callerOutputs...)

	var wg sync.WaitGroup

	for range 16 {
		wg.Add(2)

		go func() {
			defer wg.Done()

			_, extra := output.SafeBuffer(level.Trace)
			l1.AddOutputs(extra)
		}()

		go func() {
			defer wg.Done()

			_, extra := output.SafeBuffer(level.Trace)
			l2.AddOutputs(extra)
		}()
	}

	wg.Wait()

	// The caller's spare capacity must be untouched.
	for i, spare := range callerOutputs[1:cap(callerOutputs)] {
		if spare != nil {
			t.Fatalf("caller's backing array slot %d was written to: %v", i+1, spare)
		}
	}

	if got := len(l1.GetOutputs()); got != 1+16 {
		t.Fatalf("logger 1 outputs = %d, expected %d", got, 1+16)
	}

	if got := len(l2.GetOutputs()); got != 1+16 {
		t.Fatalf("logger 2 outputs = %d, expected %d", got, 1+16)
	}
}

//////
// F2 - NewDefault/StdErr processors slice aliasing.
//////

// NewDefault must not alias the caller's processors slice (same class as the
// fixed ElasticSearchWithTagMap bug): with a spare-capacity caller slice,
// StdErr's internal append overwrote Console's MuteBasedOnLevel in the
// shared backing array - Console ended up with [Prefixer PrintOnlyAtLevel],
// muting everything except Fatal/Error, and double-printing those.
func TestAudit_NewDefaultNoProcessorAliasing(t *testing.T) {
	// Spare capacity is the trigger.
	procs := make([]processor.IProcessor, 0, 8)
	procs = append(procs, processor.Prefixer("P-"))

	l := sypl.NewDefault("aliasing", level.Info, procs...)

	consoleOut := l.GetOutput("Console")
	stderrOut := l.GetOutput("StdErr")

	// Structural: Console keeps MuteBasedOnLevel; PrintOnlyAtLevel belongs
	// to StdErr only.
	if findProcessor(consoleOut, "MuteBasedOnLevel") == nil {
		t.Fatalf("Console lost MuteBasedOnLevel: %v", consoleOut.GetProcessorsNames())
	}

	if findProcessor(consoleOut, "PrintOnlyAtLevel") != nil {
		t.Fatalf("Console gained StdErr's PrintOnlyAtLevel: %v", consoleOut.GetProcessorsNames())
	}

	if findProcessor(stderrOut, "PrintOnlyAtLevel") == nil {
		t.Fatalf("StdErr lost PrintOnlyAtLevel: %v", stderrOut.GetProcessorsNames())
	}

	// The caller's spare capacity must be untouched.
	for i, spare := range procs[1:cap(procs)] {
		if spare != nil {
			t.Fatalf("caller's backing array slot %d was written to: %v", i+1, spare)
		}
	}

	// Behavioral: Info prints exactly once, on the stdout side only;
	// Error routes to the stderr side only (Fatal shares the exact same
	// MuteBasedOnLevel/PrintOnlyAtLevel routing, but calls os.Exit).
	var bufOut, bufErr safebuffer.Buffer

	consoleOut.SetWriter(&bufOut)
	consoleOut.GetBuiltinLogger().SetOutput(&bufOut)

	stderrOut.SetWriter(&bufErr)
	stderrOut.GetBuiltinLogger().SetOutput(&bufErr)

	l.Infoln("info-msg")
	l.Errorln("err-msg")

	if got := strings.Count(bufOut.String(), "info-msg"); got != 1 {
		t.Fatalf("stdout side printed info %d time(s), expected exactly once: %q", got, bufOut.String())
	}

	if strings.Contains(bufErr.String(), "info-msg") {
		t.Fatalf("stderr side leaked an info message: %q", bufErr.String())
	}

	if strings.Contains(bufOut.String(), "err-msg") {
		t.Fatalf("stdout side leaked an error message (MuteBasedOnLevel broken): %q", bufOut.String())
	}

	if got := strings.Count(bufErr.String(), "P-err-msg"); got != 1 {
		t.Fatalf("stderr side printed error %d time(s), expected exactly once: %q", got, bufErr.String())
	}
}

//////
// F5 - Value-receiver Stringer compatibility (master surface).
//////

// A Sypl VALUE must satisfy fmt.Stringer, as on master. The mutex is held
// by pointer, so consumer copies of Sypl values are vet-copylocks clean.
var _ fmt.Stringer = sypl.Sypl{}

func TestAudit_SyplValueStringer(t *testing.T) {
	l := sypl.New("value-stringer")

	if got := fmt.Sprint(*l); got != "value-stringer" {
		t.Fatalf(`Sprintf of a Sypl value = %q, expected "value-stringer"`, got)
	}

	// Consumer copies of a Sypl value are legal again (no copylocks).
	cp := *l

	if got := cp.String(); got != "value-stringer" {
		t.Fatalf(`copied value String() = %q, expected "value-stringer"`, got)
	}

	// A zero-value Sypl stays usable, as on master (and as with the
	// by-value mutex): empty name, no-op logging, reconfigurable.
	//
	// V2: `SetName` is gone - the name is fixed at construction - so a
	// zero-value logger keeps its empty name.
	var zero sypl.Sypl

	if got := zero.String(); got != "" {
		t.Fatalf(`zero-value String() = %q, expected ""`, got)
	}

	zero.SetTags("zt")

	if got := fmt.Sprint(zero); got != "" {
		t.Fatalf(`zero-value Sprintf = %q, expected ""`, got)
	}

	// No outputs: must be a silent no-op, not a panic.
	zero.Infoln("goes nowhere")
}

//////
// F3 - SYPL_FILTER semantics: sanctioned drift from v1.9.17.
//////

// SYPL_FILTER is an exact, case-insensitive component-name match per
// comma-separated entry. Both cases below are DELIBERATE behavior changes
// from v1.9.17, which used a case-SENSITIVE substring match
// (strings.Contains(filter, name)):
//
//  1. Filter "svc-worker" with component "svc": v1.9.17 PRINTED (the
//     component was a substring of the filter, the accidental direction).
//     Now: SILENT - a filter entry only matches the exact component name.
//  2. Filter "SVC" with component "svc": v1.9.17 was SILENT (case
//     mismatch). Now: PRINTS - case-insensitive is the chosen semantic,
//     consistent with GetOutput's EqualFold precedent.
func TestAudit_FilterSemanticsDrift(t *testing.T) {
	t.Run("filter superstring of component is silent", func(t *testing.T) {
		t.Setenv(shared.FilterEnvVar, "svc-worker")

		buf, o := output.SafeBuffer(level.Trace)

		l := sypl.New("svc", o)

		l.Infoln("should-not-print")

		if buf.String() != "" {
			t.Fatalf(`filter "svc-worker" must not match component "svc": %q`, buf.String())
		}
	})

	t.Run("filter matches case-insensitively", func(t *testing.T) {
		t.Setenv(shared.FilterEnvVar, "SVC")

		buf, o := output.SafeBuffer(level.Trace)

		l := sypl.New("svc", o)

		l.Infoln("should-print")

		if !strings.Contains(buf.String(), "should-print") {
			t.Fatalf(`filter "SVC" must match component "svc" (EqualFold): %q`, buf.String())
		}
	})
}

//////
// F4 - Output/processor dispatch: exact, case-insensitive (EqualFold).
//////

// Output dispatch by name is an exact, case-insensitive match - deliberate,
// consistent with GetOutput's EqualFold precedent: WithOutputsNames
// ("console") targets the output named "Console".
func TestAudit_OutputDispatchCaseInsensitive(t *testing.T) {
	bufConsole, oConsole := namedSafeBuffer("Console", level.Trace)

	bufOther, oOther := namedSafeBuffer("Other", level.Trace)

	l := sypl.New("dispatch-case", oConsole, oOther)

	l.PrintWithOptions(level.Info, "cased\n", sypl.WithOutputsNames("console"))

	if !strings.Contains(bufConsole.String(), "cased") {
		t.Fatalf(`WithOutputsNames("console") must match output "Console": %q`, bufConsole.String())
	}

	if bufOther.String() != "" {
		t.Fatalf(`output "Other" received a message targeted at "console": %q`, bufOther.String())
	}
}

// Processor dispatch by name follows the same exact, case-insensitive
// semantic: WithProcessorsNames("prefixer") runs the processor named
// "Prefixer".
func TestAudit_ProcessorDispatchCaseInsensitive(t *testing.T) {
	prefixer := processor.New("Prefixer", func(m message.IMessage) error {
		m.GetContent().SetProcessed("PREFIXER-" + m.GetContent().GetProcessed())

		return nil
	})

	suffixer := processor.New("Suffixer", func(m message.IMessage) error {
		m.GetContent().SetProcessed(m.GetContent().GetProcessed() + "-SUFFIXER")

		return nil
	})

	buf, o := output.SafeBuffer(level.Trace, prefixer, suffixer)

	l := sypl.New("dispatch-case", o)

	l.PrintWithOptions(level.Info, "x\n", sypl.WithProcessorsNames("prefixer"))

	if !strings.Contains(buf.String(), "PREFIXER-x") {
		t.Fatalf(`WithProcessorsNames("prefixer") must run processor "Prefixer": %q`, buf.String())
	}

	if strings.Contains(buf.String(), "SUFFIXER") {
		t.Fatalf(`processor "Suffixer" ran though only "prefixer" was requested: %q`, buf.String())
	}
}
