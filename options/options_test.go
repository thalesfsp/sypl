// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package options

import (
	"testing"

	"github.com/thalesfsp/sypl/v2/flag"
)

const testValue = "value"

func TestNew(t *testing.T) {
	o := New()

	if o == nil {
		t.Fatal("New() = nil, want non-nil")
	}

	if o.Flag != flag.None {
		t.Errorf("New().Flag = %v, want %v", o.Flag, flag.None)
	}

	if o.Fields == nil {
		t.Error("New().Fields = nil, want initialized map")
	}

	if len(o.Fields) != 0 {
		t.Errorf("New().Fields len = %d, want 0", len(o.Fields))
	}

	slices := []struct {
		name  string
		value []string
	}{
		{"OutputsNames", o.OutputsNames},
		{"ProcessorsNames", o.ProcessorsNames},
		{"Tags", o.Tags},
	}

	for _, s := range slices {
		if s.value == nil {
			t.Errorf("New().%s = nil, want initialized slice", s.name)
		}

		if len(s.value) != 0 {
			t.Errorf("New().%s len = %d, want 0", s.name, len(s.value))
		}
	}
}

// The Fields map must be usable without any further initialization.
func TestNew_FieldsWritable(t *testing.T) {
	o := New()

	o.Fields["key"] = testValue

	if o.Fields["key"] != testValue {
		t.Errorf(`New().Fields["key"] = %v, want %q`, o.Fields["key"], testValue)
	}
}

// Each call must return an independent instance - no shared state.
func TestNew_IndependentInstances(t *testing.T) {
	o1 := New()
	o2 := New()

	if o1 == o2 {
		t.Fatal("New() returned the same pointer twice, want distinct instances")
	}

	o1.Fields["key"] = testValue

	o1.OutputsNames = append(o1.OutputsNames, "output")

	o1.ProcessorsNames = append(o1.ProcessorsNames, "processor")

	o1.Tags = append(o1.Tags, "tag")

	o1.Flag = flag.Force

	if len(o2.Fields) != 0 {
		t.Errorf("Mutating o1.Fields leaked into o2.Fields: %v", o2.Fields)
	}

	if len(o2.OutputsNames) != 0 {
		t.Errorf("Mutating o1.OutputsNames leaked into o2.OutputsNames: %v", o2.OutputsNames)
	}

	if len(o2.ProcessorsNames) != 0 {
		t.Errorf("Mutating o1.ProcessorsNames leaked into o2.ProcessorsNames: %v", o2.ProcessorsNames)
	}

	if len(o2.Tags) != 0 {
		t.Errorf("Mutating o1.Tags leaked into o2.Tags: %v", o2.Tags)
	}

	if o2.Flag != flag.None {
		t.Errorf("Mutating o1.Flag leaked into o2.Flag: %v", o2.Flag)
	}
}
