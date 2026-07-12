// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package content

import (
	"testing"

	"github.com/thalesfsp/sypl/shared"
)

func TestNew(t *testing.T) {
	type args struct {
		c string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "Should work",
			args: args{
				c: shared.DefaultContentOutput,
			},
			want: shared.DefaultContentOutput,
		},
		{
			name: "Should work - empty string",
			args: args{
				c: "",
			},
			want: "",
		},
		{
			name: "Should work - multiline, and unicode",
			args: args{
				c: "line1\nline2 - áéíóú - 😀",
			},
			want: "line1\nline2 - áéíóú - 😀",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := New(tt.args.c)

			if c == nil {
				t.Fatal("New() = nil, want non-nil")
			}

			if got := c.GetOriginal(); got != tt.want {
				t.Errorf("GetOriginal() = %v, want %v", got, tt.want)
			}

			if got := c.GetProcessed(); got != tt.want {
				t.Errorf("GetProcessed() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSetProcessed(t *testing.T) {
	type args struct {
		initial   string
		processed []string
	}
	tests := []struct {
		name          string
		args          args
		wantOriginal  string
		wantProcessed string
	}{
		{
			name: "Should work - single set",
			args: args{
				initial:   shared.DefaultContentOutput,
				processed: []string{"processed"},
			},
			wantOriginal:  shared.DefaultContentOutput,
			wantProcessed: "processed",
		},
		{
			name: "Should work - multiple sets - last wins",
			args: args{
				initial:   shared.DefaultContentOutput,
				processed: []string{"first", "second", "third"},
			},
			wantOriginal:  shared.DefaultContentOutput,
			wantProcessed: "third",
		},
		{
			name: "Should work - set to empty string",
			args: args{
				initial:   shared.DefaultContentOutput,
				processed: []string{""},
			},
			wantOriginal:  shared.DefaultContentOutput,
			wantProcessed: "",
		},
		{
			name: "Should work - empty original, set processed",
			args: args{
				initial:   "",
				processed: []string{"processed"},
			},
			wantOriginal:  "",
			wantProcessed: "processed",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := New(tt.args.initial)

			for _, p := range tt.args.processed {
				c.SetProcessed(p)
			}

			// Original must never be modified by SetProcessed.
			if got := c.GetOriginal(); got != tt.wantOriginal {
				t.Errorf("GetOriginal() = %v, want %v", got, tt.wantOriginal)
			}

			if got := c.GetProcessed(); got != tt.wantProcessed {
				t.Errorf("GetProcessed() = %v, want %v", got, tt.wantProcessed)
			}
		})
	}
}
