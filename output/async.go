// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package output

import (
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/thalesfsp/sypl/message"
)

//////
// Consts, vars, and types.
//////

// defaultAsyncBufferSize is the default async buffer capacity.
const defaultAsyncBufferSize = 1024

var (
	// ErrAsyncClosed is returned when writing to a closed async output.
	ErrAsyncClosed = errors.New("async output is closed")

	// ErrAsyncDropped is the sentinel wrapped into drop notifications
	// delivered to the error handler. Check it with `errors.Is`.
	ErrAsyncDropped = errors.New("async output dropped a message")
)

// AsyncPolicy determines what happens when a message is written to an async
// output whose buffer is full.
type AsyncPolicy int

// Available policies.
const (
	// AsyncPolicyBlock blocks the writer until buffer space is available.
	// It's the default policy - no message is ever dropped.
	AsyncPolicyBlock AsyncPolicy = iota

	// AsyncPolicyDropNewest drops the incoming message.
	AsyncPolicyDropNewest

	// AsyncPolicyDropOldest drops the oldest buffered message, making room
	// for the incoming one.
	AsyncPolicyDropOldest
)

// String interface implementation.
func (p AsyncPolicy) String() string {
	switch p {
	case AsyncPolicyBlock:
		return "Block"
	case AsyncPolicyDropNewest:
		return "DropNewest"
	case AsyncPolicyDropOldest:
		return "DropOldest"
	default:
		return "Unknown"
	}
}

// AsyncOption configures the async output wrapper.
type AsyncOption func(*asyncOutput)

// AsyncWithBufferSize sets the buffer capacity. Non-positive values fall
// back to the default (1024).
func AsyncWithBufferSize(size int) AsyncOption {
	return func(a *asyncOutput) {
		if size > 0 {
			a.capacity = size
		}
	}
}

// AsyncWithPolicy sets the full-buffer policy. Default: AsyncPolicyBlock.
func AsyncWithPolicy(policy AsyncPolicy) AsyncOption {
	return func(a *asyncOutput) {
		a.policy = policy
	}
}

// AsyncWithErrorHandler sets the handler receiving the wrapped output's
// write errors, drop notifications (wrapping `ErrAsyncDropped`),
// interval-flush errors, and panics from the wrapped output - converted to
// errors, so a misbehaving sink never kills the worker, nor the host
// process. The handler may be called concurrently.
func AsyncWithErrorHandler(handler func(error)) AsyncOption {
	return func(a *asyncOutput) {
		a.errorHandler = handler
	}
}

// AsyncWithFlushInterval periodically flushes the WRAPPED output - useful
// for time-buffered inner outputs (e.g. the ElasticSearch bulk output).
// Zero (the default) disables it. Flush errors are delivered to the error
// handler.
func AsyncWithFlushInterval(interval time.Duration) AsyncOption {
	return func(a *asyncOutput) {
		a.flushInterval = interval
	}
}

// asyncOutput is a buffered, asynchronous `IOutput` wrapper. Writes enqueue
// the message into a bounded buffer; a single worker goroutine drains it to
// the wrapped output - preserving FIFO order.
type asyncOutput struct {
	*proxyOutput

	// Immutable after construction.
	capacity      int
	errorHandler  func(error)
	flushInterval time.Duration
	policy        AsyncPolicy

	// mu guards the mutable state below. `cond` is signaled whenever the
	// buffer, the in-flight marker, or the closed flag change.
	mu   sync.Mutex
	cond *sync.Cond

	closed  bool
	dropped uint64
	queue   []message.IMessage

	// Sequence-based flush accounting. Sequences are assigned contiguously
	// at enqueue (starting at 1), so the queue always holds the sequences
	// `[headSeq, headSeq+len(queue)-1]`. A sequence is RESOLVED once its
	// message was written to the wrapped output, or dropped by policy -
	// `Flush` waits until every sequence enqueued before the call resolved.
	enqueuedSeq uint64 // Sequence of the most recently enqueued message.
	headSeq     uint64 // Sequence of `queue[0]` - meaningful when non-empty.
	inFlightSeq uint64 // Sequence being written by the worker - 0 when idle.

	// closeOnce guards Close - making it idempotent; closeErr records its
	// outcome for subsequent calls.
	closeOnce sync.Once
	closeErr  error

	// workerDone is closed when the worker goroutine exits. flusherStop
	// stops the interval flusher; flusherDone is closed when it exits.
	// Both flusher channels are nil when the interval is disabled.
	workerDone  chan struct{}
	flusherStop chan struct{}
	flusherDone chan struct{}
}

//////
// Methods.
//////

