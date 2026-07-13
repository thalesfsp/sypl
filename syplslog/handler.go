// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package syplslog

import (
	"context"
	"log/slog"
	"strings"

	"github.com/thalesfsp/sypl"
	"github.com/thalesfsp/sypl/fields"
	"github.com/thalesfsp/sypl/message"
	"github.com/thalesfsp/sypl/status"
)

// groupKeySeparator joins group names, and attr keys into flattened field
// keys, e.g.: "group.key".
const groupKeySeparator = "."

// field is a resolved, flattened - "group.key"-qualified - attribute.
type field struct {
	// key is the group-qualified attribute key.
	key string

	// value is the resolved attribute value, as a native Go value.
	value interface{}
}

// Handler is a `slog.Handler` forwarding records to a sypl logger. See
// `NewHandler`.
type Handler struct {
	// leveler is the optional minimum slog level (floor). `nil` means no
	// floor - `Enabled` is then governed by the sypl outputs alone.
	leveler slog.Leveler

	// logger records are forwarded to.
	logger *sypl.Sypl

	// attrs accumulated via `WithAttrs` - already resolved, and flattened.
	attrs []field

	// groups opened via `WithGroup`.
	groups []string
}

// HandlerOption allows to specify optional `Handler` configuration.
type HandlerOption func(*Handler)

// HandlerWithLevel sets the minimum (floor) slog level: records below it are
// discarded before reaching sypl. Without it, `Enabled` is governed by the
// sypl outputs alone.
func HandlerWithLevel(leveler slog.Leveler) HandlerOption {
	return func(h *Handler) {
		h.leveler = leveler
	}
}

// Enabled reports whether the handler handles records at the given level:
// the level must be at, or above the optional floor - see
// `HandlerWithLevel` - and at least one ENABLED sypl output must accept the
// mapped sypl level.
//
// NOTE: The `SYPL_DEBUG` env var - which overrides output max levels at
// write time - isn't consulted here.
func (h *Handler) Enabled(_ context.Context, l slog.Level) bool {
	if h.leveler != nil && l < h.leveler.Level() {
		return false
	}

	syplLevel := ToSyplLevel(l)

	for _, o := range h.logger.GetOutputs() {
		if o.GetStatus() == status.Enabled && syplLevel <= o.GetMaxLevel() {
			return true
		}
	}

	return false
}

// Handle converts the record into a sypl message, and prints it through the
// sypl logger:
//   - The record message becomes the content. Each record is a single log
//     line: a trailing linebreak is ensured - sypl strips it for processing,
//     and restores it on write.
//   - The record level is mapped per `ToSyplLevel`.
//   - The record time is honored - a zero `Record.Time` is forwarded as the
//     zero timestamp, meaning "no time".
//   - Record attrs, and `WithAttrs`-accumulated attrs become fields:
//     `LogValuer`s are resolved - a panicking `LogValue` is recovered by
//     `slog.Value.Resolve`, yielding an error value describing the panic -
//     and groups are flattened as "group.key".
//   - The record PC (source) is not forwarded.
func (h *Handler) Handle(_ context.Context, r slog.Record) error {
	content := r.Message

	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}

	m := message.New(ToSyplLevel(r.Level), content)

	m.SetTimestamp(r.Time)

	flattened := make([]field, 0, len(h.attrs)+r.NumAttrs())

	flattened = append(flattened, h.attrs...)

	prefix := strings.Join(h.groups, groupKeySeparator)

	r.Attrs(func(a slog.Attr) bool {
		flattened = appendAttr(flattened, prefix, a)

		return true
	})

	if len(flattened) > 0 {
		f := fields.Fields{}

		for _, fld := range flattened {
			f[fld.key] = fld.value
		}

		m.SetFields(f)
	}

	h.logger.PrintMessage(m)

	return nil
}

// WithAttrs returns a NEW handler whose attributes consist of both the
// receiver's attributes, and the arguments - resolved, and flattened under
// the currently open groups. The receiver is not modified.
func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}

	h2 := h.clone(len(attrs), 0)

	prefix := strings.Join(h2.groups, groupKeySeparator)

	for _, a := range attrs {
		h2.attrs = appendAttr(h2.attrs, prefix, a)
	}

	return h2
}

// WithGroup returns a NEW handler with the given group appended to the
// receiver's existing groups: subsequent attribute keys are qualified as
// "group.key". If the name is empty, the receiver is returned. The receiver
// is not modified.
func (h *Handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}

	h2 := h.clone(0, 1)

	h2.groups = append(h2.groups, name)

	return h2
}

// clone derives a new handler with FRESH backing arrays - sized for the
// upcoming appends - so sibling derivations never clobber each other.
func (h *Handler) clone(extraAttrs, extraGroups int) *Handler {
	h2 := &Handler{
		leveler: h.leveler,
		logger:  h.logger,
		attrs:   make([]field, len(h.attrs), len(h.attrs)+extraAttrs),
		groups:  make([]string, len(h.groups), len(h.groups)+extraGroups),
	}

	copy(h2.attrs, h.attrs)
	copy(h2.groups, h.groups)

	return h2
}

// appendAttr resolves `a`, flattens groups - "group.key", empty-named groups
// are inlined, attr-less groups are elided - ignores zero attrs, and appends
// the result to `dst`.
func appendAttr(dst []field, prefix string, a slog.Attr) []field {
	// Resolve handles `LogValuer`s - including panic recovery: a panicking
	// `LogValue` yields an error value describing the panic.
	v := a.Value.Resolve()

	if v.Kind() == slog.KindGroup {
		attrs := v.Group()

		// A group with no attributes is elided - even if it has a name.
		if len(attrs) == 0 {
			return dst
		}

		// A group with an empty key inlines its attributes.
		p := prefix

		if a.Key != "" {
			p = joinKey(prefix, a.Key)
		}

		for _, ga := range attrs {
			dst = appendAttr(dst, p, ga)
		}

		return dst
	}

	// An attr with both a zero key, and a zero value is ignored.
	if a.Key == "" && v.Any() == nil {
		return dst
	}

	return append(dst, field{key: joinKey(prefix, a.Key), value: v.Any()})
}

// joinKey qualifies `key` with `prefix`, if any.
func joinKey(prefix, key string) string {
	if prefix == "" {
		return key
	}

	return prefix + groupKeySeparator + key
}

// NewHandler is the `Handler` factory: a `slog.Handler` forwarding records to
// the given sypl logger - see `Handler.Handle` for the conversion rules, and
// the package documentation for the level mapping.
func NewHandler(l *sypl.Sypl, opts ...HandlerOption) slog.Handler {
	h := &Handler{logger: l}

	for _, opt := range opts {
		opt(h)
	}

	return h
}
