// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package output

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/thalesfsp/sypl/formatter"
	"github.com/thalesfsp/sypl/internal/builtin"
	"github.com/thalesfsp/sypl/level"
	"github.com/thalesfsp/sypl/message"
	"github.com/thalesfsp/sypl/processor"
	"github.com/thalesfsp/sypl/safebuffer"
	"github.com/thalesfsp/sypl/status"
)

//////
// Test helpers.
//////

// Recurring test literals.
const (
	asyncMsg0    = "m0\n"
	prefixerName = "Prefixer"
	renamedName  = "Renamed"
)

// gatedWriter blocks each Write until released, signaling when a Write
// starts - it makes the async worker's progress deterministic in tests.
type gatedWriter struct {
	// started receives one token per Write call, as it starts.
	started chan struct{}

	// release must receive one token per Write call, for it to finish.
	release chan struct{}

	buf safebuffer.Buffer
}

func newGatedWriter() *gatedWriter {
	return &gatedWriter{
		started: make(chan struct{}, 128),
		release: make(chan struct{}, 128),
	}
}

func (g *gatedWriter) Write(p []byte) (int, error) {
	g.started <- struct{}{}

	<-g.release

	return g.buf.Write(p)
}

// slowWriter delays each write - keeping the async queue non-empty under
// sustained concurrent writers.
type slowWriter struct {
	delay time.Duration
	buf   safebuffer.Buffer
}

func (s *slowWriter) Write(p []byte) (int, error) {
	time.Sleep(s.delay)

	return s.buf.Write(p)
}

// errorCollector concurrency-safely accumulates handler errors.
type errorCollector struct {
	mu     sync.Mutex
	errors []error

	// notified signals every collected error - buffered, never blocks.
	notified chan struct{}
}

func newErrorCollector() *errorCollector {
	return &errorCollector{notified: make(chan struct{}, 128)}
}

func (ec *errorCollector) handler() func(error) {
	return func(err error) {
		ec.mu.Lock()
		ec.errors = append(ec.errors, err)
		ec.mu.Unlock()

		select {
		case ec.notified <- struct{}{}:
		default:
		}
	}
}

func (ec *errorCollector) all() []error {
	ec.mu.Lock()
	defer ec.mu.Unlock()

	out := make([]error, len(ec.errors))
	copy(out, ec.errors)

	return out
}

// flushCloseOutput wraps an IOutput adding recordable Flush, and Close.
type flushCloseOutput struct {
	IOutput

	mu         sync.Mutex
	flushCount int
	closeCount int
	flushErr   error
	closeErr   error

	// flushed signals every Flush - buffered, never blocks.
	flushed chan struct{}
}

func newFlushCloseOutput(inner IOutput) *flushCloseOutput {
	return &flushCloseOutput{IOutput: inner, flushed: make(chan struct{}, 128)}
}

func (f *flushCloseOutput) Flush() error {
	f.mu.Lock()
	f.flushCount++
	err := f.flushErr
	f.mu.Unlock()

	select {
	case f.flushed <- struct{}{}:
	default:
	}

	return err
}

func (f *flushCloseOutput) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.closeCount++

	return f.closeErr
}

func (f *flushCloseOutput) counts() (int, int) {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.flushCount, f.closeCount
}

// asyncFlush flushes `o` via the Flush capability, failing the test if the
// capability is missing.
func asyncFlush(t *testing.T, o IOutput) error {
	t.Helper()

	f, ok := o.(interface{ Flush() error })

	if !ok {
		t.Fatal("Async output should implement Flush() error")
	}

	return f.Flush()
}

// asyncClose closes `o` via the Close capability, failing the test if the
// capability is missing.
func asyncClose(t *testing.T, o IOutput) error {
	t.Helper()

	c, ok := o.(interface{ Close() error })

	if !ok {
		t.Fatal("Async output should implement Close() error")
	}

	return c.Close()
}

//////
// Proxying.
//////