// Write enqueues the message. The message is expected to be this output's
// own copy - Sypl isolates messages per output - so retaining it is safe.
// Behavior on a full buffer is determined by the policy. After Close, it
// returns `ErrAsyncClosed`.
func (a *asyncOutput) Write(m message.IMessage) error {
	a.mu.Lock()

	if a.closed {
		a.mu.Unlock()

		return ErrAsyncClosed
	}

	switch a.policy {
	case AsyncPolicyDropNewest:
		if len(a.queue) >= a.capacity {
			a.dropped++

			total := a.dropped

			a.mu.Unlock()

			a.notifyDrop(total)

			return nil
		}
	case AsyncPolicyDropOldest:
		if len(a.queue) >= a.capacity {
			// Dropping the oldest RESOLVES its sequence - waiting
			// flushers are woken by the broadcast below.
			a.dequeueLocked()

			a.dropped++

			total := a.dropped

			a.enqueueLocked(m)

			a.cond.Broadcast()
			a.mu.Unlock()

			a.notifyDrop(total)

			return nil
		}
	case AsyncPolicyBlock:
		fallthrough
	default:
		for len(a.queue) >= a.capacity && !a.closed {
			a.cond.Wait()
		}

		if a.closed {
			a.mu.Unlock()

			return ErrAsyncClosed
		}
	}

	a.enqueueLocked(m)

	a.cond.Broadcast()
	a.mu.Unlock()

	return nil
}

// Flush guarantees - SNAPSHOT semantics - that every message enqueued
// BEFORE the call was resolved (written to the wrapped output, or dropped
// by policy), then flushes the wrapped output, if it implements
// `Flush() error`. Messages enqueued AFTER the call may remain buffered -
// so Flush returns even under sustained concurrent writes. After Close
// it's a no-op: Close already drained, and flushed everything.
//
// NOTE: Flush waits - UNBOUNDED - for the wrapped output's in-flight
// write: a sink that never returns blocks Flush forever. Only the Fatal
// path's pre-exit flush is time-bounded - see `Async` for the recommended
// guards.
func (a *asyncOutput) Flush() error {
	a.mu.Lock()

	target := a.enqueuedSeq

	// Terminates even mid-Close: the worker drains the whole buffer before
	// exiting, resolving every enqueued sequence.
	for a.minUnresolvedSeqLocked() <= target {
		a.cond.Wait()
	}

	closed := a.closed

	a.mu.Unlock()

	if closed {
		return nil
	}

	return a.flushInner()
}

// Close flushes, stops the worker (and the interval flusher, if any), and
// closes the wrapped output, if it implements `io.Closer`. It's idempotent:
// subsequent calls return the first call's outcome without re-closing.
// Writes after Close return `ErrAsyncClosed` - never panic.
//
// NOTE: Close waits - UNBOUNDED - for the worker to drain the buffer: a
// sink that never returns blocks Close forever. Only the Fatal path's
// pre-exit flush is time-bounded - see `Async` for the recommended guards.
func (a *asyncOutput) Close() error {
	a.closeOnce.Do(func() {
		a.mu.Lock()
		a.closed = true
		a.cond.Broadcast()
		a.mu.Unlock()

		// Stop the interval flusher, then wait for the worker to drain
		// the buffer, and exit.
		if a.flusherStop != nil {
			close(a.flusherStop)

			<-a.flusherDone
		}

		<-a.workerDone

		errs := []error{}

		errs = append(errs, a.flushInner())

		if c, ok := a.inner.(io.Closer); ok {
			errs = append(errs, c.Close())
		}

		a.closeErr = errors.Join(errs...)
	})

	return a.closeErr
}

//////
// Helpers.
//////

// enqueueLocked appends the message to the buffer, assigning it the next
// sequence. The caller must hold `mu`.
func (a *asyncOutput) enqueueLocked(m message.IMessage) {
	a.enqueuedSeq++

	if len(a.queue) == 0 {
		a.headSeq = a.enqueuedSeq
	}

	a.queue = append(a.queue, m)
}

// dequeueLocked removes, and returns the oldest buffered message with its
// sequence. The caller must hold `mu`, and guarantee the buffer isn't
// empty. The backing array slot is cleared so the message can be collected.
func (a *asyncOutput) dequeueLocked() (message.IMessage, uint64) {
	m := a.queue[0]

	seq := a.headSeq

	a.headSeq++

	copy(a.queue, a.queue[1:])

	a.queue[len(a.queue)-1] = nil
	a.queue = a.queue[:len(a.queue)-1]

	return m, seq
}

// minUnresolvedSeqLocked returns the lowest sequence not yet resolved -
// neither written to the wrapped output, nor dropped by policy. When
// everything resolved, it returns `enqueuedSeq + 1`. The caller must hold
// `mu`.
func (a *asyncOutput) minUnresolvedSeqLocked() uint64 {
	if a.inFlightSeq != 0 {
		return a.inFlightSeq
	}

	if len(a.queue) > 0 {
		return a.headSeq
	}

	return a.enqueuedSeq + 1
}

