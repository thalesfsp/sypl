// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package output

import (
	"fmt"
	"io"
	"reflect"
	"testing"

	"github.com/thalesfsp/sypl/level"
	"github.com/thalesfsp/sypl/message"
	"github.com/thalesfsp/sypl/processor"
)

// An output VALUE must satisfy fmt.Stringer, as on master. The mutex is
// held by pointer, so copies are vet-copylocks clean.
var _ fmt.Stringer = output{}

// The factory must not alias the caller's processors slice: two outputs
// built from the same spare-capacity slice would otherwise share the backing
// array, and a later AddProcessors on one output would overwrite the
// other's spare-capacity slot.
func TestAudit_FactoryProcessorsSliceNotAliased(t *testing.T) {
	noop := func(m message.IMessage) error { return nil }

	// Spare capacity is the trigger.
	procs := make([]processor.IProcessor, 0, 8)
	procs = append(procs, processor.New("Shared", noop))

	o1 := New("o1", level.Trace, io.Discard, procs...)
	o2 := New("o2", level.Trace, io.Discard, procs...)

	o1.AddProcessors(processor.New("A", noop))
	o2.AddProcessors(processor.New("B", noop))

	if got := o1.GetProcessorsNames(); !reflect.DeepEqual(got, []string{"Shared", "A"}) {
		t.Fatalf("output 1 processors = %v, expected [Shared A]", got)
	}

	if got := o2.GetProcessorsNames(); !reflect.DeepEqual(got, []string{"Shared", "B"}) {
		t.Fatalf("output 2 processors = %v, expected [Shared B]", got)
	}

	// The caller's spare capacity must be untouched.
	for i, spare := range procs[1:cap(procs)] {
		if spare != nil {
			t.Fatalf("caller's backing array slot %d was written to: %v", i+1, spare)
		}
	}
}
