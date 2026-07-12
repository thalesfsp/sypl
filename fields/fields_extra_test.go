// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package fields

import (
	"reflect"
	"testing"
)

// Copy with a nil dst must allocate a NEW map - not alias src.
func TestCopy_NilDstAllocatesNewMap(t *testing.T) {
	src := Fields{"a": 1, "b": 2}

	got := Copy(src, nil)

	if got == nil {
		t.Fatal("Copy(src, nil) = nil, want allocated Fields")
	}

	if !reflect.DeepEqual(got, src) {
		t.Fatalf("Copy(src, nil) = %v, want %v", got, src)
	}

	// Mutating the returned map must not leak into src.
	got["c"] = 3

	if _, ok := src["c"]; ok {
		t.Error("Copy(src, nil) aliased src: mutation of result leaked into src")
	}
}

// Copy with a non-nil dst must mutate dst in place, and return it.
func TestCopy_NonNilDstMutatedInPlace(t *testing.T) {
	src := Fields{"a": 1}
	dst := Fields{"b": 2}

	got := Copy(src, dst)

	// dst itself must have received the src entries.
	want := Fields{"a": 1, "b": 2}

	if !reflect.DeepEqual(dst, want) {
		t.Errorf("dst after Copy = %v, want %v", dst, want)
	}

	// The returned map must BE dst, not a copy of it.
	if reflect.ValueOf(got).Pointer() != reflect.ValueOf(dst).Pointer() {
		t.Error("Copy(src, dst) returned a different map than dst")
	}
}

// Overlapping keys: src takes precedence, non-overlapping dst keys survive.
func TestCopy_OverlappingKeysPrecedence(t *testing.T) {
	type args struct {
		src Fields
		dst Fields
	}
	tests := []struct {
		name string
		args args
		want Fields
	}{
		{
			name: "Should work - src wins on overlap",
			args: args{
				src: Fields{"shared": "fromSrc", "srcOnly": 1},
				dst: Fields{"shared": "fromDst", "dstOnly": 2},
			},
			want: Fields{"shared": "fromSrc", "srcOnly": 1, "dstOnly": 2},
		},
		{
			name: "Should work - src nil value overrides dst value",
			args: args{
				src: Fields{"shared": nil},
				dst: Fields{"shared": "fromDst"},
			},
			want: Fields{"shared": nil},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Copy(tt.args.src, tt.args.dst); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Copy() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Empty (but non-nil) src is not the same as nil src: it must still
// allocate/return, just without adding entries.
func TestCopy_EmptySrc(t *testing.T) {
	t.Run("empty src - nil dst - allocates empty map", func(t *testing.T) {
		got := Copy(Fields{}, nil)

		if got == nil {
			t.Fatal("Copy(Fields{}, nil) = nil, want allocated empty Fields")
		}

		if len(got) != 0 {
			t.Errorf("Copy(Fields{}, nil) len = %d, want 0", len(got))
		}
	})

	t.Run("empty src - populated dst - dst unchanged", func(t *testing.T) {
		dst := Fields{"keep": true}

		got := Copy(Fields{}, dst)

		want := Fields{"keep": true}

		if !reflect.DeepEqual(got, want) {
			t.Errorf("Copy(Fields{}, dst) = %v, want %v", got, want)
		}
	})
}

// nil src returns nil regardless of dst, and must not touch dst.
func TestCopy_NilSrcLeavesDstUntouched(t *testing.T) {
	dst := Fields{"keep": true}

	got := Copy(nil, dst)

	if got != nil {
		t.Errorf("Copy(nil, dst) = %v, want nil", got)
	}

	want := Fields{"keep": true}

	if !reflect.DeepEqual(dst, want) {
		t.Errorf("dst after Copy(nil, dst) = %v, want %v", dst, want)
	}
}

// src must never be modified by Copy.
func TestCopy_SrcNotMutated(t *testing.T) {
	src := Fields{"a": 1}
	dst := Fields{"b": 2}

	Copy(src, dst)

	want := Fields{"a": 1}

	if !reflect.DeepEqual(src, want) {
		t.Errorf("src after Copy = %v, want %v", src, want)
	}
}
