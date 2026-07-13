// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

// Laziness of the message UUID, and content hash must be TRANSPARENT to the
// pipeline: consumers that read the ID (e.g. the JSON formatter) observe
// exactly the eager behavior.
package sypl_test

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/thalesfsp/sypl/v2"
	"github.com/thalesfsp/sypl/v2/formatter"
	"github.com/thalesfsp/sypl/v2/level"
	"github.com/thalesfsp/sypl/v2/message"
	"github.com/thalesfsp/sypl/v2/output"
)

var uuidV4E2ERegex = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// The JSON formatter reads GetID/GetContentBasedHashID: the emitted values
// must be exactly what the eager implementation produced - a well-formed
// UUIDv4, and the deterministic content hash.
func TestLazyID_TransparentToJSONFormatter(t *testing.T) {
	buf, o := output.SafeBuffer(level.Trace)
	o.SetFormatter(formatter.JSON())

	l := sypl.New("lazyid-json", o)

	l.Println(level.Info, "lazy-id-msg")

	line := strings.TrimSuffix(buf.String(), "\n")

	decoded := map[string]interface{}{}
	if err := json.Unmarshal([]byte(line), &decoded); err != nil {
		t.Fatalf("output is not valid JSON: %v: %q", err, line)
	}

	id, ok := decoded["id"].(string)
	if !ok || !uuidV4E2ERegex.MatchString(id) {
		t.Fatalf("id = %v, want a well-formed UUIDv4", decoded["id"])
	}

	// The content hash is deterministic: it must match a freshly-computed
	// eager message for the same content.
	want := message.New(level.Info, "lazy-id-msg").GetContentBasedHashID()

	if decoded["contentBasedHashID"] != want {
		t.Fatalf("contentBasedHashID = %v, want %v", decoded["contentBasedHashID"], want)
	}
}

// WithID must still pin the message ID end-to-end.
func TestLazyID_WithIDStillWorks(t *testing.T) {
	buf, o := output.SafeBuffer(level.Trace)
	o.SetFormatter(formatter.JSON())

	l := sypl.New("lazyid-withid", o)

	l.PrintWithOptions(level.Info, "pinned-id-msg\n", sypl.WithID("pinned-id-123"))

	line := strings.TrimSuffix(buf.String(), "\n")

	decoded := map[string]interface{}{}
	if err := json.Unmarshal([]byte(line), &decoded); err != nil {
		t.Fatalf("output is not valid JSON: %v: %q", err, line)
	}

	if decoded["id"] != "pinned-id-123" {
		t.Fatalf("id = %v, want pinned-id-123", decoded["id"])
	}
}

// Two outputs, both JSON: the per-output copies must report the SAME message
// identity (the copies share the lazy cells).
func TestLazyID_SameIdentityAcrossOutputs(t *testing.T) {
	bufA, oA := output.SafeBuffer(level.Trace)
	oA.SetName("A")
	oA.SetFormatter(formatter.JSON())

	bufB, oB := output.SafeBuffer(level.Trace)
	oB.SetName("B")
	oB.SetFormatter(formatter.JSON())

	l := sypl.New("lazyid-fanout", oA, oB)

	l.Println(level.Info, "fanout-id-msg")

	decode := func(raw string) map[string]interface{} {
		t.Helper()

		decoded := map[string]interface{}{}
		if err := json.Unmarshal([]byte(strings.TrimSuffix(raw, "\n")), &decoded); err != nil {
			t.Fatalf("output is not valid JSON: %v: %q", err, raw)
		}

		return decoded
	}

	a, b := decode(bufA.String()), decode(bufB.String())

	if a["id"] == "" || a["id"] != b["id"] {
		t.Fatalf("per-output copies diverged: id A=%v B=%v", a["id"], b["id"])
	}

	if a["contentBasedHashID"] == "" || a["contentBasedHashID"] != b["contentBasedHashID"] {
		t.Fatalf("per-output copies diverged: hash A=%v B=%v", a["contentBasedHashID"], b["contentBasedHashID"])
	}
}
