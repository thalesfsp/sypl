// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package output

import (
	"io"

	"github.com/thalesfsp/sypl/v2/formatter"
	"github.com/thalesfsp/sypl/v2/internal/builtin"
	"github.com/thalesfsp/sypl/v2/level"
	"github.com/thalesfsp/sypl/v2/message"
	"github.com/thalesfsp/sypl/v2/processor"
	"github.com/thalesfsp/sypl/v2/status"
)

// Proxy forwards every `IOutput` method to the wrapped (inner) output,
// so wrappers - such as the async output - behave, from Sypl's dispatch
// perspective (name matching, level checks, status checks), exactly like the
// output they wrap.
//
// Chainable setters return `self` - the outer wrapper - instead of the inner
// output, so the wrapping survives chained configuration calls, e.g.
// `Async(o).SetFormatter(f)`.
//
// Exported so capability-carrying wrappers living outside this package -
// such as the `es` submodule's bulk output - can embed it.
type Proxy struct {
	// inner is the wrapped output.
	inner IOutput

	// self is the outer wrapper, returned by chainable setters.
	self IOutput
}

// NewProxy is the Proxy factory. `self` is the outer wrapper
// embedding this proxy.
func NewProxy(inner, self IOutput) *Proxy {
	return &Proxy{
		inner: inner,
		self:  self,
	}
}

// String interface implementation.
func (p *Proxy) String() string {
	return p.inner.GetName()
}

//////
// IMeta interface implementation.
//////

// GetName returns the inner output name.
func (p *Proxy) GetName() string {
	return p.inner.GetName()
}

// SetName sets the inner output name.
func (p *Proxy) SetName(name string) {
	p.inner.SetName(name)
}

// GetStatus returns the inner output status.
func (p *Proxy) GetStatus() status.Status {
	return p.inner.GetStatus()
}

// SetStatus sets the inner output status.
func (p *Proxy) SetStatus(s status.Status) {
	p.inner.SetStatus(s)
}

//////
// IOutput interface implementation.
//////

// GetBuiltinLogger returns the inner output's Golang's builtin logger.
func (p *Proxy) GetBuiltinLogger() *builtin.Builtin {
	return p.inner.GetBuiltinLogger()
}

// SetBuiltinLogger sets the inner output's Golang's builtin logger.
func (p *Proxy) SetBuiltinLogger(builtinLogger *builtin.Builtin) IOutput {
	p.inner.SetBuiltinLogger(builtinLogger)

	return p.self
}

// GetFormatter returns the inner output's formatter.
func (p *Proxy) GetFormatter() formatter.IFormatter {
	return p.inner.GetFormatter()
}

// SetFormatter sets the inner output's formatter.
func (p *Proxy) SetFormatter(fmtr formatter.IFormatter) IOutput {
	p.inner.SetFormatter(fmtr)

	return p.self
}

// GetMaxLevel returns the inner output's max level.
func (p *Proxy) GetMaxLevel() level.Level {
	return p.inner.GetMaxLevel()
}

// SetMaxLevel sets the inner output's max level.
func (p *Proxy) SetMaxLevel(l level.Level) IOutput {
	p.inner.SetMaxLevel(l)

	return p.self
}

// AddProcessors adds one or more processors to the inner output.
func (p *Proxy) AddProcessors(processors ...processor.IProcessor) IOutput {
	p.inner.AddProcessors(processors...)

	return p.self
}

// GetProcessor returns the inner output's registered processor by its name.
// If not found, will be nil.
func (p *Proxy) GetProcessor(name string) processor.IProcessor {
	return p.inner.GetProcessor(name)
}

// SetProcessors sets one or more processors on the inner output.
func (p *Proxy) SetProcessors(processors ...processor.IProcessor) IOutput {
	p.inner.SetProcessors(processors...)

	return p.self
}

// GetProcessors returns the inner output's registered processors.
func (p *Proxy) GetProcessors() []processor.IProcessor {
	return p.inner.GetProcessors()
}

// GetProcessorsNames returns the names of the inner output's registered
// processors.
func (p *Proxy) GetProcessorsNames() []string {
	return p.inner.GetProcessorsNames()
}

// GetWriter returns the inner output's writer.
func (p *Proxy) GetWriter() io.Writer {
	return p.inner.GetWriter()
}

// SetWriter sets the inner output's writer.
func (p *Proxy) SetWriter(w io.Writer) IOutput {
	p.inner.SetWriter(w)

	return p.self
}

// Write writes the message to the inner output.
func (p *Proxy) Write(m message.IMessage) error {
	return p.inner.Write(m)
}
