// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package syplslog

import (
	"context"
	"log/slog"
	"sort"
	"strings"

	"github.com/thalesfsp/sypl/v2/level"
	"github.com/thalesfsp/sypl/v2/message"
	"github.com/thalesfsp/sypl/v2/output"
	"github.com/thalesfsp/sypl/v2/processor"
)

// forwardedMark marks content the bridge formatter already forwarded to
// slog, so the output's writer doesn't forward it twice.
const forwardedMark = "\x00syplslog:forwarded\x00"

// slogWriter is the `io.Writer` backing the bridge output. It only forwards
// content the bridge formatter did NOT handle: messages flagged `Skip`, or
// `SkipAndForce` bypass processing, and formatting - by design - so no
// level, nor fields are available; their raw content is forwarded @ the
// `slog.LevelInfo` level, trailing linebreaks trimmed.
type slogWriter struct {
	// sl is the logger raw content is forwarded to.
	sl *slog.Logger
}

// Write implements `io.Writer`.
func (w *slogWriter) Write(p []byte) (int, error) {
	s := string(p)

	// Content already forwarded - structured - by the bridge formatter.
	if strings.HasPrefix(s, forwardedMark) {
		return len(p), nil
	}

	w.sl.Log(context.Background(), slog.LevelInfo, strings.TrimRight(s, "\r\n"))

	return len(p), nil
}

// newForwarder is the bridge formatter factory. Running as the output's
// formatter - the last stage before the write, AFTER standard dispatch
// gating (max level, `Force`, `Mute`) - it converts the processed message
// into a `slog.Record`: content as message, level mapped per `ToSlogLevel`,
// timestamp honored, and fields as attrs - sorted by key, for determinism.
// The content is then marked, so the output's writer drops it.
func newForwarder(sl *slog.Logger) processor.IProcessor {
	return processor.New("SlogForwarder", func(m message.IMessage) error {
		ctx := context.Background()

		l := ToSlogLevel(m.GetLevel())

		content := m.GetContent().GetProcessed()

		m.GetContent().SetProcessed(forwardedMark)

		handler := sl.Handler()

		// The slog side may filter further.
		if !handler.Enabled(ctx, l) {
			return nil
		}

		r := slog.NewRecord(m.GetTimestamp(), l, content, 0)

		f := m.GetFields()

		if len(f) > 0 {
			keys := make([]string, 0, len(f))

			for k := range f {
				keys = append(keys, k)
			}

			sort.Strings(keys)

			for _, k := range keys {
				r.AddAttrs(slog.Any(k, f[k]))
			}
		}

		return handler.Handle(ctx, r)
	})
}

// Output is a built-in `output` factory: a sypl output forwarding processed
// messages to the given `*slog.Logger` - fields as attrs, and the sypl level
// mapped back to a slog level, per `ToSlogLevel`. It's built with
// `output.New`, so dispatch, and processor semantics are standard: max-level
// gating, `Force`, `Mute`, and `processors` all behave as in any other
// output.
//
// Notes:
//   - The forwarding is implemented as the output's FORMATTER: do NOT
//     replace it via `SetFormatter` - doing so breaks the bridge.
//   - Messages flagged `Skip`, or `SkipAndForce` - e.g.: `PrintPretty`,
//     `PrintNewLine` - bypass processing, and formatting by design: they are
//     forwarded raw @ the `slog.LevelInfo` level, without fields.
//   - A `Handle` error on the slog side is contained by the standard output
//     error handling - logged to stderr - and the message is NOT re-forwarded.
func Output(
	name string,
	sl *slog.Logger,
	maxLevel level.Level,
	processors ...processor.IProcessor,
) output.IOutput {
	return output.
		New(name, maxLevel, &slogWriter{sl: sl}, processors...).
		SetFormatter(newForwarder(sl))
}
