// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package output

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"slices"
	"strings"
	"sync"
	"syscall"

	"github.com/thalesfsp/sypl/flag"
	"github.com/thalesfsp/sypl/formatter"
	"github.com/thalesfsp/sypl/internal/builtin"
	"github.com/thalesfsp/sypl/level"
	"github.com/thalesfsp/sypl/message"
	"github.com/thalesfsp/sypl/processor"
	"github.com/thalesfsp/sypl/shared"
	"github.com/thalesfsp/sypl/status"
)

// Output process, and write the message to the defined writer. A writer is
// anything that implements io.Writer.
//
// Notes:
// - Any message with a `level` beyond `maxLevel` will not be written.
// - Messages are processed according to the order processors are added.
type output struct {
	// mu guards the mutable state below, allowing the output to be
	// safely reconfigured while logging.
	//
	// NOTE: Never held across the actual write - the builtin logger has
	// its own mutex serializing writes.
	mu sync.RWMutex

	// Golang's builtin logger.
	builtinLogger *builtin.Builtin

	// Formats the message.
	formatter formatter.IFormatter

	// Any message above the max level will not be written.
	maxLevel level.Level

	// Name of the processor.
	name string

	// Processors used to process the message.
	processors []processor.IProcessor

	// Status of the processor.
	status status.Status

	// Writer to write.
	writer io.Writer
}

// String interface implementation.
func (o *output) String() string {
	return o.GetName()
}

//////
// IMeta interface implementation.
//////

// GetName returns the processor name.
func (o *output) GetName() string {
	o.mu.RLock()
	defer o.mu.RUnlock()

	return o.name
}

// SetName sets the processor name.
func (o *output) SetName(name string) {
	o.mu.Lock()
	defer o.mu.Unlock()

	o.name = name
}

// GetStatus returns the processor status.
func (o *output) GetStatus() status.Status {
	o.mu.RLock()
	defer o.mu.RUnlock()

	return o.status
}

// SetStatus sets the processor status.
func (o *output) SetStatus(s status.Status) {
	o.mu.Lock()
	defer o.mu.Unlock()

	o.status = s
}

//////
// IOutput interface implementation.
//////

// GetBuiltinLogger returns the Golang's builtin logger.
func (o *output) GetBuiltinLogger() *builtin.Builtin {
	o.mu.RLock()
	defer o.mu.RUnlock()

	return o.builtinLogger
}

// SetBuiltinLogger sets the Golang's builtin logger.
func (o *output) SetBuiltinLogger(builtinLogger *builtin.Builtin) IOutput {
	o.mu.Lock()
	defer o.mu.Unlock()

	o.builtinLogger = builtinLogger

	return o
}

// GetFormatter returns the formatter.
func (o *output) GetFormatter() formatter.IFormatter {
	o.mu.RLock()
	defer o.mu.RUnlock()

	return o.formatter
}

// SetFormatter sets the formatter.
func (o *output) SetFormatter(fmtr formatter.IFormatter) IOutput {
	o.mu.Lock()
	defer o.mu.Unlock()

	o.formatter = fmtr

	return o
}

// GetMaxLevel returns the max level.
func (o *output) GetMaxLevel() level.Level {
	o.mu.RLock()
	defer o.mu.RUnlock()

	return o.maxLevel
}

// SetMaxLevel sets the max level.
func (o *output) SetMaxLevel(l level.Level) IOutput {
	o.mu.Lock()
	defer o.mu.Unlock()

	o.maxLevel = l

	return o
}

// AddProcessors adds one or more processors.
func (o *output) AddProcessors(processors ...processor.IProcessor) IOutput {
	o.mu.Lock()
	defer o.mu.Unlock()

	o.processors = append(o.processors, processors...)

	return o
}

// GetProcessor returns the registered processor by its name. If not found, will
// be nil.
func (o *output) GetProcessor(name string) processor.IProcessor {
	for _, p := range o.GetProcessors() {
		if strings.EqualFold(p.GetName(), name) {
			return p
		}
	}

	return nil
}

// GetProcessors returns registered processors.
func (o *output) GetProcessors() []processor.IProcessor {
	o.mu.RLock()
	defer o.mu.RUnlock()

	return o.processors
}

// SetProcessors sets one or more processors.
func (o *output) SetProcessors(processors ...processor.IProcessor) IOutput {
	o.mu.Lock()
	defer o.mu.Unlock()

	// Operates on a fresh copy of the slice so concurrent readers,
	// iterating over a previously obtained slice, never observe in-place
	// writes.
	updated := make([]processor.IProcessor, len(o.processors))
	copy(updated, o.processors)

	for _, processor := range processors {
		for i, p := range updated {
			if strings.EqualFold(p.GetName(), processor.GetName()) {
				updated[i] = processor
			}
		}
	}

	o.processors = updated

	return o
}

// GetProcessorsNames returns the names of the registered processors.
func (o *output) GetProcessorsNames() []string {
	processorsNames := []string{}

	for _, processor := range o.GetProcessors() {
		processorsNames = append(processorsNames, processor.GetName())
	}

	return processorsNames
}

