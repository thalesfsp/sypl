// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package sypl

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
