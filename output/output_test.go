// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package output

import (
	"bufio"
	"bytes"
	"testing"

	"github.com/thalesfsp/sypl/internal/builtin"
	"github.com/thalesfsp/sypl/level"
	"github.com/thalesfsp/sypl/message"
	"github.com/thalesfsp/sypl/processor"
	"github.com/thalesfsp/sypl/shared"
	"github.com/thalesfsp/sypl/status"
)

func TestNewOutput(t *testing.T) {
	type args struct {
		maxLevel level.Level
		name     string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "Should work",
			args: args{
				name:     "Buffer",
				maxLevel: level.Trace,
			},
			want: shared.DefaultPrefixValue + shared.DefaultContentOutput,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			bufWriter := bufio.NewWriter(&buf)

			output := New(tt.args.name, tt.args.maxLevel, bufWriter, processor.Prefixer(shared.DefaultPrefixValue))

			message := message.New(level.Info, shared.DefaultContentOutput)

			if message.GetComponentName() != "" &&
				message.GetOutputName() != "" &&
				message.GetProcessorName() != "" {
				t.Error("Message should not have sypl, output, and processor")
			}

			for _, processor := range output.GetProcessors() {
				_ = processor.Run(message)
			}

			if err := output.GetBuiltinLogger().OutputBuiltin(
				builtin.DefaultCallDepth,
				message.GetContent().GetProcessed(),
			); err != nil {
				t.Errorf("Failed to log to output: %+v", err)
			}

			bufWriter.Flush()

			if buf.String() != tt.want {
				t.Errorf("Got %v, want %v", buf.String(), tt.want)
			}
		})
	}
}

func TestOutput_GetStatus(t *testing.T) {
	type args struct {
		maxLevel level.Level
		name     string
	}
	tests := []struct {
		name string
		args args
		want status.Status
	}{
		{
			name: "Should work",
			args: args{
				name:     "Buffer",
				maxLevel: level.Trace,
			},
			want: status.Disabled,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := Console(level.Trace)
			output.SetStatus(status.Disabled)

			if output.GetStatus() != tt.want {
				t.Errorf("Got %v, want %v", output.GetStatus(), tt.want)
			}
		})
	}
}