// GetWriter returns the writer.
func (o *output) GetWriter() io.Writer {
	o.mu.RLock()
	defer o.mu.RUnlock()

	return o.writer
}

// SetWriter sets the writer.
func (o *output) SetWriter(w io.Writer) IOutput {
	o.mu.Lock()
	defer o.mu.Unlock()

	o.writer = w

	return o
}

// Write the message to the defined output. In case of any error, it can be
// introspected, providing more information about the failure. The error will be
// the type of `ProcessingError`.
//
//nolint:nestif
func (o *output) Write(m message.IMessage) error {
	// Should allows to specify `Output`(s).
	processorsNames := o.GetProcessorsNames()

	if len(m.GetProcessorsNames()) > 0 {
		processorsNames = m.GetProcessorsNames()
	}

	m.SetProcessorsNames(processorsNames)

	// Strips the last line break, which allows the content to be
	// properly processed. It gets restore later, if any.
	m.Strip()

	// Executes processors in series.
	o.processProcessors(m, processorsNames)

	// Should print the message - regardless of the level, if flagged
	// with `Force`.
	if m.GetFlag() == flag.Force || m.GetFlag() == flag.SkipAndForce {
		if err := o.write(m); err != nil {
			log.Println(shared.ErrorPrefix, err)

			return err
		}
	} else {
		// Debug capability.
		finalMaxLevel := o.GetMaxLevel()

		// Should only run if Debug env var is set.
		if os.Getenv(shared.LevelEnvVar) != "" {
			debug := m.GetDebugEnvVarRegexes()

			l, _, ok := debug.Level()

			if ok {
				finalMaxLevel = l
			}
		}

		// Should only print if message `level` isn't above `MaxLevel`.
		// Should only print if `level` isn't `None`.
		// Should only print if not flagged with `Mute`, or `SkipAndMute`.
		if m.GetLevel() != level.None &&
			m.GetLevel() <= finalMaxLevel &&
			m.GetFlag() != flag.Mute &&
			m.GetFlag() != flag.SkipAndMute {
			if err := o.write(m); err != nil {
				log.Println(shared.ErrorPrefix, err)

				return err
			}
		}
	}

	return nil
}

//////
// Helpers.
//////

// contains checks if `list` contains - exact, case-insensitive match - the
// specified `name`.
func contains(list []string, name string) bool {
	for _, item := range list {
		if strings.EqualFold(item, name) {
			return true
		}
	}

	return false
}

// Processors logic of the Write method.
func (o *output) processProcessors(m message.IMessage, processorsNames []string) {
	// Should not process if message is flagged with `Skip`, `SkipAndForce`,
	// or `SkipAndMute`.
	if m.GetFlag() != flag.Skip &&
		m.GetFlag() != flag.SkipAndForce &&
		m.GetFlag() != flag.SkipAndMute {
		for _, p := range o.GetProcessors() {
			// Should only use enabled Processors, and named (listed) ones.
			//
			// NOTE: `Enabled` status is checked in the `Run` method.
			if contains(processorsNames, p.GetName()) {
				m.SetProcessorName(p.GetName())

				if err := p.Run(m); err != nil {
					log.Println(shared.ErrorPrefix,
						processor.NewProcessingError(m, err))
				}
			}
		}
	}
}

// DRY for the writing step.
func (o *output) write(m message.IMessage) error {
	// Should only format if any, and if not flagged.
	if o.GetFormatter() != nil &&
		m.GetFlag() != flag.Skip &&
		m.GetFlag() != flag.SkipAndForce &&
		m.GetFlag() != flag.SkipAndMute {
		if err := o.GetFormatter().Run(m); err != nil {
			log.Println(shared.ErrorPrefix, processor.NewProcessingError(m, err))
		}
	}

	// Restore linebreak(s), if needed.
	m.Restore()

	// Write to writer.
	if err := o.GetBuiltinLogger().OutputBuiltin(
		builtin.DefaultCallDepth,
		m.GetContent().GetProcessed(),
	); err != nil {
		// It means application using Sypl was piped, but the pipe was broken so
		// nothing to do.
		if errors.Is(err, syscall.EPIPE) {
			return nil
		}

		// It the output passed to Sypl is already closed, nothing to do.
		if errors.Is(err, os.ErrClosed) {
			log.Printf(`%s Attempt to write to closed writer. Output: "%s". Error: "%v"`,
				shared.WarnPrefix,
				o.GetName(),
				err,
			)

			return nil
		}

		return fmt.Errorf(`output: "%s". error: "%w"`, o.GetName(), err)
	}

	return nil
}

//////
// Factory.
//////

// New is the Output factory.
func New(name string,
	maxLevel level.Level,
	w io.Writer,
	processors ...processor.IProcessor,
) IOutput {
	return &output{
		builtinLogger: builtin.NewBuiltin(w, "", 0),
		maxLevel:      maxLevel,

		name: name,
		// Defensively cloned: a caller passing `mySlice...` shares the
		// backing array with this output - and with any other output built
		// from the same slice - so a later `AddProcessors` append could
		// write into a sibling's spare-capacity slot. The processor
		// ELEMENTS stay shared by design.
		processors: slices.Clone(processors),
		status:     status.Enabled,
		writer:     w,
	}
}
