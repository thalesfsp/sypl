// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package message

import (
	"crypto/sha1"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/thalesfsp/sypl/v2/shared"
)

// lazyString memoizes a string computed on first `get` - thread-safe, the
// message is used across output goroutines. Copies of a message SHARE the
// cell (see `Copy`), so the value is computed at most once per message
// family, and every member observes the same value.
type lazyString struct {
	mu  sync.Mutex
	gen func() string
	set bool
	val string
}

// get returns the memoized value, computing it on first call.
func (ls *lazyString) get() string {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	if !ls.set {
		ls.val = ls.gen()
		ls.gen = nil
		ls.set = true
	}

	return ls.val
}

// isResolved returns whether the value has already been computed, or pinned.
func (ls *lazyString) isResolved() bool {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	return ls.set
}

// newLazyString is the lazyString factory: `gen` runs on first `get`.
func newLazyString(gen func() string) *lazyString {
	return &lazyString{gen: gen}
}

// resolvedLazyString pins the cell to `v` - no generation will ever run.
func resolvedLazyString(v string) *lazyString {
	return &lazyString{set: true, val: v}
}

// generateUUID generates UUIDv4 for message ID.
func generateUUID() string {
	id, err := uuid.NewRandom()
	if err != nil {
		log.Println(shared.ErrorPrefix, "generateUUID: Failed to generate UUID for message", err)
	}

	return id.String()
}

// generateID generates MD5 hash (content-based) for message ID. Good to be used
// to avoid duplicated messages.
func generateID(ct string) string {
	return fmt.Sprintf("%x", sha1.Sum([]byte(strings.Trim(ct, "\f\t\r\n "))))
}
