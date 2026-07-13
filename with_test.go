// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package sypl_test

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/thalesfsp/sypl"
	"github.com/thalesfsp/sypl/fields"
	"github.com/thalesfsp/sypl/formatter"
	"github.com/thalesfsp/sypl/level"
	"github.com/thalesfsp/sypl/output"
	"github.com/thalesfsp/sypl/safebuffer"
	"github.com/thalesfsp/sypl/status"
)

// Shared expected values - hoisted so repeated literals stay lint-clean.
const (
	envProd  = "prod"
	whoChild = "child"
)

// jsonLine decodes the last JSON line written to buf.
func jsonLine(t *testing.T, buf *safebuffer.Buffer) map[string]interface{} {
	t.Helper()

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")

	decoded := map[string]interface{}{}
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &decoded); err != nil {
		t.Fatalf("output is not valid JSON: %v: %q", err, buf.String())
	}

	return decoded
}

// A derived logger emits the parent's fields merged with its own - the
// child's fields win on key conflict - while the PARENT stays untouched.
func TestWith_MergedFieldsChildWins(t *testing.T) {
	buf, o := output.SafeBuffer(level.Trace)
	o.SetFormatter(formatter.JSON())

	parent := sypl.New("with-merge", o)
	parent.SetFields(fields.Fields{"env": envProd, "shared": "parent-value"})

	child := parent.With(fields.Fields{"request_id": "r-1", "shared": "child-value"})

	child.Println(level.Info, "child message")

	decoded := jsonLine(t, buf)

	if decoded["env"] != envProd {
		t.Fatalf("child lost the parent field: env = %v", decoded["env"])
	}

	if decoded["request_id"] != "r-1" {
		t.Fatalf("child lost its own field: request_id = %v", decoded["request_id"])
	}

	if decoded["shared"] != "child-value" {
		t.Fatalf("child's field must win on conflict: shared = %v", decoded["shared"])
	}

	// Parent emits WITHOUT the child's fields.
	buf.Reset()

	parent.Println(level.Info, "parent message")

	decoded = jsonLine(t, buf)

	if _, ok := decoded["request_id"]; ok {
		t.Fatal("parent inherited the child's field: request_id present")
	}

	if decoded["shared"] != "parent-value" {
		t.Fatalf("parent's field mutated: shared = %v", decoded["shared"])
	}
}

// The derived logger shares the OUTPUT INSTANCES, and inherits Name,
// defaultIoWriterLevel, status, error handler, and the fast-gate setting.
func TestWith_InheritanceAndSharedOutputs(t *testing.T) {
	buf, o := output.SafeBuffer(level.Trace)

	parent := sypl.New("with-inherit", o)
	parent.SetDefaultIoWriterLevel(level.Warn)
	parent.SetStatus(status.Disabled)
	parent.SetFastGate(true)

	handlerCalled := false
	parent.SetErrorHandler(func(_ error) { handlerCalled = true })

	child := parent.With(nil)

	if child == parent {
		t.Fatal("With must return a NEW derived logger, not the receiver")
	}

	if child.GetName() != "with-inherit" {
		t.Fatalf("child name = %q, want %q", child.GetName(), "with-inherit")
	}

	if child.GetDefaultIoWriterLevel() != level.Warn {
		t.Fatalf("child defaultIoWriterLevel = %v, want %v", child.GetDefaultIoWriterLevel(), level.Warn)
	}

	if child.GetStatus() != status.Disabled {
		t.Fatalf("child status = %v, want %v", child.GetStatus(), status.Disabled)
	}

	if !child.FastGateEnabled() {
		t.Fatal("child did not inherit the fast-gate setting")
	}

	if h := child.GetErrorHandler(); h == nil {
		t.Fatal("child did not inherit the error handler")
	} else {
		h(nil)

		if !handlerCalled {
			t.Fatal("child's error handler is not the parent's")
		}
	}

	// Output INSTANCES are shared: the child logs into the same buffer.
	childOutputs, parentOutputs := child.GetOutputs(), parent.GetOutputs()

	if len(childOutputs) != 1 || childOutputs[0] != parentOutputs[0] {
		t.Fatal("child does not share the parent's output instances")
	}

	child.Println(level.Info, "via child")

	if !strings.Contains(buf.String(), "via child") {
		t.Fatalf("child write did not reach the shared output: %q", buf.String())
	}
}

