// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package message

import (
	"sort"
	"strings"
	"time"

	"github.com/thalesfsp/sypl/v2/content"
	"github.com/thalesfsp/sypl/v2/debug"
	"github.com/thalesfsp/sypl/v2/fields"
	"github.com/thalesfsp/sypl/v2/flag"
	"github.com/thalesfsp/sypl/v2/level"
	"github.com/thalesfsp/sypl/v2/options"
	"github.com/thalesfsp/sypl/v2/status"
)

// LineBreaker defines if `Content` needs linebreak strip/restoration.
//
// Context: When a message enters the pipeline, line breakers are
// removed (see: `output.Write`). Content is then processed and only, at the
// final stage - before printing, the line break is restored, if needed.
// (see: `output.write`).
type lineBreaker struct {
	// ControlChars accumulates stripped control chars.
	ControlChars []string

	// KnownLineBreakers is a list known linebreakers.
	KnownLineBreakers []string

	// Status indicates whether message had control chars stripped, or not.
	Status status.Status
}

// newLineBreaker is the lineBreaker factory.
func newLineBreaker(knownLineBreakers ...string) *lineBreaker {
	return &lineBreaker{
		ControlChars:      []string{},
		KnownLineBreakers: knownLineBreakers,
		Status:            status.Enabled,
	}
}

// Message envelops the content and contains meta-information about it.
//
// NOTE: Changes in the `Message` or `Options` data structure may trigger
// changes in the `New`, `Copy`, `mergeOptions` (from `Sypl`), `New` (from
// `Options`) methods, and the formatters.
type message struct {
	*options.Options

	// Name of the component logging the message.
	componentName string

	// Message's linebreaker. See `lineBreaker` for more information.
	lineBreaker *lineBreaker `json:"-"`

	// tags are indicators consumed by `Output`s and `Processor`s.
	tags map[string]struct{}

	// Debug capabilities.
	debug *debug.Debug

	// A randomly generated UUIDv4 that uniquely identifies the message -
	// computed lazily, and memoized on first `GetID` call. `SetID` pins it.
	id *lazyString

	// A hash of the message's content - computed lazily, and memoized on
	// first `GetContentBasedHashID` call. `SetContentBasedHashID` pins it.
	contentBasedHashID *lazyString

	// Content that should be written to `Output`.
	Content content.IContent

	// Message's level.
	Level level.Level

	// Output in use.
	OutputName string `json:"-"`

	// Processor in use.
	ProcessorName string `json:"-"`

	// The point in time when the message was created.
	Timestamp time.Time
}

// String interface implementation.
func (m message) String() string {
	return m.Content.GetProcessed()
}

//////
// ITag interface implementation.
//////

// AddTags adds one or more tags.
func (m *message) AddTags(tags ...string) {
	for _, tag := range tags {
		m.tags[tag] = struct{}{}
	}
}

// ContainTag verifies if tags contains the specified tag.
func (m *message) ContainTag(tag string) bool {
	_, ok := m.tags[tag]

	return ok
}

// DeleteTag deletes a tag.
func (m *message) DeleteTag(tag string) {
	delete(m.tags, tag)
}

// GetTags retrieves tags, lexicographically sorted.
func (m *message) GetTags() []string {
	tags := []string{}

	for tag := range m.tags {
		tags = append(tags, tag)
	}

	sort.Strings(tags)

	return tags
}

//////
// ILineBreaker interface implementation.
//////

// getLineBreaker returns linebreaker.
func (m *message) getLineBreaker() *lineBreaker {
	return m.lineBreaker
}

// setLineBreaker sets the line break status.
func (m *message) setLineBreaker(lB *lineBreaker) IMessage {
	m.lineBreaker = lB

	return m
}

// Restore known linebreaks.
func (m *message) Restore() {
	if m.getLineBreaker().Status == status.Enabled {
		controlChars := m.getLineBreaker().ControlChars

		// Control chars are re-appended in the reverse order they were
		// stripped, restoring the original sequence (e.g.: "\r\n").
		for i := len(controlChars) - 1; i >= 0; i-- {
			m.GetContent().SetProcessed(m.GetContent().GetProcessed() + controlChars[i])
		}
	}
}

