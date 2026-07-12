// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package sypl

import (
	"reflect"
	"testing"

	"github.com/thalesfsp/sypl/fields"
	"github.com/thalesfsp/sypl/flag"
	"github.com/thalesfsp/sypl/level"
	"github.com/thalesfsp/sypl/message"
	"github.com/thalesfsp/sypl/options"
)

// mergeOptions with everything set must override fields, flag, outputs
// names, and processors names, and merge tags.
func TestMergeOptions_AllSet(t *testing.T) {
	m := message.New(level.Info, "content")

	o := &options.Options{
		Fields:          fields.Fields{"k": "v"},
		Flag:            flag.Force,
		OutputsNames:    []string{"o1"},
		ProcessorsNames: []string{"p1"},
		Tags:            []string{"t1"},
	}

	got := mergeOptions(m, o)

	if !reflect.DeepEqual(got.GetFields(), fields.Fields{"k": "v"}) {
		t.Fatalf("fields = %v, expected map[k:v]", got.GetFields())
	}

	if got.GetFlag() != flag.Force {
		t.Fatalf("flag = %v, expected Force", got.GetFlag())
	}

	if !reflect.DeepEqual(got.GetOutputsNames(), []string{"o1"}) {
		t.Fatalf("outputs names = %v, expected [o1]", got.GetOutputsNames())
	}

	if !reflect.DeepEqual(got.GetProcessorsNames(), []string{"p1"}) {
		t.Fatalf("processors names = %v, expected [p1]", got.GetProcessorsNames())
	}

	// Tags land both in `options.Tags` and in the message's tags.
	if !reflect.DeepEqual(got.GetMessage().Tags, []string{"t1"}) {
		t.Fatalf("options tags = %v, expected [t1]", got.GetMessage().Tags)
	}

	if !got.ContainTag("t1") {
		t.Fatalf("message tags = %v, expected to contain t1", got.GetTags())
	}
}

// mergeOptions with a zero-value Options (nil fields, None flag, empty
// names/tags) must leave the message untouched.
func TestMergeOptions_ZeroValueLeavesMessageUntouched(t *testing.T) {
	m := message.New(level.Info, "content")
	m.SetFields(fields.Fields{"orig": "1"})
	m.SetFlag(flag.Skip)
	m.SetOutputsNames([]string{"keep-out"})
	m.SetProcessorsNames([]string{"keep-proc"})
	m.AddTags("keep-tag")

	got := mergeOptions(m, &options.Options{})

	if !reflect.DeepEqual(got.GetFields(), fields.Fields{"orig": "1"}) {
		t.Fatalf("nil options fields overwrote message fields: %v", got.GetFields())
	}

	if got.GetFlag() != flag.Skip {
		t.Fatalf("None options flag overwrote message flag: %v", got.GetFlag())
	}

	if !reflect.DeepEqual(got.GetOutputsNames(), []string{"keep-out"}) {
		t.Fatalf("empty options outputs names overwrote message ones: %v", got.GetOutputsNames())
	}

	if !reflect.DeepEqual(got.GetProcessorsNames(), []string{"keep-proc"}) {
		t.Fatalf("empty options processors names overwrote message ones: %v", got.GetProcessorsNames())
	}

	if !reflect.DeepEqual(got.GetTags(), []string{"keep-tag"}) {
		t.Fatalf("empty options tags changed message tags: %v", got.GetTags())
	}
}
