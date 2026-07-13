// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package message

import (
	"testing"

	"github.com/thalesfsp/sypl/fields"
	"github.com/thalesfsp/sypl/level"
)

// BenchmarkMessageNew measures the message factory - the very first step of
// every Print-family call.
func BenchmarkMessageNew(b *testing.B) {
	b.ReportAllocs()

	for range b.N {
		_ = New(level.Info, "benchmark message")
	}
}

// BenchmarkMessageCopy measures the per-output message isolation copy.
func BenchmarkMessageCopy(b *testing.B) {
	m := New(level.Info, "benchmark message")

	m.SetFields(fields.Fields{"key1": "value1", "key2": 2})
	m.AddTags("tag1", "tag2")

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		_ = Copy(m)
	}
}