func TestAsync_ProxiesIOutput(t *testing.T) {
	var buf safebuffer.Buffer

	inner := New("Inner", level.Info, &buf, processor.Prefixer("p1: "))

	a := Async(inner)

	defer func() { _ = asyncClose(t, a) }()

	// String, and IMeta.
	if a.String() != "Inner" {
		t.Errorf("String() = %q, want %q", a.String(), "Inner")
	}

	if a.GetName() != "Inner" {
		t.Errorf("GetName() = %q, want %q", a.GetName(), "Inner")
	}

	a.SetName(renamedName)

	if a.GetName() != renamedName || inner.GetName() != renamedName {
		t.Error("SetName should reach the inner output")
	}

	if a.GetStatus() != status.Enabled {
		t.Errorf("GetStatus() = %v, want %v", a.GetStatus(), status.Enabled)
	}

	a.SetStatus(status.Disabled)

	if inner.GetStatus() != status.Disabled {
		t.Error("SetStatus should reach the inner output")
	}

	// Max level.
	if a.GetMaxLevel() != level.Info {
		t.Errorf("GetMaxLevel() = %v, want %v", a.GetMaxLevel(), level.Info)
	}

	if got := a.SetMaxLevel(level.Trace); got != a {
		t.Error("SetMaxLevel should return the wrapper - not the inner output")
	}

	if inner.GetMaxLevel() != level.Trace {
		t.Error("SetMaxLevel should reach the inner output")
	}

	// Formatter.
	if got := a.SetFormatter(formatter.Text()); got != a {
		t.Error("SetFormatter should return the wrapper - not the inner output")
	}

	if a.GetFormatter().GetName() != inner.GetFormatter().GetName() {
		t.Error("GetFormatter should proxy the inner output")
	}

	// Builtin logger.
	bl := builtin.NewBuiltin(&buf, "", 0)

	if got := a.SetBuiltinLogger(bl); got != a {
		t.Error("SetBuiltinLogger should return the wrapper - not the inner output")
	}

	if a.GetBuiltinLogger() != bl || inner.GetBuiltinLogger() != bl {
		t.Error("GetBuiltinLogger should proxy the inner output")
	}

	// Processors.
	if got := a.AddProcessors(processor.Suffixer(" s1")); got != a {
		t.Error("AddProcessors should return the wrapper - not the inner output")
	}

	if a.GetProcessor("Suffixer") == nil {
		t.Error("GetProcessor should proxy the inner output")
	}

	if len(a.GetProcessors()) != 2 {
		t.Errorf("GetProcessors() len = %d, want 2", len(a.GetProcessors()))
	}

	names := a.GetProcessorsNames()

	if len(names) != 2 || names[0] != prefixerName || names[1] != "Suffixer" {
		t.Errorf("GetProcessorsNames() = %v, want [Prefixer Suffixer]", names)
	}

	if got := a.SetProcessors(processor.Prefixer("p2: ")); got != a {
		t.Error("SetProcessors should return the wrapper - not the inner output")
	}

	// Writer.
	var otherBuf safebuffer.Buffer

	if got := a.SetWriter(&otherBuf); got != a {
		t.Error("SetWriter should return the wrapper - not the inner output")
	}

	if a.GetWriter() != &otherBuf || inner.GetWriter() != &otherBuf {
		t.Error("GetWriter should proxy the inner output")
	}
}

//////
// Policy naming.
//////

func TestAsyncPolicy_String(t *testing.T) {
	tests := []struct {
		policy AsyncPolicy
		want   string
	}{
		{AsyncPolicyBlock, "Block"},
		{AsyncPolicyDropNewest, "DropNewest"},
		{AsyncPolicyDropOldest, "DropOldest"},
		{AsyncPolicy(42), "Unknown"},
	}
	for _, tt := range tests {
		if got := tt.policy.String(); got != tt.want {
			t.Errorf("String() = %q, want %q", got, tt.want)
		}
	}
}

//////
// Ordering, and draining.
//////

func TestAsync_FIFOOrderingAndFlushDrains(t *testing.T) {
	buf, inner := SafeBuffer(level.Trace)

	a := Async(inner)

	const total = 100

	for i := range total {
		if err := a.Write(message.New(level.Info, fmt.Sprintf("m%03d\n", i))); err != nil {
			t.Fatalf("Write() error = %v, want nil", err)
		}
	}

	if err := asyncFlush(t, a); err != nil {
		t.Fatalf("Flush() error = %v, want nil", err)
	}

	lines := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")

	if len(lines) != total {
		t.Fatalf("Flushed %d lines, want %d", len(lines), total)
	}

	for i, line := range lines {
		if want := fmt.Sprintf("m%03d", i); line != want {
			t.Fatalf("Line %d = %q, want %q - FIFO ordering broken", i, line, want)
		}
	}

	if err := asyncClose(t, a); err != nil {
		t.Fatalf("Close() error = %v, want nil", err)
	}
}