// Chained With().With() merges cumulatively - the innermost value wins.
func TestWith_ChainedMerge(t *testing.T) {
	buf, o := output.SafeBuffer(level.Trace)
	o.SetFormatter(formatter.JSON())

	grandchild := sypl.New("with-chain", o).
		With(fields.Fields{"l1": "one", "shared": "first"}).
		With(fields.Fields{"l2": "two", "shared": "second"})

	grandchild.Println(level.Info, "chained")

	decoded := jsonLine(t, buf)

	if decoded["l1"] != "one" || decoded["l2"] != "two" {
		t.Fatalf("chained fields lost: l1=%v l2=%v", decoded["l1"], decoded["l2"])
	}

	if decoded["shared"] != "second" {
		t.Fatalf("innermost With must win: shared = %v", decoded["shared"])
	}
}

// nil, and empty fields are valid: the child simply inherits the parent's
// fields.
func TestWith_NilAndEmptyFields(t *testing.T) {
	buf, o := output.SafeBuffer(level.Trace)
	o.SetFormatter(formatter.JSON())

	parent := sypl.New("with-nil", o)
	parent.SetFields(fields.Fields{"env": envProd})

	for name, f := range map[string]fields.Fields{"nil": nil, "empty": {}} {
		buf.Reset()

		child := parent.With(f)

		child.Println(level.Info, "child message")

		if decoded := jsonLine(t, buf); decoded["env"] != envProd {
			t.Fatalf("%s fields: child lost the parent field: env = %v", name, decoded["env"])
		}
	}
}

// Mutating the child must never leak into the parent - and vice versa. The
// containers are unshared (the 25dfacc audit class), so concurrent
// SetFields/SetTags on both sides must be race-clean.
func TestWith_ParentChildIsolationRace(t *testing.T) {
	_, o := output.SafeBuffer(level.Trace)

	parent := sypl.New("with-race", o)
	parent.SetFields(fields.Fields{"who": "parent"})

	// Three separate appends leave the tags slice with SPARE CAPACITY
	// (len 3, cap 4) - the 25dfacc precondition: were the backing array
	// shared, parent, and child would both write the same spare slot.
	parent.SetTags("t1")
	parent.SetTags("t2")
	parent.SetTags("t3")

	child := parent.With(fields.Fields{"who": whoChild})

	var wg sync.WaitGroup

	wg.Add(4)

	go func() {
		defer wg.Done()

		for i := range 100 {
			parent.SetTags("parent-tag")
			parent.SetFields(fields.Fields{"who": "parent", "i": i})
		}
	}()

	go func() {
		defer wg.Done()

		for i := range 100 {
			child.SetTags("child-tag")
			child.SetFields(fields.Fields{"who": whoChild, "i": i})
		}
	}()

	go func() {
		defer wg.Done()

		for range 100 {
			parent.Println(level.Info, "parent race")
		}
	}()

	go func() {
		defer wg.Done()

		for range 100 {
			child.Println(level.Info, "child race")
		}
	}()

	wg.Wait()

	// Steady-state isolation.
	if parent.GetFields()["who"] != "parent" {
		t.Fatalf("child leaked into parent fields: %v", parent.GetFields())
	}

	if child.GetFields()["who"] != whoChild {
		t.Fatalf("parent leaked into child fields: %v", child.GetFields())
	}

	for _, tag := range parent.GetTags() {
		if tag == "child-tag" {
			t.Fatal("child tags leaked into the parent")
		}
	}
}

// With must never mutate the CALLER's fields argument.
func TestWith_DoesNotAliasArgument(t *testing.T) {
	_, o := output.SafeBuffer(level.Trace)

	parent := sypl.New("with-alias", o)
	parent.SetFields(fields.Fields{"env": envProd})

	arg := fields.Fields{"request_id": "r-9"}

	child := parent.With(arg)

	// Mutating the child's fields must not touch the caller's map.
	child.SetFields(fields.Fields{"replaced": true})
	child.GetFields()["injected"] = true

	if len(arg) != 1 || arg["request_id"] != "r-9" {
		t.Fatalf("With aliased the caller's fields map: %v", arg)
	}
}
