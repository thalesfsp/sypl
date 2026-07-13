// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package output

import (
	"io"

	"github.com/thalesfsp/sypl/formatter"
	"github.com/thalesfsp/sypl/internal/builtin"
	"github.com/thalesfsp/sypl/level"
	"github.com/thalesfsp/sypl/message"
	"github.com/thalesfsp/sypl/processor"
	"github.com/thalesfsp/sypl/status"
)

// proxyOutput forwards every `IOutput` method to the wrapped (inner) output,
// so wrappers - such as the async output - behave, from Sypl's dispatch
// perspective (name matching, level checks, status checks), exactly like the
// output they wrap.
//
// Chainable setters return `self` - the outer wrapper - instead of the inner
// output, so the wrapping survives chained configuration calls, e.g.
// `Async(o).SetFormatter(f)`.
type proxyOutput struct {
	// inner is the wrapped output.
	inner IOutput

	// self is the outer wrapper, returned by chainable setters.
	self IOutput
}

// newProxyOutput is the proxyOutput factory. `self` is the outer wrapper
// embedding this proxy.
func newProxyOutput(inner, self IOutput) *proxyOutput {
	return &proxyOutput{
		inner: inner,
		self:  self,
	}
}

// String interface implementation.
func (p *proxyOutput) String() string {
	return p.inner.GetName()
}

//////
// IMeta interface implementation.
//////

// GetName returns the inner output name.
func (p *proxyOutput) GetName() string {
	return p.inner.GetName()
}

// SetName sets the inner output name.
func (p *proxyOutput) SetName(name string) {
	p.inner.SetName(name)
}

// GetStatus returns the inner output status.
func (p *proxyOutput) GetStatus() status.Status {
	return p.inner.GetStatus()
}

// SetStatus sets the inner output status.
func (p *proxyOutput) SetStatus(s status.Status) {
	p.inner.SetStatus(s)
}

//////
// IOutput interface implementation.
//////

// GetBuiltinLogger returns the inner output's Golang's builtin logger.
func (p *proxyOutput) GetBuiltinLogger() *builtin.Builtin {
	return p.inner.GetBuiltinLogger()
}

// SetBuiltinLogger sets the inner output's Golang's builtin logger.
func (p *proxyOutput) SetBuiltinLogger(builtinLogger *builtin.Builtin) IOutput {
	p.inner.SetBuiltinLogger(builtinLogger)

	return p.self
}

// GetFormatter returns the inner output's formatter.
func (p *proxyOutput) GetFormatter() formatter.IFormatter {
	return p.inner.GetFormatter()
}

// SetFormatter sets the inner output's formatter.
func (p *proxyOutput) SetFormatter(fmtr formatter.IFormatter) IOutput {
	p.inner.SetFormatter(fmtr)

	return p.self
}

// GetMaxLevel returns the inner output's max level.
func (p *proxyOutput) GetMaxLevel() level.Level {
	return p.inner.GetMaxLevel()
}

// SetMaxLevel sets the inner output's max level.
func (p *proxyOutput) SetMaxLevel(l level.Level) IOutput {
	p.inner.SetMaxLevel(l)

	return p.self
}

// AddProcessors adds one or more processors to the inner output.
func (p *proxyOutput) AddProcessors(processors ...processor.IProcessor) IOutput {
	p.inner.AddProcessors(processors...)

	return p.self
}

// GetProcessor returns the inner output's registered processor by its name.
// If not found, will be nil.
func (p *proxyOutput) GetProcessor(name string) processor.IProcessor {
	return p.inner.GetProcessor(name)
}

// SetProcessors sets one or more processors on the inner output.
func (p *proxyOutput) SetProcessors(processors ...processor.IProcessor) IOutput {
	p.inner.SetProcessors(processors...)

	return p.self
}

// GetProcessors returns the inner output's registered processors.
func (p *proxyOutput) GetProcessors() []processor.IProcessor {
	return p.inner.GetProcessors()
}

// GetProcessorsNames returns the names of the inner output's registered
// processors.
func (p *proxyOutput) GetProcessorsNames() []string {
	return p.inner.GetProcessorsNames()
}

// GetWriter returns the inner output's writer.
func (p *proxyOutput) GetWriter() io.Writer {
	return p.inner.GetWriter()
}

// SetWriter sets the inner output's writer.
func (p *proxyOutput) SetWriter(w io.Writer) IOutput {
	p.inner.SetWriter(w)

	return p.self
}

// Write writes the message to the inner output.
func (p *proxyOutput) Write(m message.IMessage) error {
	return p.inner.Write(m)
}
