// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package sypl

import (
	"bufio"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/thalesfsp/sypl/debug"
	"github.com/thalesfsp/sypl/fields"
	"github.com/thalesfsp/sypl/flag"
	"github.com/thalesfsp/sypl/formatter"
	"github.com/thalesfsp/sypl/level"
	"github.com/thalesfsp/sypl/message"
	"github.com/thalesfsp/sypl/options"
	"github.com/thalesfsp/sypl/output"
	"github.com/thalesfsp/sypl/processor"
	"github.com/thalesfsp/sypl/shared"
	"github.com/thalesfsp/sypl/status"
	"golang.org/x/sync/errgroup"
)

// MessageToOutput defines a `Message` to printed at the specified `Level`, and
// to the specified `Output`.
type MessageToOutput struct {
	// Content to be printed.
	Content string

	// Level of the message.
	Level level.Level

	// OutputName name of the output.
	OutputName string
}

// Sypl logger definition.
type Sypl struct {
	// Name returns the logger name.
	//
	// NOTE: Exposed to deal with https://github.com/golang/go/issues/5819.
	Name string

	// NOTE: Changes here may reflect in the `New(name string)` method (Child).
	defaultIoWriterLevel level.Level
	fields               fields.Fields
	outputs              []output.IOutput
	status               status.Status
	tags                 []string
}

// String interface implementation.
func (sypl Sypl) String() string {
	return sypl.Name
}

//////
// IMeta interface implementation.
//////

// GetName returns the sypl Name.
func (sypl *Sypl) GetName() string {
	return sypl.Name
}

// SetName sets the sypl Name.
func (sypl *Sypl) SetName(name string) {
	sypl.Name = name
}

// GetStatus returns the sypl status.
func (sypl *Sypl) GetStatus() status.Status {
	return sypl.status
}

// SetStatus sets the sypl status.
func (sypl *Sypl) SetStatus(s status.Status) {
	sypl.status = s
}

// GetDefaultIoWriterLevel returns the sypl status.
func (sypl *Sypl) GetDefaultIoWriterLevel() level.Level {
	return sypl.defaultIoWriterLevel
}

// SetDefaultIoWriterLevel sets the default io.Writer level.
func (sypl *Sypl) SetDefaultIoWriterLevel(l level.Level) {
	sypl.defaultIoWriterLevel = l
}

//////
// Writer interface implementation.
//////

// Writer implements the io.Writer interface. Message level will be the one set
// via `SetIoWriterLevel`, default is `none`. It always returns `0, nil`.
//
// NOTE: This is a convenient method, if it doesn't fits your need, just
// implement the way you need.
func (sypl *Sypl) Write(p []byte) (int, error) {
	sypl.process(message.New(sypl.defaultIoWriterLevel, string(p)))

	return 0, nil
}

//////
// IBasePrinter interface implementation.
//////

// PrintMessage prints messages. It's a powerful option because it gives
// full-control over the message. Use `New` to create the message.
// it gives full-control over the message. Use `New` to create the
// message.
func (sypl *Sypl) PrintMessage(messages ...message.IMessage) ISypl {
	sypl.process(messages...)

	return sypl
}

// PrintWithOptions is a more flexible way of printing, allowing to specify
// a few message's options in a functional way. For full-control over the
// message is possible via `PrintMessage`.
func (sypl *Sypl) PrintWithOptions(l level.Level, ct string, o ...OptionFunc) ISypl {
	m := message.New(l, ct)

	// Iterate over the options.
	for _, opt := range o {
		m = opt(m)
	}

	return sypl.PrintMessage(m)
}

// PrintlnWithOptions is a more flexible way of printing, allowing to specify
// a few message's options in a functional way. For full-control over the
// message is possible via `PrintMessage`.
func (sypl *Sypl) PrintlnWithOptions(l level.Level, ct string, o ...OptionFunc) ISypl {
	return sypl.PrintWithOptions(l, fmt.Sprintln(ct), o...)
}

//////
// IBasicPrinter interface implementation.
//////

// Print just prints.
func (sypl *Sypl) Print(l level.Level, args ...interface{}) ISypl {
	return sypl.PrintWithOptions(l, fmt.Sprint(args...))
}

// Printf prints according with the specified format.
func (sypl *Sypl) Printf(l level.Level, format string, args ...interface{}) ISypl {
	return sypl.PrintWithOptions(l, fmt.Sprintf(format, args...))
}

// Printlnf prints according with the specified format, also adding a new line
// to the end.
func (sypl *Sypl) Printlnf(l level.Level, format string, args ...interface{}) ISypl {
	return sypl.PrintWithOptions(l, fmt.Sprintf(format+"\n", args...))
}

