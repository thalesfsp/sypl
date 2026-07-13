// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package builtin

import (
	"io"
	"sync"
)

// A Builtin represents an active logging object that writes lines of
// output to an io.Writer. Each logging operation makes a single call to
// the Writer's Write method. A Builtin can be used simultaneously from
// multiple goroutines; it guarantees to serialize access to the Writer.
//
// NOTE: It does NOT force-append a newline - the reason this package
// exists (see doc.go).
type Builtin struct {
	mu  sync.Mutex // ensures atomic writes; protects the following fields
	out io.Writer  // destination for output
	buf []byte     // for accumulating text to write
}

// NewBuiltin creates a new Builtin. The out variable sets the
// destination to which log data will be written.
func NewBuiltin(out io.Writer) *Builtin {
	return &Builtin{out: out}
}

// SetOutput sets the output destination for the logger.
func (l *Builtin) SetOutput(w io.Writer) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.out = w
}

// OutputBuiltin writes the output for a logging event: exactly `s`, no
// forced newline, no padding.
//
// NOTE: The internal write buffer is reused across calls - per the
// io.Writer contract, the destination must not retain the passed slice
// past Write.
func (l *Builtin) OutputBuiltin(s string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.buf = append(l.buf[:0], s...)

	_, err := l.out.Write(l.buf)

	return err
}
