// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package message

import (
	"regexp"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/thalesfsp/sypl/v2/level"
)

var uuidV4Regex = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// The lazy cell computes exactly once - even under concurrent readers - and
// memoizes.
func TestLazyString_ComputesExactlyOnceConcurrently(t *testing.T) {
	var calls int32

	ls := newLazyString(func() string {
		atomic.AddInt32(&calls, 1)

		return "computed"
	})

	const goroutines = 50

	var wg sync.WaitGroup

	wg.Add(goroutines)

	results := make([]string, goroutines)

	for g := range goroutines {
		go func(g int) {
			defer wg.Done()

			results[g] = ls.get()
		}(g)
	}

	wg.Wait()

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("generator ran %d times under concurrency, want exactly 1", got)
	}

	for g, r := range results {
		if r != "computed" {
			t.Fatalf("goroutine %d observed %q, want %q", g, r, "computed")
		}
	}
}

// A resolved cell never invokes a generator.
func TestLazyString_Resolved(t *testing.T) {
	ls := resolvedLazyString("pinned")

	if got := ls.get(); got != "pinned" {
		t.Fatalf("resolved cell = %q, want %q", got, "pinned")
	}
}

// New must NOT generate the UUID, or the content hash eagerly - only on
// first read - and the values must be stable, and well-formed.
func TestMessageNew_LazyIDGeneration(t *testing.T) {
	m := New(level.Info, "lazy content")

	raw := m.GetMessage()

	if raw.id.isResolved() {
		t.Fatal("New generated the UUID eagerly, want lazy")
	}

	if raw.contentBasedHashID.isResolved() {
		t.Fatal("New generated the content hash eagerly, want lazy")
	}

	// First read materializes; both must be well-formed, and stable.
	id1 := m.GetID()

	if !raw.id.isResolved() {
		t.Fatal("GetID did not memoize the generated UUID")
	}

	if !uuidV4Regex.MatchString(id1) {
		t.Fatalf("GetID() = %q, want a well-formed UUIDv4", id1)
	}

	if id2 := m.GetID(); id2 != id1 {
		t.Fatalf("GetID() unstable: %q then %q", id1, id2)
	}

	hash1 := m.GetContentBasedHashID()

	if want := generateID("lazy content"); hash1 != want {
		t.Fatalf("GetContentBasedHashID() = %q, want %q", hash1, want)
	}

	if hash2 := m.GetContentBasedHashID(); hash2 != hash1 {
		t.Fatalf("GetContentBasedHashID() unstable: %q then %q", hash1, hash2)
	}

	// Distinct messages must still get distinct UUIDs.
	if other := New(level.Info, "lazy content"); other.GetID() == id1 {
		t.Fatal("two messages generated the same UUID")
	}
}

// SetID/SetContentBasedHashID must pin the value, preventing generation -
// and must override an already-generated value.
func TestMessageSetID_PreventsGeneration(t *testing.T) {
	m := New(level.Info, "pinned content")

	m.SetID("custom-id")
	m.SetContentBasedHashID("custom-hash")

	if got := m.GetID(); got != "custom-id" {
		t.Fatalf("GetID() = %q, want %q", got, "custom-id")
	}

	if got := m.GetContentBasedHashID(); got != "custom-hash" {
		t.Fatalf("GetContentBasedHashID() = %q, want %q", got, "custom-hash")
	}

	// Overriding a generated value.
	g := New(level.Info, "generated first")

	_ = g.GetID()

	g.SetID("override")

	if got := g.GetID(); got != "override" {
		t.Fatalf("GetID() after SetID = %q, want %q", got, "override")
	}
}

// Copy must preserve the source identity WITHOUT forcing generation: source,
// and copy share the lazy cells, so whoever reads first resolves the same
// value for the whole family.
func TestMessageCopy_SharesLazyIdentity(t *testing.T) {
	m := New(level.Info, "copied content")

	c := Copy(m)

	rawM, rawC := m.GetMessage(), c.GetMessage()

	if rawM.id.isResolved() || rawC.id.isResolved() {
		t.Fatal("Copy forced UUID generation")
	}

	if rawC.id != rawM.id {
		t.Fatal("Copy did not share the lazy UUID cell with the source")
	}

	if rawC.contentBasedHashID != rawM.contentBasedHashID {
		t.Fatal("Copy did not share the lazy content-hash cell with the source")
	}

	// Reading via the COPY resolves the same identity for the source.
	if c.GetID() != m.GetID() {
		t.Fatalf("copy ID %q != source ID %q", c.GetID(), m.GetID())
	}

	if c.GetContentBasedHashID() != m.GetContentBasedHashID() {
		t.Fatal("copy content hash != source content hash")
	}

	// A copy of an ALREADY-resolved message preserves the value.
	resolved := New(level.Info, "resolved content")
	id := resolved.GetID()

	if got := Copy(resolved).GetID(); got != id {
		t.Fatalf("copy of resolved message ID = %q, want %q", got, id)
	}

	// SetID on a copy must NOT leak into the source (snapshot semantics).
	c2 := Copy(resolved)
	c2.SetID("copy-only")

	if resolved.GetID() != id {
		t.Fatalf("SetID on a copy mutated the source: %q", resolved.GetID())
	}
}

// Concurrent GetID across a message family (source + N copies, as the
// per-output fan-out does) must resolve exactly one shared identity. Run
// under -race.
func TestMessageLazyID_ConcurrentFamilyReads(t *testing.T) {
	m := New(level.Info, "family content")

	const copies = 16

	family := make([]IMessage, 0, copies+1)
	family = append(family, m)

	for range copies {
		family = append(family, Copy(m))
	}

	ids := make([]string, len(family))
	hashes := make([]string, len(family))

	var wg sync.WaitGroup

	wg.Add(len(family))

	for i, member := range family {
		go func(i int, member IMessage) {
			defer wg.Done()

			ids[i] = member.GetID()
			hashes[i] = member.GetContentBasedHashID()
		}(i, member)
	}

	wg.Wait()

	for i := 1; i < len(family); i++ {
		if ids[i] != ids[0] {
			t.Fatalf("family member %d ID = %q, want %q", i, ids[i], ids[0])
		}

		if hashes[i] != hashes[0] {
			t.Fatalf("family member %d hash = %q, want %q", i, hashes[i], hashes[0])
		}
	}

	if !uuidV4Regex.MatchString(ids[0]) {
		t.Fatalf("family ID %q is not a well-formed UUIDv4", ids[0])
	}
}