// Detects (cross-OS) and removes any newline/line-break, at the end of the
// content, ensuring text processing is done properly (e.g.: suffix).
func (m *message) Strip() {
	if m.getLineBreaker().Status == status.Enabled {
		for _, knownLineBreaker := range m.getLineBreaker().KnownLineBreakers {
			if strings.HasSuffix(m.GetContent().GetProcessed(), knownLineBreaker) {
				m.GetContent().SetProcessed(
					strings.TrimSuffix(m.GetContent().GetProcessed(), knownLineBreaker),
				)

				m.getLineBreaker().ControlChars = append(m.getLineBreaker().ControlChars, knownLineBreaker)

				m.Strip()
			}
		}
	}
}

//////
// IMessage interface implementation.
//////

// GetComponentName returns the component name.
func (m *message) GetComponentName() string {
	return m.componentName
}

// SetComponentName sets the component name.
func (m *message) SetComponentName(name string) IMessage {
	m.componentName = name

	return m
}

// GetContent returns the content.
func (m *message) GetContent() content.IContent {
	return m.Content
}

// SetContentBasedHashID sets a hash of the message's content - pinning it,
// no generation will run.
func (m *message) SetContentBasedHashID(hash string) IMessage {
	m.contentBasedHashID = resolvedLazyString(hash)

	return m
}

// GetContentBasedHashID returns the hash of the message's content - computed
// lazily, and memoized on first call.
func (m *message) GetContentBasedHashID() string {
	return m.contentBasedHashID.get()
}

// GetDebugEnvVarRegexeses returns the Debug env var regexes matchers.
func (m *message) GetDebugEnvVarRegexes() *debug.Debug {
	return m.debug
}

// SetDebugEnvVarRegexeses sets the Debug env var regexes matchers.
func (m *message) SetDebugEnvVarRegexes(d *debug.Debug) *message {
	m.debug = d

	return m
}

// GetFields returns the structured fields.
func (m *message) GetFields() fields.Fields {
	return m.Fields
}

// SetFields sets the structured fields.
func (m *message) SetFields(fields fields.Fields) IMessage {
	m.Fields = fields

	return m
}

// GetFlag returns the flag.
func (m *message) GetFlag() flag.Flag {
	return m.Flag
}

// SetFlag sets the flag.
func (m *message) SetFlag(flag flag.Flag) IMessage {
	m.Flag = flag

	return m
}

// GetID returns the id - computed lazily, and memoized on first call.
func (m *message) GetID() string {
	return m.id.get()
}

// SetID sets the id - pinning it, no generation will run.
func (m *message) SetID(id string) {
	m.id = resolvedLazyString(id)
}

// GetLevel returns the level.
func (m *message) GetLevel() level.Level {
	return m.Level
}

// GetMessage (low-level) returns the message.
func (m *message) GetMessage() *message {
	return m
}

// GetOutputName returns the name of the output in use.
func (m *message) GetOutputName() string {
	return m.OutputName
}

// SetOutputName sets the name of the output in use.
func (m *message) SetOutputName(outputName string) IMessage {
	m.OutputName = outputName

	return m
}

// GetOutputsNames returns the outputs names that should be used.
func (m *message) GetOutputsNames() []string {
	return m.OutputsNames
}

// SetOutputsNames sets the outputs names that should be used.
func (m *message) SetOutputsNames(outputsNames []string) IMessage {
	m.OutputsNames = outputsNames

	return m
}

// GetProcessorName returns the name of the processor in use.
func (m *message) GetProcessorName() string {
	return m.ProcessorName
}

// SetProcessorName sets the name of the processor in use.
func (m *message) SetProcessorName(processorName string) IMessage {
	m.ProcessorName = processorName

	return m
}

// GetProcessorsNames returns the processors names that should be used.
func (m *message) GetProcessorsNames() []string {
	return m.ProcessorsNames
}