//////
// Policies.
//////

func TestAsync_BlockPolicyBackpressure(t *testing.T) {
	gate := newGatedWriter()

	inner := New("Inner", level.Trace, gate)

	a := Async(inner, AsyncWithBufferSize(2), AsyncWithPolicy(AsyncPolicyBlock))

	// First message: picked up by the worker, blocked in the gate.
	if err := a.Write(message.New(level.Info, asyncMsg0)); err != nil {
		t.Fatalf("Write() error = %v, want nil", err)
	}

	<-gate.started

	// Fill the buffer.
	for i := 1; i <= 2; i++ {
		if err := a.Write(message.New(level.Info, fmt.Sprintf("m%d\n", i))); err != nil {
			t.Fatalf("Write() error = %v, want nil", err)
		}
	}

	// Buffer full - this write must block.
	blockedDone := make(chan struct{})

	go func() {
		defer close(blockedDone)

		if err := a.Write(message.New(level.Info, "m3\n")); err != nil {
			t.Errorf("Blocked Write() error = %v, want nil", err)
		}
	}()

	select {
	case <-blockedDone:
		t.Fatal("Write on a full buffer should block under the Block policy")
	case <-time.After(50 * time.Millisecond):
	}

	// Drain: releasing the gate frees buffer slots - the blocked write
	// completes.
	go func() {
		for range 4 {
			gate.release <- struct{}{}
		}
	}()

	select {
	case <-blockedDone:
	case <-time.After(2 * time.Second):
		t.Fatal("Blocked Write should complete once the buffer drains")
	}

	if err := asyncFlush(t, a); err != nil {
		t.Fatalf("Flush() error = %v, want nil", err)
	}

	if got := gate.buf.String(); got != "m0\nm1\nm2\nm3\n" {
		t.Errorf("Drained %q, want %q", got, "m0\nm1\nm2\nm3\n")
	}

	if err := asyncClose(t, a); err != nil {
		t.Fatalf("Close() error = %v, want nil", err)
	}
}

func TestAsync_DropPolicies(t *testing.T) {
	tests := []struct {
		name       string
		policy     AsyncPolicy
		wantOutput string
	}{
		{
			name:   "Should drop the newest message",
			policy: AsyncPolicyDropNewest,

			// m2 - the incoming message - is dropped.
			wantOutput: "m0\nm1\n",
		},
		{
			name:   "Should drop the oldest message",
			policy: AsyncPolicyDropOldest,

			// m1 - the oldest buffered message - is dropped.
			wantOutput: "m0\nm2\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gate, collector, a := primeDropScenario(t, tt.policy)

			// The drop notification carries the typed sentinel.
			select {
			case <-collector.notified:
			case <-time.After(2 * time.Second):
				t.Fatal("The error handler should be notified about the drop")
			}

			handlerErrors := collector.all()

			if len(handlerErrors) != 1 {
				t.Fatalf("Handler got %d errors, want 1: %v", len(handlerErrors), handlerErrors)
			}

			if !errors.Is(handlerErrors[0], ErrAsyncDropped) {
				t.Errorf("Handler error = %v, want ErrAsyncDropped", handlerErrors[0])
			}

			if !strings.Contains(handlerErrors[0].Error(), tt.policy.String()) {
				t.Errorf("Drop notification %q should carry the policy %q",
					handlerErrors[0].Error(), tt.policy.String())
			}

			// Drain, and verify WHICH message was dropped.
			drainFlushClose(t, gate, a, 2)

			if got := gate.buf.String(); got != tt.wantOutput {
				t.Errorf("Drained %q, want %q", got, tt.wantOutput)
			}
		})
	}
}

