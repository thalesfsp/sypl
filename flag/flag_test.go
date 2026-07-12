// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package flag

import "testing"

func TestFlag_String(t *testing.T) {
	tests := []struct {
		name string
		f    Flag
		want string
	}{
		{
			name: "Should work - None",
			f:    None,
			want: "None",
		},
		{
			name: "Should work - Force",
			f:    Force,
			want: "Force",
		},
		{
			name: "Should work - Mute",
			f:    Mute,
			want: "Mute",
		},
		{
			name: "Should work - Skip",
			f:    Skip,
			want: "Skip",
		},
		{
			name: "Should work - SkipAndForce",
			f:    SkipAndForce,
			want: "SkipAndForce",
		},
		{
			name: "Should work - SkipAndMute",
			f:    SkipAndMute,
			want: "SkipAndMute",
		},
		{
			name: "Should work - out-of-range - negative",
			f:    Flag(-1),
			want: "Unknown",
		},
		{
			name: "Should work - out-of-range - SkipAndMute + 1",
			f:    SkipAndMute + 1,
			want: "Unknown",
		},
		{
			name: "Should work - out-of-range - big positive",
			f:    Flag(100),
			want: "Unknown",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.f.String(); got != tt.want {
				t.Errorf("Flag.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Guards the Flag values themselves: consumers persist/compare these as ints,
// so a reordering of the const block is a breaking change.
func TestFlag_Values(t *testing.T) {
	tests := []struct {
		name string
		f    Flag
		want int
	}{
		{name: "None", f: None, want: 0},
		{name: "Force", f: Force, want: 1},
		{name: "Mute", f: Mute, want: 2},
		{name: "Skip", f: Skip, want: 3},
		{name: "SkipAndForce", f: SkipAndForce, want: 4},
		{name: "SkipAndMute", f: SkipAndMute, want: 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if int(tt.f) != tt.want {
				t.Errorf("int(%s) = %d, want %d", tt.name, int(tt.f), tt.want)
			}
		})
	}
}
