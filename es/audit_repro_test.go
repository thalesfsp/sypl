// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package es

import "testing"

// The ES output must return an error, not panic, when the document "id" is
// not a string. The panic fires before any network call.
func TestAudit_ESWriteNonStringIDNoPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("ES Write panicked on non-string id: %v", r)
		}
	}()

	es := &ElasticSearch{
		DynamicIndex: func() string { return "idx" },
	}

	// Client is nil: the call may error, but it must not panic on the
	// unchecked type assertion.
	//nolint:errcheck
	es.Write([]byte(`{"id":123}`))
}