// primeDropScenario builds a gated async output with a single-slot buffer,
// and fills it past capacity: m0 in-flight (blocked in the gate), m1
// buffered - so m2 triggers the drop policy.
func primeDropScenario(
	t *testing.T,
	policy AsyncPolicy,
) (*gatedWriter, *errorCollector, IOutput) {
	t.Helper()

	gate := newGatedWriter()

	collector := newErrorCollector()

	inner := New("Inner", level.Trace, gate)

	a := Async(inner,
		AsyncWithBufferSize(1),
		AsyncWithPolicy(policy),
		AsyncWithErrorHandler(collector.handler()),
	)

	if err := a.Write(message.New(level.Info, asyncMsg0)); err != nil {
		t.Fatalf("Write() error = %v, want nil", err)
	}

	<-gate.started

	if err := a.Write(message.New(level.Info, "m1\n")); err != nil {
		t.Fatalf("Write() error = %v, want nil", err)
	}

	// Buffer full - this write triggers the drop policy. Never an error:
	// drops are reported through the handler.
	if err := a.Write(message.New(level.Info, "m2\n")); err != nil {
		t.Fatalf("Write() error = %v, want nil", err)
	}

	return gate, collector, a
}

// drainFlushClose releases the gate for `n` writes, then flushes, and
// closes the async output.
func drainFlushClose(t *testing.T, gate *gatedWriter, a IOutput, n int) {
	t.Helper()

	go func() {
		for range n {
			gate.release <- struct{}{}
		}
	}()

	if err := asyncFlush(t, a); err != nil {
		t.Fatalf("Flush() error = %v, want nil", err)
	}

	if err := asyncClose(t, a); err != nil {
		t.Fatalf("Close() error = %v, want nil", err)
	}
}

//////
// Flush.
//////

func TestAsync_FlushProxiesInnerFlush(t *testing.T) {
	buf, innerOutput := SafeBuffer(level.Trace)

	inner := newFlushCloseOutput(innerOutput)

	a := Async(inner)

	if err := a.Write(message.New(level.Info, asyncMsg0)); err != nil {
		t.Fatalf("Write() error = %v, want nil", err)
	}

	if err := asyncFlush(t, a); err != nil {
		t.Fatalf("Flush() error = %v, want nil", err)
	}

	if buf.String() != asyncMsg0 {
		t.Errorf("Buffer = %q, want %q", buf.String(), asyncMsg0)
	}

	flushes, _ := inner.counts()

	if flushes != 1 {
		t.Errorf("Inner Flush called %d times, want 1", flushes)
	}

	// Inner flush errors propagate.
	inner.mu.Lock()
	inner.flushErr = errors.New("flush failed")
	inner.mu.Unlock()

	if err := asyncFlush(t, a); err == nil || !strings.Contains(err.Error(), "flush failed") {
		t.Errorf("Flush() error = %v, want the inner flush error", err)
	}

	inner.mu.Lock()
	inner.flushErr = nil
	inner.mu.Unlock()

	if err := asyncClose(t, a); err != nil {
		t.Fatalf("Close() error = %v, want nil", err)
	}
}

// Flush has SNAPSHOT semantics: everything enqueued BEFORE the call is
// guaranteed written, and Flush returns even while concurrent writers keep
// hammering - a quiescence wait (`len(queue) == 0 && !inFlight`) livelocks
// here, as the queue never empties.
func TestAsync_FlushSnapshotUnderSustainedWrites(t *testing.T) {
	sink := &slowWriter{delay: time.Millisecond}

	inner := New("Inner", level.Trace, sink)

	a := Async(inner, AsyncWithBufferSize(16))

	const preFlush = 64

	for i := range preFlush {
		if err := a.Write(message.New(level.Info, fmt.Sprintf("pre-%03d\n", i))); err != nil {
			t.Fatalf("Write() error = %v, want nil", err)
		}
	}

	// 4 concurrent writers keep hammering for the whole flush.
	stop := make(chan struct{})

	var wg sync.WaitGroup

	for w := range 4 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for i := 0; ; i++ {
				select {
				case <-stop:
					return
				default:
				}

				_ = a.Write(message.New(level.Info, fmt.Sprintf("post-w%d-%d\n", w, i)))
			}
		}()
	}

	f, ok := a.(interface{ Flush() error })

	if !ok {
		t.Fatal("Async output should implement Flush() error")
	}

	flushErr := make(chan error, 1)

	go func() { flushErr <- f.Flush() }()

	select {
	case err := <-flushErr:
		if err != nil {
			t.Fatalf("Flush() error = %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Flush should return under sustained concurrent writes - quiescence livelock")
	}

	// Everything enqueued BEFORE the Flush call must be in the sink.
	content := sink.buf.String()

	for i := range preFlush {
		if want := fmt.Sprintf("pre-%03d", i); !strings.Contains(content, want) {
			t.Fatalf("Sink is missing %q - Flush returned before draining the snapshot", want)
		}
	}

	close(stop)

	wg.Wait()

	if err := asyncClose(t, a); err != nil {
		t.Fatalf("Close() error = %v, want nil", err)
	}
}

