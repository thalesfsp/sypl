// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package sypl_test

import (
	"strings"

	"github.com/thalesfsp/sypl/v2/level"
	"github.com/thalesfsp/sypl/v2/output"
	"github.com/thalesfsp/sypl/v2/processor"
	"github.com/thalesfsp/sypl/v2/safebuffer"
)

// namedSafeBuffer builds a buffer-backed output with the given name - the
// v2 replacement for the removed `SetName`: names are fixed at
// construction.
func namedSafeBuffer(
	name string,
	maxLevel level.Level,
	processors ...processor.IProcessor,
) (*safebuffer.Buffer, output.IOutput) {
	var buf safebuffer.Buffer

	return &buf, output.New(name, maxLevel, &buf, processors...)
}

// findProcessor returns the processor registered under `name` - nil if
// absent. The v2 replacement for the removed `IOutput.GetProcessor`.
func findProcessor(o output.IOutput, name string) processor.IProcessor {
	for _, p := range o.GetProcessors() {
		if strings.EqualFold(p.GetName(), name) {
			return p
		}
	}

	return nil
}
