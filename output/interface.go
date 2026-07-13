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
	"github.com/thalesfsp/sypl/v2/meta"
	"github.com/thalesfsp/sypl/v2/processor"
)

// IOutput specifies what an output does.
type IOutput interface {
	meta.IMeta

	// String interface.
	String() string

	// GetBuiltinLogger returns the Golang's builtin logger.
	GetBuiltinLogger() *builtin.Builtin

	// GetFormatter returns the formatter.
	GetFormatter() formatter.IFormatter

	// SetFormatter sets the formatter.
	SetFormatter(fmtr formatter.IFormatter) IOutput

	// GetMaxLevel returns the max level.
	GetMaxLevel() level.Level

	// SetMaxLevel sets the max level.
	SetMaxLevel(l level.Level) IOutput

	// AddProcessors adds one or more processors.
	AddProcessors(processors ...processor.IProcessor) IOutput

	// SetProcessors sets one or more processors.
	SetProcessors(processors ...processor.IProcessor) IOutput

	// GetProcessors returns registered processors.
	GetProcessors() []processor.IProcessor

	// GetProcessorsNames returns the names of the registered processors.
	GetProcessorsNames() []string

	// GetWriter returns the writer.
	GetWriter() io.Writer

	// SetWriter sets the writer.
	SetWriter(w io.Writer) IOutput

	// Write write the message to the defined output.
	Write(m message.IMessage) error
}
