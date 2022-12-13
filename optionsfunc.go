package sypl

import (
	"github.com/thalesfsp/sypl/fields"
	"github.com/thalesfsp/sypl/flag"
	"github.com/thalesfsp/sypl/message"
)

// OptionFunc allows to specify message's options.
type OptionFunc func(m message.IMessage) message.IMessage

// WithID set message's ID.
func WithID(id string) OptionFunc {
	return func(m message.IMessage) message.IMessage {
		m.SetID(id)

		return m
	}
}

// WithTags add tags to a message.
func WithTags(tags ...string) OptionFunc {
	return func(m message.IMessage) message.IMessage {
		m.AddTags(tags...)

		return m
	}
}

// WithFields add fields to a message.
func WithFields(fields fields.Fields) OptionFunc {
	return func(m message.IMessage) message.IMessage {
		m.SetFields(fields)

		return m
	}
}

// WithFlag set message's flag.
func WithFlag(f flag.Flag) OptionFunc {
	return func(m message.IMessage) message.IMessage {
		m.SetFlag(f)

		return m
	}
}

// WithOutputsNames set message's output names.
func WithOutputsNames(outputsNames ...string) OptionFunc {
	return func(m message.IMessage) message.IMessage {
		m.SetOutputsNames(outputsNames)

		return m
	}
}

// WithProcessorsNames set message's processors names.
func WithProcessorsNames(processorsNames ...string) OptionFunc {
	return func(m message.IMessage) message.IMessage {
		m.SetProcessorsNames(processorsNames)

		return m
	}
}