// writeInner writes the message to the wrapped output, converting a panic
// into an error - a misbehaving sink must never kill the worker goroutine,
// nor the host process.
func (a *asyncOutput) writeInner(m message.IMessage) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("async: panic in output %q: %v", a.GetName(), r)
		}
	}()

	return a.inner.Write(m)
}

// flushInner flushes the wrapped output, if it implements `Flush() error` -
// converting a panic into an error, exactly like `writeInner`.
func (a *asyncOutput) flushInner() (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("async: panic in output %q: %v", a.GetName(), r)
		}
	}()

	if f, ok := a.inner.(interface{ Flush() error }); ok {
		return f.Flush()
	}

	return nil
}

// notifyError delivers `err` to the error handler, if any.
func (a *asyncOutput) notifyError(err error) {
	if a.errorHandler != nil && err != nil {
		a.errorHandler(err)
	}
}

// notifyDrop delivers a drop notification - wrapping `ErrAsyncDropped` with
// context - to the error handler, if any.
func (a *asyncOutput) notifyDrop(total uint64) {
	if a.errorHandler == nil {
		return
	}

	a.errorHandler(fmt.Errorf(
		`%w: output "%s", buffer full (capacity: %d, policy: %s), total dropped: %d`,
		ErrAsyncDropped,
		a.GetName(),
		a.capacity,
		a.policy,
		total,
	))
}

// worker sequentially drains the buffer to the wrapped output, preserving
// FIFO order. It exits - after draining any remaining messages - when the
// output is closed.
func (a *asyncOutput) worker() {
	defer close(a.workerDone)

	for {
		a.mu.Lock()

		for len(a.queue) == 0 && !a.closed {
			a.cond.Wait()
		}

		if len(a.queue) == 0 && a.closed {
			a.mu.Unlock()

			return
		}

		m, seq := a.dequeueLocked()

		a.inFlightSeq = seq

		// The freed slot may unblock writers.
		a.cond.Broadcast()
		a.mu.Unlock()

		err := a.writeInner(m)

		a.mu.Lock()
		a.inFlightSeq = 0
		a.cond.Broadcast()
		a.mu.Unlock()

		a.notifyError(err)
	}
}

// flusher periodically flushes the wrapped output until stopped.
func (a *asyncOutput) flusher() {
	defer close(a.flusherDone)

	ticker := time.NewTicker(a.flushInterval)

	defer ticker.Stop()

	for {
		select {
		case <-a.flusherStop:
			return
		case <-ticker.C:
			a.notifyError(a.flushInner())
		}
	}
}

//////
// Factory.
//////

// Async wraps `o` into a bounded, buffered, asynchronous output: writes
// enqueue into a buffer (default capacity: 1024), and a single worker
// goroutine drains it to `o` - preserving FIFO order. All other `IOutput`
// methods are proxied to `o`, so Sypl-level dispatch (name matching, level
// checks, status checks) behaves identically.
//
// Capabilities:
// - `Flush() error`: guarantees everything enqueued BEFORE the call was
// written to `o` (or dropped by policy), then flushes `o` - if `o`
// implements `Flush() error`. SNAPSHOT semantics: messages enqueued after
// the call may remain buffered, so Flush returns even under sustained
// concurrent writes.
// - `Close() error`: flushes, stops the worker, and closes `o` - if `o`
// implements `io.Closer`. Idempotent. Writes after Close return
// `ErrAsyncClosed`.
//
// Hung sinks: direct Flush, and Close calls wait - UNBOUNDED - on the
// wrapped output's in-flight write, so a sink that never returns blocks
// them forever. The Fatal path's pre-exit flush is time-bounded (the
// process exits regardless), but direct Flush/Close are NOT: prefer
// bounded sinks (e.g. writers carrying I/O deadlines/timeouts), and
// `AsyncWithFlushInterval` for time-buffered inner outputs, so draining
// never depends on a single unbounded call.
//
// See the `Async*` options for buffer size, full-buffer policy, error
// handling, and periodic flushing.
func Async(o IOutput, opts ...AsyncOption) IOutput {
	a := &asyncOutput{
		capacity:   defaultAsyncBufferSize,
		policy:     AsyncPolicyBlock,
		workerDone: make(chan struct{}),
	}

	a.proxyOutput = newProxyOutput(o, a)

	a.cond = sync.NewCond(&a.mu)

	for _, opt := range opts {
		opt(a)
	}

	if a.flushInterval > 0 {
		a.flusherStop = make(chan struct{})
		a.flusherDone = make(chan struct{})

		go a.flusher()
	}

	go a.worker()

	return a
}