// Println prints, also adding a new line to the end.
func (sypl *Sypl) Println(l level.Level, args ...interface{}) ISypl {
	return sypl.PrintWithOptions(l, fmt.Sprintln(args...))
}

//////
// IConvenientPrinter interface implementation.
//////

// PrintPretty prints data structures as JSON text.
//
// Notes:
// - Only exported fields of the data structure will be printed.
// - Message isn't processed.
func (sypl *Sypl) PrintPretty(l level.Level, data interface{}) ISypl {
	msg := message.New(l, fmt.Sprint(shared.Prettify(data)))
	msg.SetFlag(flag.Skip)

	return sypl.PrintMessage(msg)
}

// PrintlnPretty prints data structures as JSON text, also adding a new line
// to the end.
//
// Notes:
// - Only exported fields of the data structure will be printed.
// - Message isn't processed.
func (sypl *Sypl) PrintlnPretty(l level.Level, data interface{}) ISypl {
	msg := message.New(l, fmt.Sprintln(shared.Prettify(data)))
	msg.SetFlag(flag.Skip)

	return sypl.PrintMessage(msg)
}

// PrintMessagesToOutputs allows you to concurrently print messages, each one,
// at the specified level and to the specified output.
//
// NOTE: If the named output doesn't exits, the message will not be printed.
func (sypl *Sypl) PrintMessagesToOutputs(messagesToOutputs ...MessageToOutput) ISypl {
	messages := []message.IMessage{}

	for _, mto := range messagesToOutputs {
		m := message.New(mto.Level, mto.Content)
		m.SetOutputsNames([]string{mto.OutputName})

		messages = append(messages, m)
	}

	sypl.process(messages...)

	return sypl
}

// PrintMessagesToOutputsWithOptions allows you to concurrently print messages,
// each one, at the specified level and to the specified output, with options.
//
// NOTE: If the named output doesn't exits, the message will not be printed.
func (sypl *Sypl) PrintMessagesToOutputsWithOptions(
	o *options.Options,
	messagesToOutputs ...MessageToOutput,
) ISypl {
	messages := []message.IMessage{}

	for _, mto := range messagesToOutputs {
		m := message.New(mto.Level, mto.Content)
		m.SetOutputsNames([]string{mto.OutputName})

		messages = append(messages, mergeOptions(m, o))
	}

	sypl.process(messages...)

	return sypl
}

// PrintNewLine prints a new line. It always print, independent of the level,
// and without any processing.
func (sypl *Sypl) PrintNewLine() ISypl {
	m := message.New(level.Info, "\n")
	m.SetFlag(flag.SkipAndForce)

	sypl.process(m)

	return sypl
}

//////
// ILeveledPrinter interface implementation.
//////

// Fatal prints, and exit with os.Exit(1).
func (sypl *Sypl) Fatal(args ...interface{}) ISypl {
	return sypl.Print(level.Fatal, args...)
}

// Fatalf prints according with the format, and exit with os.Exit(1).
func (sypl *Sypl) Fatalf(format string, args ...interface{}) ISypl {
	return sypl.Printf(level.Fatal, format, args...)
}

// Fatallnf prints according with the format, also adding a new line to the
// end, and exit with os.Exit(1).
func (sypl *Sypl) Fatallnf(format string, args ...interface{}) ISypl {
	return sypl.Printlnf(level.Fatal, format, args...)
}

// Fatalln prints, also adding a new line and the end, and exit with os.Exit(1).
func (sypl *Sypl) Fatalln(args ...interface{}) ISypl {
	return sypl.Println(level.Fatal, args...)
}

// Error prints @ the Error level.
func (sypl *Sypl) Error(args ...interface{}) ISypl {
	return sypl.Print(level.Error, args...)
}

// Errorf prints according with the format @ the Error level.
func (sypl *Sypl) Errorf(format string, args ...interface{}) ISypl {
	return sypl.Printf(level.Error, format, args...)
}

// Errorlnf prints according with the format @ the Error level, also adding
// a new line to the end.
func (sypl *Sypl) Errorlnf(format string, args ...interface{}) ISypl {
	return sypl.Printlnf(level.Error, format, args...)
}

// Errorln prints, also adding a new line to the end @ the Error level.
func (sypl *Sypl) Errorln(args ...interface{}) ISypl {
	return sypl.Println(level.Error, args...)
}

// Serror prints like Error, and returns an error with the non-processed
// content.
func (sypl *Sypl) Serror(args ...interface{}) error {
	sypl.Print(level.Error, args...)

	return errors.New(fmt.Sprint(args...))
}

