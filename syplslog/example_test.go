// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package syplslog_test

import (
	"log/slog"
	"os"

	"github.com/thalesfsp/sypl"
	"github.com/thalesfsp/sypl/fields"
	"github.com/thalesfsp/sypl/level"
	"github.com/thalesfsp/sypl/output"
	"github.com/thalesfsp/sypl/syplslog"
)

// ExampleNewHandler demonstrates the slog -> sypl direction: code using the
// standard slog API logs through a sypl logger.
func ExampleNewHandler() {
	// A sypl logger printing to stdout @ Trace, and above.
	l := sypl.New("app", output.Console(level.Trace))

	// A standard *slog.Logger backed by the sypl logger.
	logger := slog.New(syplslog.NewHandler(l))

	logger.Info("Hello from slog!")

	// Output:
	// Hello from slog!
}

// ExampleOutput demonstrates the sypl -> slog direction: a sypl logger
// forwarding processed messages - fields as attrs - to a slog handler.
func ExampleOutput() {
	// A standard slog logger - time dropped for a deterministic example.
	sl := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey && len(groups) == 0 {
				return slog.Attr{}
			}

			return a
		},
	}))

	// A sypl logger whose output forwards to the slog logger.
	l := sypl.New("app", syplslog.Output("slog", sl, level.Trace))

	l.PrintWithOptions(
		level.Info,
		"Hello from sypl!",
		sypl.WithFields(fields.Fields{"user": "thales"}),
	)

	// Output:
	// level=INFO msg="Hello from sypl!" user=thales
}
