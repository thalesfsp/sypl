// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package message

import "testing"

func Test_generateUUID(t *testing.T) {
	tests := []struct {
		name string
	}{
		{
			name: "Should work",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := generateUUID()

			if len(got) < 30 {
				t.Errorf("generateUUID() = %v", got)
			}
		})
	}
}

func Test_generateID(t *testing.T) {
	type args struct {
		ct string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "Should work",
			args: args{
				ct: "test",
			},
			want: "a94a8fe5ccb19ba61c4c0873d391e987982fbbd3",
		},
		{
			name: "Should work",
			args: args{
				ct: `{"key1":"value1"}`,
			},
			want: "17c9f74584d86d5a852200d274799f5f7d5828da",
		},
		{
			name: "Should work",
			args: args{
				ct: "",
			},
			want: "da39a3ee5e6b4b0d3255bfef95601890afd80709",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got1 := generateID(tt.args.ct)
			got2 := generateID(tt.args.ct)

			if got1 != got2 || got1 != tt.want || got2 != tt.want {
				t.Errorf("generateID() = %v, %v, want %v", got1, got2, tt.want)
			}
		})
	}
}