// Serrorf prints like Errorf, and returns an error with the non-processed
// content.
func (sypl *Sypl) Serrorf(format string, args ...interface{}) error {
	sypl.Printf(level.Error, format, args...)

	return fmt.Errorf(format, args...)
}

// Serrorlnf prints like Errorlnf, and returns an error with the
// non-processed content.
func (sypl *Sypl) Serrorlnf(format string, args ...interface{}) error {
	sypl.Printlnf(level.Error, format, args...)

	return fmt.Errorf(format+"\n", args...)
}

// Serrorln prints like Errorln, and returns an error with the non-processed
// content.
func (sypl *Sypl) Serrorln(args ...interface{}) error {
	sypl.Println(level.Error, args...)

	return errors.New(fmt.Sprintln(args...))
}

// Info prints @ the Info level.
func (sypl *Sypl) Info(args ...interface{}) ISypl {
	return sypl.Print(level.Info, args...)
}

// Infof prints according with the specified format @ the Info level.
func (sypl *Sypl) Infof(format string, args ...interface{}) ISypl {
	return sypl.Printf(level.Info, format, args...)
}

// Infolnf prints according with the specified format @ the Info level, also
// adding a new line to the end.
func (sypl *Sypl) Infolnf(format string, args ...interface{}) ISypl {
	return sypl.Printlnf(level.Info, format, args...)
}

// Infoln prints, also adding a new line to the end @ the Info level.
func (sypl *Sypl) Infoln(args ...interface{}) ISypl {
	return sypl.Println(level.Info, args...)
}

// Warn prints @ the Warn level.
func (sypl *Sypl) Warn(args ...interface{}) ISypl {
	return sypl.Print(level.Warn, args...)
}

// Warnf prints according with the specified format @ the Warn level.
func (sypl *Sypl) Warnf(format string, args ...interface{}) ISypl {
	return sypl.Printf(level.Warn, format, args...)
}

// Warnlnf prints according with the specified format @ the Warn level, also
// adding a new line to the end.
func (sypl *Sypl) Warnlnf(format string, args ...interface{}) ISypl {
	return sypl.Printlnf(level.Warn, format, args...)
}

// Warnln prints, also adding a new line to the end @ the Warn level.
func (sypl *Sypl) Warnln(args ...interface{}) ISypl {
	return sypl.Println(level.Warn, args...)
}

// Debug prints @ the Debug level.
func (sypl *Sypl) Debug(args ...interface{}) ISypl {
	return sypl.Print(level.Debug, args...)
}

// Debugf prints according with the specified format @ the Debug level.
func (sypl *Sypl) Debugf(format string, args ...interface{}) ISypl {
	return sypl.Printf(level.Debug, format, args...)
}

// Debuglnf prints according with the specified format @ the Debug level,
// also adding a new line to the end.
func (sypl *Sypl) Debuglnf(format string, args ...interface{}) ISypl {
	return sypl.Printlnf(level.Debug, format, args...)
}

// Debugln prints, also adding a new line to the end @ the Debug level.
func (sypl *Sypl) Debugln(args ...interface{}) ISypl {
	return sypl.Println(level.Debug, args...)
}

// Trace prints @ the Trace level.
func (sypl *Sypl) Trace(args ...interface{}) ISypl {
	return sypl.Print(level.Trace, args...)
}

// Tracef prints according with the specified format @ the Trace level.
func (sypl *Sypl) Tracef(format string, args ...interface{}) ISypl {
	return sypl.Printf(level.Trace, format, args...)
}

// Tracelnf prints according with the specified format @ the Trace level,
// also adding a new line to the end.
func (sypl *Sypl) Tracelnf(format string, args ...interface{}) ISypl {
	return sypl.Printlnf(level.Trace, format, args...)
}

// Traceln prints, also adding a new line to the end @ the Trace level.
func (sypl *Sypl) Traceln(args ...interface{}) ISypl {
	return sypl.Println(level.Trace, args...)
}

//////
// ISypl interface implementation.
//////

