// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package sypl_test

import (
	"context"
	"fmt"
	"os"

	"github.com/thalesfsp/sypl"
	"github.com/thalesfsp/sypl/fields"
	"github.com/thalesfsp/sypl/level"
	"github.com/thalesfsp/sypl/message"
	"github.com/thalesfsp/sypl/output"
	"github.com/thalesfsp/sypl/processor"
)

// exampleTraceKey is the application's own context key - sypl imports no
// tracing library; the application wires its own extractor.
type exampleTraceKey struct{}

// ExampleSypl_SetContextExtractor demonstrates wiring a fake trace-id
// extractor: the `*WithContext` printers pull fields out of the context, and
// merge them into every message.
func ExampleSypl_SetContextExtractor() {
	// A processor that renders the extracted field, keeping the example
	// output deterministic.
	appendTraceID := processor.New("AppendTraceID", func(m message.IMessage) error {
		if v, ok := m.GetFields()["trace_id"]; ok {
			m.GetContent().SetProcessed(
				fmt.Sprintf("%s trace_id=%v", m.GetContent().GetProcessed(), v),
			)
		}

		return nil
	})

	l := sypl.New("api", output.New("Stdout", level.Info, os.Stdout, appendTraceID)).
		SetContextExtractor(func(ctx context.Context) fields.Fields {
			if traceID, ok := ctx.Value(exampleTraceKey{}).(string); ok {
				return fields.Fields{"trace_id": traceID}
			}

			return nil
		})

	ctx := context.WithValue(context.Background(), exampleTraceKey{}, "abc-123")

	l.InfoWithContext(ctx, "handling request")

	// Output:
	// handling request trace_id=abc-123
}