func TestAsync_FlushIntervalFires(t *testing.T) {
	_, innerOutput := SafeBuffer(level.Trace)

	inner := newFlushCloseOutput(innerOutput)

	a := Async(inner, AsyncWithFlushInterval(5*time.Millisecond))

	select {
	case <-inner.flushed:
	case <-time.After(2 * time.Second):
		t.Fatal("The flush interval should periodically flush the inner output")
	}

	if err := asyncClose(t, a); err != nil {
		t.Fatalf("Close() error = %v, want nil", err)
	}
}

func TestAsync_FlushIntervalErrorsReachHandler(t *testing.T) {
	collector := newErrorCollector()

	_, innerOutput := SafeBuffer(level.Trace)

	inner := newFlushCloseOutput(innerOutput)

	inner.mu.Lock()
	inner.flushErr = errors.New("interval flush failed")
	inner.mu.Unlock()

	a := Async(inner,
		AsyncWithFlushInterval(5*time.Millisecond),
		AsyncWithErrorHandler(collector.handler()),
	)

	select {
	case <-collector.notified:
	case <-time.After(2 * time.Second):
		t.Fatal("Interval flush errors should reach the error handler")
	}

	if handlerErrors := collector.all(); !strings.Contains(handlerErrors[0].Error(), "interval flush failed") {
		t.Errorf("Handler error = %v, want the interval flush error", handlerErrors[0])
	}

	// Clear the error so Close's final flush succeeds.
	inner.mu.Lock()
	inner.flushErr = nil
	inner.mu.Unlock()

	if err := asyncClose(t, a); err != nil {
		t.Fatalf("Close() error = %v, want nil", err)
	}
}

//////
// Close.
//////

func TestAsync_CloseFlushesClosesAndIsIdempotent(t *testing.T) {
	buf, innerOutput := SafeBuffer(level.Trace)

	inner := newFlushCloseOutput(innerOutput)

	a := Async(inner)

	if err := a.Write(message.New(level.Info, asyncMsg0)); err != nil {
		t.Fatalf("Write() error = %v, want nil", err)
	}

	if err := asyncClose(t, a); err != nil {
		t.Fatalf("Close() error = %v, want nil", err)
	}

	// Close drains pending messages, flushes, and closes the inner output.
	if buf.String() != asyncMsg0 {
		t.Errorf("Buffer = %q, want %q - Close should drain first", buf.String(), asyncMsg0)
	}

	flushes, closes := inner.counts()

	if flushes != 1 || closes != 1 {
		t.Errorf("Inner flushes = %d, closes = %d, want 1, and 1", flushes, closes)
	}

	// Idempotent: a second Close is a no-op.
	if err := asyncClose(t, a); err != nil {
		t.Fatalf("Second Close() error = %v, want nil", err)
	}

	if _, closes := inner.counts(); closes != 1 {
		t.Errorf("Inner closes = %d after double Close, want 1", closes)
	}

	// Write after Close: typed error, no panic.
	if err := a.Write(message.New(level.Info, "late\n")); !errors.Is(err, ErrAsyncClosed) {
		t.Errorf("Write() after Close error = %v, want ErrAsyncClosed", err)
	}

	// Flush after Close: documented no-op.
	if err := asyncFlush(t, a); err != nil {
		t.Errorf("Flush() after Close error = %v, want nil", err)
	}

	if flushes, _ := inner.counts(); flushes != 1 {
		t.Errorf("Inner flushes = %d after post-Close Flush, want 1", flushes)
	}
}

func TestAsync_CloseAggregatesInnerErrors(t *testing.T) {
	_, innerOutput := SafeBuffer(level.Trace)

	inner := newFlushCloseOutput(innerOutput)

	flushErr := errors.New("inner flush error")
	closeErr := errors.New("inner close error")

	inner.mu.Lock()
	inner.flushErr = flushErr
	inner.closeErr = closeErr
	inner.mu.Unlock()

	a := Async(inner)

	err := asyncClose(t, a)

	if !errors.Is(err, flushErr) {
		t.Errorf("Close() error = %v, want the inner flush error joined", err)
	}

	if !errors.Is(err, closeErr) {
		t.Errorf("Close() error = %v, want the inner close error joined", err)
	}

	// Idempotent: the same aggregated error is returned again - the inner
	// output is NOT flushed/closed twice.
	if second := asyncClose(t, a); !errors.Is(second, closeErr) {
		t.Errorf("Second Close() error = %v, want the recorded error", second)
	}

	flushes, closes := inner.counts()

	if flushes != 1 || closes != 1 {
		t.Errorf("Inner flushes = %d, closes = %d, want 1, and 1", flushes, closes)
	}
}

