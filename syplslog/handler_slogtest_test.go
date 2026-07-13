// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package syplslog

import (
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"testing/slogtest"
	"time"

	"github.com/thalesfsp/sypl"
	"github.com/thalesfsp/sypl/formatter"
	"github.com/thalesfsp/sypl/level"
	"github.com/thalesfsp/sypl/output"
)

// syplJSONBuiltins are the keys sypl's JSON formatter emits on its own -
// everything else on a line is a (flattened) field.
var syplJSONBuiltins = map[string]struct{}{
	"component":          {},
	"contentBasedHashID": {},
	"flag":               {},
	"id":                 {},
	"level":              {},
	"message":            {},
	"output":             {},
	"outputsNames":       {},
	"processorsNames":    {},
	"tags":               {},
	"timestamp":          {},
}

// nest converts the handler's flattened "group.key" field encoding back into
// nested maps - the structure `slogtest` expects for groups.
func nest(flat map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}

	for key, value := range flat {
		parts := strings.Split(key, ".")

		current := out

		for i, part := range parts {
			if i == len(parts)-1 {
				current[part] = value

				break
			}

			next, ok := current[part].(map[string]interface{})
			if !ok {
				next = map[string]interface{}{}

				current[part] = next
			}

			current = next
		}
	}

	return out
}

// parseSyplJSONLines decodes each JSON line emitted by sypl's JSON formatter
// into the map shape `slogtest` verifies: built-in keys are translated to the
// standard slog keys, a zero timestamp is omitted - the handler forwards a
// zero `Record.Time` as-is - and flattened fields are re-nested.
func parseSyplJSONLines(t *testing.T, raw string) []map[string]interface{} {
	t.Helper()

	results := []map[string]interface{}{}

	for _, line := range strings.Split(raw, "\n") {
		// sypl's JSON formatter content carries the JSON encoder's own
		// trailing newline, and the pipeline restores the record's linebreak
		// after it - so records are separated by a blank line.
		if line == "" {
			continue
		}

		decoded := map[string]interface{}{}

		if err := json.Unmarshal([]byte(line), &decoded); err != nil {
			t.Fatalf("output line is not valid JSON: %v: %q", err, line)
		}

		entry := map[string]interface{}{
			slog.LevelKey:   decoded["level"],
			slog.MessageKey: decoded["message"],
		}

		timestamp, ok := decoded["timestamp"].(string)
		if !ok {
			t.Fatalf("line without a timestamp: %q", line)
		}

		parsed, err := time.Parse(time.RFC3339, timestamp)
		if err != nil {
			t.Fatalf("invalid timestamp %q: %v", timestamp, err)
		}

		if !parsed.IsZero() {
			entry[slog.TimeKey] = parsed
		}

		flat := map[string]interface{}{}

		for k, v := range decoded {
			if _, isBuiltin := syplJSONBuiltins[k]; isBuiltin {
				continue
			}

			flat[k] = v
		}

		for k, v := range nest(flat) {
			entry[k] = v
		}

		results = append(results, entry)
	}

	return results
}

// The handler passes the stdlib `slog.Handler` conformance suite end-to-end:
// slog.Logger -> Handler -> sypl -> JSON formatter -> buffer.
func TestHandler_Slogtest(t *testing.T) {
	buf, o := output.SafeBuffer(level.Trace)
	o.SetFormatter(formatter.JSON())

	l := sypl.New("slogtest", o)

	h := NewHandler(l)

	if err := slogtest.TestHandler(h, func() []map[string]interface{} {
		return parseSyplJSONLines(t, buf.String())
	}); err != nil {
		t.Fatalf("slogtest failed: %v", err)
	}
}