// Breakpoint will stop execution waiting the user press "/n" ("enter") to
// continue. It helps users doing log-to-console debug strategy. A message
// with the breakpoint `name`, and PID of the process will be printed using
// the `debug` level. Arbitrary `data` can optionally be set - if set, it'll
// be printed. Errors are printed using the `error` level. Set logging level
// to `trace` for more.
func (sypl *Sypl) Breakpoint(name string, data ...interface{}) ISypl {
	breakpointName := fmt.Sprintf(`Breakpoint: %s. PID: %d`, name, os.Getpid())

	if data != nil {
		breakpointName = fmt.Sprintf("%s. Data:", breakpointName)

		for _, d := range data {
			breakpointName = fmt.Sprintf("%s %+v,", breakpointName, d)
		}

		breakpointName = strings.TrimSuffix(breakpointName, ",")
	}

	sypl.PrintWithOptions(
		level.Debug,
		fmt.Sprintf("%s. Press enter to continue...", breakpointName),
		WithFlag(flag.Force),
	)

	reader := bufio.NewReader(os.Stdin)

	if _, err := reader.ReadString('\n'); err != nil {
		sypl.Errorlnf("%s. Failed to read input: %s", breakpointName, err)
	} else {
		sypl.Traceln("Resuming")
	}

	return sypl
}

// GetFields returns the global structured fields.
func (sypl *Sypl) GetFields() fields.Fields {
	return sypl.fields
}

// SetFields sets the global structured fields.
func (sypl *Sypl) SetFields(fields fields.Fields) ISypl {
	sypl.fields = fields

	return sypl
}

// GetTags returns the global tags.
func (sypl *Sypl) GetTags() []string {
	return sypl.tags
}

// SetTags adds the global tags.
func (sypl *Sypl) SetTags(tags ...string) ISypl {
	sypl.tags = append(sypl.tags, tags...)

	return sypl
}

// GetMaxLevel returns the `maxLevel` of all outputs.
func (sypl *Sypl) GetMaxLevel() map[string]level.Level {
	levelMap := map[string]level.Level{}

	for _, output := range sypl.GetOutputs() {
		levelMap[output.GetName()] = output.GetMaxLevel()
	}

	return levelMap
}

// AnyMaxLevel returns if any output has the specified `maxLevel`.
func (sypl *Sypl) AnyMaxLevel(l level.Level) bool {
	// Check level when set when output is created.
	for _, output := range sypl.GetOutputs() {
		if output.GetMaxLevel() == l {
			return true
		}
	}

	// Check level when set output `maxLevel` is modified after creation,
	// real-time, runtime.
	return os.Getenv(shared.LevelEnvVar) == l.String()
}

// SetMaxLevel sets the `maxLevel` of all outputs.
func (sypl *Sypl) SetMaxLevel(l level.Level) ISypl {
	for _, output := range sypl.GetOutputs() {
		output.SetMaxLevel(l)
	}

	return sypl
}

// AddOutputs adds one or more outputs.
func (sypl *Sypl) AddOutputs(outputs ...output.IOutput) ISypl {
	sypl.outputs = append(sypl.outputs, outputs...)

	return sypl
}

// GetOutput returns the registered output by its name. If not found, will be
// nil.
func (sypl *Sypl) GetOutput(name string) output.IOutput {
	for _, o := range sypl.outputs {
		if strings.EqualFold(o.GetName(), name) {
			return o
		}
	}

	return nil
}

// SetOutputs sets one or more outputs. Use to update output(s).
func (sypl *Sypl) SetOutputs(outputs ...output.IOutput) ISypl {
	for _, output := range outputs {
		for i, o := range sypl.outputs {
			if strings.EqualFold(o.GetName(), output.GetName()) {
				sypl.outputs[i] = output
			}
		}
	}

	return sypl
}

// GetOutputs returns registered outputs.
func (sypl *Sypl) GetOutputs() []output.IOutput {
	return sypl.outputs
}

// GetOutputsNames returns the names of the registered outputs.
func (sypl *Sypl) GetOutputsNames() []string {
	outputsNames := []string{}

	for _, output := range sypl.outputs {
		outputsNames = append(outputsNames, output.GetName())
	}

	return outputsNames
}

// New creates a child logger. The child logger is an accurate, efficient and
// shallow copy of the parent logger. Changes to internals, such as the state of
// outputs, and processors, are reflected cross all other loggers.
func (sypl *Sypl) New(name string) *Sypl {
	s := New(name, sypl.outputs...)

	s.defaultIoWriterLevel = sypl.defaultIoWriterLevel
	s.fields = sypl.fields
	s.status = sypl.status
	s.tags = sypl.tags

	return s
}

