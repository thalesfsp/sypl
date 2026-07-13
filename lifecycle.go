// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package sypl

import (
	"errors"
	"io"
)

//////
// Flush/Close contract.
//
// Outputs are not required to buffer - so the capability is detected by
// type-asserting each registered output against the SMALL interfaces
// `interface{ Flush() error }`, and `io.Closer`. Outputs lacking the
// capability are skipped. Calls happen in registration order, and ALL errors
// are aggregated via `errors.Join` - one failing output never shadows its
// siblings.
//////

// Flush flushes every registered output implementing
// `interface{ Flush() error }`, in registration order, aggregating all
// errors via `errors.Join`. Outputs lacking the capability are skipped.
//
// NOTE: The outputs are snapshotted under the read lock, which is released
// BEFORE any Flush call.
func (sypl *Sypl) Flush() error {
	outputs := sypl.GetOutputs()

	errs := make([]error, 0, len(outputs))

	for _, o := range outputs {
		if f, ok := o.(interface{ Flush() error }); ok {
			errs = append(errs, f.Flush())
		}
	}

	return errors.Join(errs...)
}

// Close closes every registered output implementing `io.Closer`, in
// registration order, aggregating all errors via `errors.Join`. Outputs
// lacking the capability are skipped.
//
// NOTE: The outputs are snapshotted under the read lock, which is released
// BEFORE any Close call.
func (sypl *Sypl) Close() error {
	outputs := sypl.GetOutputs()

	errs := make([]error, 0, len(outputs))

	for _, o := range outputs {
		if c, ok := o.(io.Closer); ok {
			errs = append(errs, c.Close())
		}
	}

	return errors.Join(errs...)
}

//////
// Error handler.
//
// `processOutputs` historically discarded output write errors. When a handler
// is set, each non-nil error is delivered to it, wrapped with the failing
// output's name ("output <name>: <err>"). The handler:
//   - is NEVER invoked holding sypl's mutex - it may safely reconfigure the
//     logger;
//   - may be invoked concurrently (multi-output fan-out) - it must be
//     concurrency-safe;
//   - when nil (the default), behavior is unchanged: errors are silently
//     swallowed.
//////

// SetErrorHandler sets the handler invoked with each output write error. A
// nil handler restores the default behavior (errors silently swallowed).
func (sypl *Sypl) SetErrorHandler(h func(err error)) *Sypl {
	sypl.lock()
	defer sypl.unlock()

	sypl.errorHandler = h

	return sypl
}

// GetErrorHandler returns the registered error handler - nil if none.
func (sypl *Sypl) GetErrorHandler() func(err error) {
	sypl.rLock()
	defer sypl.rUnlock()

	return sypl.errorHandler
}
