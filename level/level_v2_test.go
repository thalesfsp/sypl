// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package level

import "testing"

// V2 BREAKING CHANGE: conventional level ordering. Warn, and Info swapped
// places so verbosity nests conventionally:
// None(0) Fatal(1) Error(2) Warn(3) Info(4) Debug(5) Trace(6).
func TestV2Ordering_NumericValues(t *testing.T) {
	tests := []struct {
		level Level
		want  int
	}{
		{None, 0},
		{Fatal, 1},
		{Error, 2},
		{Warn, 3},
		{Info, 4},
		{Debug, 5},
		{Trace, 6},
	}
	for _, tt := range tests {
		if int(tt.level) != tt.want {
			t.Errorf("int(%s) = %d, want %d", tt.level, int(tt.level), tt.want)
		}
	}
}

// FromInt follows the v2 numeric order.
func TestV2Ordering_FromInt(t *testing.T) {
	if FromInt(3) != Warn {
		t.Errorf("FromInt(3) = %s, want warn", FromInt(3))
	}

	if FromInt(4) != Info {
		t.Errorf("FromInt(4) = %s, want info", FromInt(4))
	}
}

// The String/FromString round-trip is UNAFFECTED by the reordering: names
// still map to themselves.
func TestV2Ordering_StringRoundTripUnaffected(t *testing.T) {
	for _, name := range []string{"none", "fatal", "error", "warn", "info", "debug", "trace"} {
		l, err := FromString(name)
		if err != nil {
			t.Fatalf("FromString(%q) error = %v, want nil", name, err)
		}

		if got := l.String(); got != name {
			t.Errorf("FromString(%q).String() = %q - round-trip broken", name, got)
		}
	}
}

// LevelsNames follows the v2 order: warn before info.
func TestV2Ordering_LevelsNames(t *testing.T) {
	want := []string{"none", "fatal", "error", "warn", "info", "debug", "trace"}

	got := LevelsNames()

	if len(got) != len(want) {
		t.Fatalf("LevelsNames() = %v, want %v", got, want)
	}

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("LevelsNames()[%d] = %q, want %q (%v)", i, got[i], want[i], got)
		}
	}
}