// SetProcessorsNames sets the processors names that should be used.
func (m *message) SetProcessorsNames(processorsNames []string) IMessage {
	m.ProcessorsNames = processorsNames

	return m
}

// GetTimestamp returns the timestamp.
func (m *message) GetTimestamp() time.Time {
	return m.Timestamp
}

// SetTimestamp sets the timestamp.
func (m *message) SetTimestamp(timestamp time.Time) IMessage {
	m.Timestamp = timestamp

	return m
}

// IsEmpty returns true if the message is empty.
func (m *message) IsEmpty() bool {
	return strings.Trim(m.GetContent().GetOriginal(), "\f\t\r\n ") == ""
}

//////
// Helpers.
//////

// Copy message.
//
// Notes:
// - Changes in the `Message` or `Options` data structure may reflects here.
// Should reflect in the formatters too.
// - Could use something like the `Copier` package, but that's going to cause a
// data race, because `Output`s are processed concurrently.
//
// TODO: This can be improved.
func Copy(m IMessage) IMessage {
	// ID, content-based hash, and timestamp generation are skipped -
	// they are copied from the source message below.
	msg := newMessage(m.GetLevel(), m.GetContent().GetOriginal())

	// Copy `options.Tags`. Should be a real copy, not slice aliasing.
	if mTags := m.GetMessage().Tags; mTags != nil {
		tags := make([]string, len(mTags))
		copy(tags, mTags)

		msg.GetMessage().Tags = tags
	}

	// Identity is SHARED, not forced: source, and copy point at the same
	// lazy cells, so the UUID, and content hash are computed at most once
	// per message family - by whoever reads first - and every member
	// observes the same values. `SetID`/`SetContentBasedHashID` on any
	// member replaces only that member's cell (snapshot semantics).
	msg.id = m.GetMessage().id
	msg.contentBasedHashID = m.GetMessage().contentBasedHashID

	// Adds tags to `message.tags`.
	msg.AddTags(m.GetTags()...)

	msg.SetComponentName(m.GetComponentName())
	msg.SetDebugEnvVarRegexes(m.GetDebugEnvVarRegexes())

	// Fields should be deep copied - per-output copies are processed
	// concurrently.
	msg.SetFields(fields.Copy(m.GetFields(), fields.Fields{}))
	msg.SetFlag(m.GetFlag())

	gLB := *m.getLineBreaker()
	msg.setLineBreaker(&gLB)

	msg.SetOutputName(m.GetOutputName())
	msg.SetOutputsNames(m.GetOutputsNames())
	msg.SetProcessorName(m.GetProcessorName())
	msg.SetProcessorsNames(m.GetProcessorsNames())
	msg.SetTimestamp(m.GetTimestamp())

	return msg
}

//////
// Factory.
//////

// newMessage creates a bare message, skipping ID, content-based hash, and
// timestamp generation. Used by `Copy` - which overwrites them anyway.
func newMessage(l level.Level, ct string) *message {
	// NOTE: The `id`, and `contentBasedHashID` lazy cells are NOT allocated
	// here - BOTH construction sites assign them: `New` installs lazy
	// generators, and `Copy` shares the source's cells. A message never
	// leaves this package without them.
	return &message{
		Options: options.New(),

		Content:     content.New(ct),
		Level:       l,
		lineBreaker: newLineBreaker("\n", "\r"),
		tags:        map[string]struct{}{},
	}
}

// New is the Message factory.
//
// NOTE: Changes in the `Message` or `Options` data structure may reflects here.
func New(l level.Level, ct string) IMessage {
	m := newMessage(l, ct)

	// The UUID (crypto/rand), and the content hash (SHA-1) are EXPENSIVE
	// relative to the rest of the hot path, and most consumers (e.g. the
	// Text formatter) never read them - so they are computed lazily, and
	// memoized on the first `GetID`/`GetContentBasedHashID` call.
	m.contentBasedHashID = newLazyString(func() string { return generateID(ct) })
	m.id = newLazyString(generateUUID)
	m.Timestamp = time.Now()

	return m
}
