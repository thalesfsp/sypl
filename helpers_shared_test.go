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

const (
	fixtureComponent          = "component"
	fixtureConsole1Title      = "Console 1"
	fixtureContentBasedHashID = "contentBasedHashID"
	fixtureEnv                = "env"
	fixtureFkey               = "fkey"
	fixtureForcedLine         = "forced\n"
	fixtureFval               = "fval"
	fixtureInfo               = "info"
	fixtureLevel              = "level"
	fixtureLnf                = "lnf"
	fixtureM1                 = "m-1"
	fixtureM1Line             = "m-1\n"
	fixtureMessage            = "message"
	fixtureMsg                = "msg"
	fixtureMsgDebug           = "msg-debug"
	fixtureMsgError           = "msg-error"
	fixtureMsgInfo            = "msg-info"
	fixtureMsgLine            = "msg\n"
	fixtureMsgTrace           = "msg-trace"
	fixtureMsgWarn            = "msg-warn"
	fixtureParent             = "parent"
	fixturePlain              = "plain"
	fixtureRequestId          = "request_id"
	fixtureShared             = "shared"
	fixtureSvc                = "svc"
	fixtureSvcApi             = "svc,api"
	fixtureTraceId            = "trace_id"
	fixtureValue1             = "value1"
	fixtureWarn               = "warn"
	fixtureWho                = "who"
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
