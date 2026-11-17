// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package sypl_test

import (
	"strings"
	"testing"

	"github.com/thalesfsp/sypl/v2"
	"github.com/thalesfsp/sypl/v2/level"
	"github.com/thalesfsp/sypl/v2/output"
)

// V2 BREAKING CHANGE: `SetMaxLevel(Info)` now SHOWS warnings - Warn(3)
// nests below Info(4) in the conventional order.
func TestV2Visibility_MaxLevelInfoShowsWarn(t *testing.T) {
	buf, o := output.SafeBuffer(level.Trace)

	l := sypl.New("v2-visibility", o)

	l.SetMaxLevel(level.Info)

	l.Warnln("warn-visible")

	if !strings.Contains(buf.String(), "warn-visible") {
		t.Fatalf("SetMaxLevel(Info) must show Warn in v2, got: %q", buf.String())
	}
}

// The counterpart nesting: an output capped at Warn no longer shows Info.
func TestV2Visibility_MaxLevelWarnHidesInfo(t *testing.T) {
	buf, o := output.SafeBuffer(level.Trace)

	l := sypl.New("v2-visibility", o)

	l.SetMaxLevel(level.Warn)

	l.Infoln("info-hidden")

	if strings.Contains(buf.String(), "info-hidden") {
		t.Fatalf("SetMaxLevel(Warn) must hide Info in v2, got: %q", buf.String())
	}
}

// Full nesting contract at each cap: everything at, or below the cap is
// visible; everything above is not.
func TestV2Visibility_NestingTable(t *testing.T) {
	emit := func(l *sypl.Sypl) {
		l.Errorln("msg-error")
		l.Warnln("msg-warn")
		l.Infoln("msg-info")
		l.Debugln("msg-debug")
		l.Traceln("msg-trace")
	}

	tests := []struct {
		cap     level.Level
		visible []string
		hidden  []string
	}{
		{
			cap:     level.Error,
			visible: []string{fixtureMsgError},
			hidden:  []string{fixtureMsgWarn, fixtureMsgInfo, fixtureMsgDebug, fixtureMsgTrace},
		},
		{
			cap:     level.Warn,
			visible: []string{fixtureMsgError, fixtureMsgWarn},
			hidden:  []string{fixtureMsgInfo, fixtureMsgDebug, fixtureMsgTrace},
		},
		{
			cap:     level.Info,
			visible: []string{fixtureMsgError, fixtureMsgWarn, fixtureMsgInfo},
			hidden:  []string{fixtureMsgDebug, fixtureMsgTrace},
		},
		{
			cap:     level.Debug,
			visible: []string{fixtureMsgError, fixtureMsgWarn, fixtureMsgInfo, fixtureMsgDebug},
			hidden:  []string{fixtureMsgTrace},
		},
		{
			cap:     level.Trace,
			visible: []string{fixtureMsgError, fixtureMsgWarn, fixtureMsgInfo, fixtureMsgDebug, fixtureMsgTrace},
			hidden:  nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.cap.String(), func(t *testing.T) {
			buf, o := output.SafeBuffer(tt.cap)

			l := sypl.New("v2-nesting", o)

			emit(l)

			for _, want := range tt.visible {
				if !strings.Contains(buf.String(), want) {
					t.Errorf("cap %s: %q must be visible, got: %q", tt.cap, want, buf.String())
				}
			}

			for _, unwanted := range tt.hidden {
				if strings.Contains(buf.String(), unwanted) {
					t.Errorf("cap %s: %q must be hidden, got: %q", tt.cap, unwanted, buf.String())
				}
			}
		})
	}
}