// Process messages, per output, and process accordingly.
func (sypl *Sypl) process(messages ...message.IMessage) {
	if sypl == nil {
		log.Fatalf("%s %s", shared.ErrorPrefix, ErrSyplNotInitialized)
	}

	shouldExit := false

	g := new(errgroup.Group)

	for _, m := range messages {
		// https://golang.org/doc/faq#closures_and_goroutines
		m := m

		g.Go(func() error {
			// Do nothing if message as no content, or flagged with `SkipAndMute`.
			if m.GetContent().GetOriginal() == "" &&
				m.GetFlag() == flag.SkipAndMute {
				return nil
			}

			// Should allows to filter logging by components names.
			syplFilterEnvVar := os.Getenv(shared.FilterEnvVar)

			if syplFilterEnvVar != "" &&
				!strings.Contains(syplFilterEnvVar, sypl.GetName()) {
				return nil
			}

			// Should allows to specify `Output`(s).
			outputsNames := sypl.GetOutputsNames()

			if len(m.GetOutputsNames()) > 0 {
				outputsNames = m.GetOutputsNames()
			}

			m.SetOutputsNames(outputsNames)

			// Should allows to set global fields.
			// Per-message fields should have precedence.
			if sypl.GetFields() != nil {
				finalFields := fields.Fields{}
				finalFields = fields.Copy(sypl.GetFields(), finalFields)
				finalFields = fields.Copy(m.GetFields(), finalFields)
				m.SetFields(finalFields)
			}

			// Should allows to set global tags.
			// Per-message tags should have precedence.
			if sypl.GetTags() != nil {
				finalTags := []string{}
				finalTags = append(finalTags, sypl.GetTags()...)
				finalTags = append(finalTags, m.GetTags()...)
				m.AddTags(finalTags...)
			}

			sypl.processOutputs(m, strings.Join(outputsNames, ","))

			if m.GetLevel() == level.Fatal {
				shouldExit = true
			}

			return nil
		})
	}

	_ = g.Wait()

	// Should exit if `level` is `Fatal`.
	if shouldExit {
		os.Exit(1)
	}
}

//////
// Helpers.
//////

// Merge options into message.
//
// Notes:
// - Changes in the `Message` or `Options` data structure may reflects here.
// - Could use something like the `Copier` package, but that's going to cause a
// data race, because `Output`s are processed concurrently.
func mergeOptions(m message.IMessage, o *options.Options) message.IMessage {
	if o.Fields != nil {
		m.SetFields(o.Fields)
	}

	if o.Flag != flag.None {
		m.SetFlag(o.Flag)
	}

	if len(o.OutputsNames) > 0 {
		m.SetOutputsNames(o.OutputsNames)
	}

	if len(o.ProcessorsNames) > 0 {
		m.SetProcessorsNames(o.ProcessorsNames)
	}

	if len(o.Tags) > 0 {
		// Merge `options.Tags`.
		m.GetMessage().Tags = o.Tags

		// Adds tags to `message.tags`.
		m.AddTags(o.Tags...)
	}

	return m
}

// Outputs logic of the Process method.
func (sypl *Sypl) processOutputs(m message.IMessage, outputsNames string) {
	g := new(errgroup.Group)

	for _, o := range sypl.outputs {
		// https://golang.org/doc/faq#closures_and_goroutines
		o := o
		m := m

		// Message is isolated per `Output`.
		msg := message.Copy(m)

		// Should only use enabled Outputs, and named (listed) ones.
		if o.GetStatus() == status.Enabled && strings.Contains(outputsNames, o.GetName()) {
			msg.SetComponentName(sypl.GetName())
			msg.SetOutputName(o.GetName())

			// Debug capability.
			// Should only run if Debug env var is set.
			if os.Getenv(shared.LevelEnvVar) != "" {
				msg.SetDebugEnvVarRegexes(
					debug.New(msg.GetComponentName(), msg.GetOutputName()),
				)
			}

			g.Go(func() error {
				return o.Write(msg)
			})
		}
	}

	_ = g.Wait()
}

//////
// Factory.
//////

// New is the Sypl factory.
func New(name string, outputs ...output.IOutput) *Sypl {
	s := &Sypl{
		Name: name,

		defaultIoWriterLevel: level.None,
		fields:               fields.Fields{},
		outputs:              outputs,
		status:               status.Enabled,
		tags:                 []string{},
	}

	return s
}

// NewDefault creates a logger that covers most of all needs:
// - Writes message to `stdout` @ the specified `maxLevel`
// - Writes error messages only to `stderr`
// - Default io.Writer level is `none`. Explicitly change that using
// `SetDefaultIoWriterLevel` to suit your need.
//
// NOTE: `processors` are applied to both outputs.
func NewDefault(name string, maxLevel level.Level, processors ...processor.IProcessor) *Sypl {
	consoleProcessors := processors
	consoleProcessors = append(consoleProcessors, processor.MuteBasedOnLevel(level.Fatal, level.Error))

	return New(name, []output.IOutput{
		output.Console(maxLevel, consoleProcessors...).SetFormatter(formatter.Text()),
		output.StdErr(processors...).SetFormatter(formatter.Text()),
	}...)
}
