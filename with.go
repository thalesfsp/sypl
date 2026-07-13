// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package sypl

import (
	"slices"

	"github.com/thalesfsp/sypl/fields"
)

// With returns a DERIVED logger: it shares the parent's output INSTANCES,
// but owns its mutex, and its own COPIES of the fields map - the parent's
// fields merged with `f`, `f` winning on key conflict - and of the tags
// slice. Reconfiguring the child's fields/tags never leaks into the parent,
// and vice versa (the containers are unshared - see the 2026-07-12 audit fix,
// commit 25dfacc).
//
// The derived logger inherits Name, the default io.Writer level, status, the
// error handler, the context extractor, and the fast-gate setting. `f` may
// be nil, or empty - the child then simply inherits the parent's fields.
func (sypl *Sypl) With(f fields.Fields) *Sypl {
	sypl.rLock()
	defer sypl.rUnlock()

	// Merged into a FRESH map: never aliases the parent's map, nor the
	// caller's argument.
	merged := make(fields.Fields, len(sypl.fields)+len(f))

	for k, v := range sypl.fields {
		merged[k] = v
	}

	for k, v := range f {
		merged[k] = v
	}

	// NOTE: The outputs slice CONTAINER is cloned by the factory; the output
	// ELEMENTS stay shared by design.
	s := New(sypl.Name, sypl.outputs...)

	s.contextExtractor = sypl.contextExtractor
	s.defaultIoWriterLevel = sypl.defaultIoWriterLevel
	s.errorHandler = sypl.errorHandler
	s.fastGate = sypl.fastGate
	s.fields = merged
	s.status = sypl.status
	s.tags = slices.Clone(sypl.tags)

	return s
}