func TestAsync_CloseWithoutInnerCapabilities(t *testing.T) {
	// A plain inner output - no Flush, no Close capability. The wrapper's
	// Flush, and Close must still work.
	buf, inner := SafeBuffer(level.Trace)

	a := Async(inner)

	if err := a.Write(message.New(level.Info, asyncMsg0)); err != nil {
		t.Fatalf("Write() error = %v, want nil", err)
	}

	if err := asyncFlush(t, a); err != nil {
		t.Fatalf("Flush() error = %v, want nil", err)
	}

	if err := asyncClose(t, a); err != nil {
		t.Fatalf("Close() error = %v, want nil", err)
	}

	if buf.String() != asyncMsg0 {
		t.Errorf("Buffer = %q, want %q", buf.String(), asyncMsg0)
	}
}

func TestAsync_BlockedWriterUnblocksOnCloseWithTypedError(t *testing.T) {
	gate := newGatedWriter()

	inner := New("Inner", level.Trace, gate)

	a := Async(inner, AsyncWithBufferSize(1))

	// Worker in flight, buffer full.
	if err := a.Write(message.New(level.Info, asyncMsg0)); err != nil {
		t.Fatalf("Write() error = %v, want nil", err)
	}

	<-gate.started

	if err := a.Write(message.New(level.Info, "m1\n")); err != nil {
		t.Fatalf("Write() error = %v, want nil", err)
	}

	// This write blocks - the worker is gated, so the buffer stays full.
	blockedErr := make(chan error, 1)

	go func() {
		blockedErr <- a.Write(message.New(level.Info, "m2\n"))
	}()

	// Close wakes the blocked writer with the typed error. Run it in the
	// background: Close itself blocks draining the gated worker.
	closeErr := make(chan error, 1)

	go func() {
		closeErr <- asyncClose(t, a)
	}()

	select {
	case err := <-blockedErr:
		if !errors.Is(err, ErrAsyncClosed) {
			t.Errorf("Blocked Write() error = %v, want ErrAsyncClosed", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Close should unblock a blocked writer")
	}

	// Release the gate - Close drains m0, and m1, then completes.
	go func() {
		for range 2 {
			gate.release <- struct{}{}
		}
	}()

	select {
	case err := <-closeErr:
		if err != nil {
			t.Errorf("Close() error = %v, want nil", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Close should complete once the queue drains")
	}

	if got := gate.buf.String(); got != "m0\nm1\n" {
		t.Errorf("Drained %q, want %q", got, "m0\nm1\n")
	}
}

func TestAsync_DropWithoutHandlerDoesNotPanic(t *testing.T) {
	gate := newGatedWriter()

	inner := New("Inner", level.Trace, gate)

	a := Async(inner, AsyncWithBufferSize(1), AsyncWithPolicy(AsyncPolicyDropNewest))

	if err := a.Write(message.New(level.Info, asyncMsg0)); err != nil {
		t.Fatalf("Write() error = %v, want nil", err)
	}

	<-gate.started

	if err := a.Write(message.New(level.Info, "m1\n")); err != nil {
		t.Fatalf("Write() error = %v, want nil", err)
	}

	// Buffer full, NO handler configured: the drop must be silent - never
	// a panic.
	if err := a.Write(message.New(level.Info, "m2\n")); err != nil {
		t.Fatalf("Write() error = %v, want nil", err)
	}

	go func() {
		for range 2 {
			gate.release <- struct{}{}
		}
	}()

	if err := asyncClose(t, a); err != nil {
		t.Fatalf("Close() error = %v, want nil", err)
	}
}

//////
// Error handling.
//////

func TestAsync_InnerWriteErrorsReachHandler(t *testing.T) {
	collector := newErrorCollector()

	inner := New("Inner", level.Trace, &failingWriter{err: errors.New("boom")})

	a := Async(inner, AsyncWithErrorHandler(collector.handler()))

	if err := a.Write(message.New(level.Info, asyncMsg0)); err != nil {
		t.Fatalf("Write() error = %v, want nil", err)
	}

	select {
	case <-collector.notified:
	case <-time.After(2 * time.Second):
		t.Fatal("Inner write errors should reach the error handler")
	}

	if handlerErrors := collector.all(); !strings.Contains(handlerErrors[0].Error(), "boom") {
		t.Errorf("Handler error = %v, want the inner write error", handlerErrors[0])
	}

	if err := asyncClose(t, a); err != nil {
		t.Fatalf("Close() error = %v, want nil", err)
	}
}

func TestAsync_InnerWriteErrorsWithoutHandlerDoNotPanic(t *testing.T) {
	inner := New("Inner", level.Trace, &failingWriter{err: errors.New("boom")})

	a := Async(inner)

	if err := a.Write(message.New(level.Info, asyncMsg0)); err != nil {
		t.Fatalf("Write() error = %v, want nil", err)
	}

	if err := asyncFlush(t, a); err != nil {
		t.Fatalf("Flush() error = %v, want nil", err)
	}

	if err := asyncClose(t, a); err != nil {
		t.Fatalf("Close() error = %v, want nil", err)
	}
}

//////
// Panic containment.
//////

// panickyOutput panics on a configurable number of writes/flushes, then
// delegates.
type panickyOutput struct {
	IOutput

	writePanics atomic.Int32
	flushPanics atomic.Int32
}

func (p *panickyOutput) Write(m message.IMessage) error {
	if p.writePanics.Add(-1) >= 0 {
		panic("sink exploded")
	}

	return p.IOutput.Write(m)
}

func (p *panickyOutput) Flush() error {
	if p.flushPanics.Add(-1) >= 0 {
		panic("flush exploded")
	}

	return nil
}

// A panicking inner output must not kill the worker - nor the host
// process: the panic is converted to an error, delivered to the handler,
// and subsequent messages are still delivered.
func TestAsync_WorkerPanicContained(t *testing.T) {
	collector := newErrorCollector()

	buf, inner := SafeBuffer(level.Trace)

	p := &panickyOutput{IOutput: inner}
	p.writePanics.Store(1)

	a := Async(p, AsyncWithErrorHandler(collector.handler()))

	if err := a.Write(message.New(level.Info, "boom\n")); err != nil {
		t.Fatalf("Write() error = %v, want nil", err)
	}

	select {
	case <-collector.notified:
	case <-time.After(2 * time.Second):
		t.Fatal("The panic should reach the error handler as an error")
	}

	handlerErrors := collector.all()

	if !strings.Contains(handlerErrors[0].Error(), "panic in output") ||
		!strings.Contains(handlerErrors[0].Error(), "sink exploded") {
		t.Errorf("Handler error = %v, want the converted panic", handlerErrors[0])
	}

	// The worker survived: subsequent messages are still delivered.
	if err := a.Write(message.New(level.Info, "after\n")); err != nil {
		t.Fatalf("Write() error = %v, want nil", err)
	}

	if err := asyncFlush(t, a); err != nil {
		t.Fatalf("Flush() error = %v, want nil", err)
	}

	if !strings.Contains(buf.String(), "after") {
		t.Errorf("Buffer = %q, want the post-panic message delivered", buf.String())
	}

	if err := asyncClose(t, a); err != nil {
		t.Fatalf("Close() error = %v, want nil", err)
	}
}

// A panicking inner Flush comes back from the direct Flush path as an
// error - never a panic.
func TestAsync_FlushPanicContained(t *testing.T) {
	_, inner := SafeBuffer(level.Trace)

	p := &panickyOutput{IOutput: inner}
	p.flushPanics.Store(1)

	a := Async(p)

	err := asyncFlush(t, a)

	if err == nil || !strings.Contains(err.Error(), "panic in output") ||
		!strings.Contains(err.Error(), "flush exploded") {
		t.Errorf("Flush() error = %v, want the converted panic", err)
	}

	if err := asyncClose(t, a); err != nil {
		t.Fatalf("Close() error = %v, want nil", err)
	}
}

// A panicking inner Flush must not kill the interval flusher either: the
// panic reaches the handler, and the flusher keeps running.
func TestAsync_FlushIntervalPanicContained(t *testing.T) {
	collector := newErrorCollector()

	_, inner := SafeBuffer(level.Trace)

	p := &panickyOutput{IOutput: inner}
	p.flushPanics.Store(1)

	a := Async(p,
		AsyncWithFlushInterval(5*time.Millisecond),
		AsyncWithErrorHandler(collector.handler()),
	)

	select {
	case <-collector.notified:
	case <-time.After(2 * time.Second):
		t.Fatal("The interval-flush panic should reach the error handler")
	}

	if handlerErrors := collector.all(); !strings.Contains(handlerErrors[0].Error(), "flush exploded") {
		t.Errorf("Handler error = %v, want the converted panic", handlerErrors[0])
	}

	// The flusher survived - Close (flushing once more) succeeds.
	if err := asyncClose(t, a); err != nil {
		t.Fatalf("Close() error = %v, want nil", err)
	}
}

//////
// Options.
//////

func TestAsync_BufferSizeDefaults(t *testing.T) {
	tests := []struct {
		name string
		opts []AsyncOption
		want int
	}{
		{
			name: "Should default to 1024",
			opts: nil,
			want: 1024,
		},
		{
			name: "Should apply a custom size",
			opts: []AsyncOption{AsyncWithBufferSize(7)},
			want: 7,
		},
		{
			name: "Should fall back to the default on a non-positive size",
			opts: []AsyncOption{AsyncWithBufferSize(-1)},
			want: 1024,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, inner := SafeBuffer(level.Trace)

			a := Async(inner, tt.opts...)

			defer func() { _ = asyncClose(t, a) }()

			concrete, ok := a.(*asyncOutput)

			if !ok {
				t.Fatal("Async should return an *asyncOutput")
			}

			if concrete.capacity != tt.want {
				t.Errorf("capacity = %d, want %d", concrete.capacity, tt.want)
			}
		})
	}
}

//////
// Goroutine hygiene.
//////

func TestAsync_NoGoroutineLeak(t *testing.T) {
	before := runtime.NumGoroutine()

	for range 10 {
		_, inner := SafeBuffer(level.Trace)

		a := Async(inner, AsyncWithFlushInterval(time.Millisecond))

		if err := a.Write(message.New(level.Info, asyncMsg0)); err != nil {
			t.Fatalf("Write() error = %v, want nil", err)
		}

		// Close waits for the worker, and the interval flusher to exit -
		// its return is itself proof both goroutines stopped.
		if err := asyncClose(t, a); err != nil {
			t.Fatalf("Close() error = %v, want nil", err)
		}
	}

	// Belt, and braces: the goroutine count returns to the baseline.
	deadline := time.Now().Add(2 * time.Second)

	for runtime.NumGoroutine() > before {
		if time.Now().After(deadline) {
			t.Fatalf("Goroutines leaked: before = %d, after = %d", before, runtime.NumGoroutine())
		}

		time.Sleep(time.Millisecond)
	}
}

//////
// Concurrency.
//////

func TestAsync_ConcurrentWritersFlushersAndClose(t *testing.T) {
	buf, inner := SafeBuffer(level.Trace)

	a := Async(inner, AsyncWithBufferSize(8))

	const (
		writers           = 8
		messagesPerWriter = 50
	)

	var written atomic.Int64

	var wg sync.WaitGroup

	// Concurrent writers.
	for w := range writers {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for i := range messagesPerWriter {
				if err := a.Write(message.New(level.Info, fmt.Sprintf("w%d-m%d\n", w, i))); err == nil {
					written.Add(1)
				}
			}
		}()
	}

	// Concurrent flushers.
	for range 2 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for range 5 {
				_ = asyncFlush(t, a)
			}
		}()
	}

	wg.Wait()

	// Concurrent Closes.
	var closeWg sync.WaitGroup

	for range 2 {
		closeWg.Add(1)

		go func() {
			defer closeWg.Done()

			if err := asyncClose(t, a); err != nil {
				t.Errorf("Close() error = %v, want nil", err)
			}
		}()
	}

	closeWg.Wait()

	// Block policy, and Close draining: every accepted write must be on
	// the buffer.
	gotLines := strings.Count(buf.String(), "\n")

	if int64(gotLines) != written.Load() {
		t.Errorf("Buffer has %d lines, want %d", gotLines, written.Load())
	}
}
