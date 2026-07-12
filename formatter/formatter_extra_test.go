// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package formatter

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/thalesfsp/sypl/fields"
	"github.com/thalesfsp/sypl/flag"
	"github.com/thalesfsp/sypl/level"
	"github.com/thalesfsp/sypl/message"
	"github.com/thalesfsp/sypl/shared"
)

// fullyLoadedMessage builds a message exercising every optional branch of
// `mapBuilder`: tags, flag, outputs names, processors names, and fields -
// including a nil-valued field, which must be dropped.
func fullyLoadedMessage() message.IMessage {
	m := message.New(level.Info, shared.DefaultContentOutput)

	m.SetComponentName(shared.DefaultComponentNameOutput)
	m.SetOutputName("Console")
	m.AddTags("alpha", "beta")
	m.SetFlag(flag.Force)
	m.SetOutputsNames([]string{"Console", "File"})
	m.SetProcessorsNames([]string{"Prefixer"})
	m.SetFields(fields.Fields{
		"customField": "customValue",
		"nilField":    nil,
	})

	return m
}

// unmarshalProcessed unmarshals the message's processed content as JSON.
func unmarshalProcessed(t *testing.T, m message.IMessage) map[string]interface{} {
	t.Helper()

	parsed := map[string]interface{}{}

	if err := json.Unmarshal([]byte(m.GetContent().GetProcessed()), &parsed); err != nil {
		t.Fatalf("Processed content isn't valid JSON: %v.\nContent: %s",
			err, m.GetContent().GetProcessed())
	}

	return parsed
}

// assertFullyLoadedJSON asserts the JSON produced from a fully loaded
// message - shared by the JSON, and JSONPretty tests.
func assertFullyLoadedJSON(t *testing.T, m message.IMessage, parsed map[string]interface{}) {
	t.Helper()

	// Always-present keys.
	if parsed["component"] != shared.DefaultComponentNameOutput {
		t.Errorf(`component = %v, want %q`, parsed["component"], shared.DefaultComponentNameOutput)
	}

	if parsed["output"] != "Console" {
		t.Errorf(`output = %v, want "Console"`, parsed["output"])
	}

	if parsed["level"] != "info" {
		t.Errorf(`level = %v, want "info"`, parsed["level"])
	}

	if parsed["message"] != shared.DefaultContentOutput {
		t.Errorf(`message = %v, want %q`, parsed["message"], shared.DefaultContentOutput)
	}

	if parsed["id"] != m.GetID() {
		t.Errorf(`id = %v, want %q`, parsed["id"], m.GetID())
	}

	if parsed["contentBasedHashID"] != m.GetContentBasedHashID() {
		t.Errorf(`contentBasedHashID = %v, want %q`, parsed["contentBasedHashID"], m.GetContentBasedHashID())
	}

	if _, ok := parsed["timestamp"]; !ok {
		t.Error("timestamp key is missing")
	}

	// Conditional keys - all present for a fully loaded message.
	tags, ok := parsed["tags"].([]interface{})
	if !ok || len(tags) != 2 {
		t.Errorf("tags = %v, want [alpha beta]", parsed["tags"])
	}

	// flag.Force marshals as its numeric value.
	if parsed["flag"] != float64(flag.Force) {
		t.Errorf("flag = %v, want %d", parsed["flag"], flag.Force)
	}

	outputsNames, ok := parsed["outputsNames"].([]interface{})
	if !ok || len(outputsNames) != 2 {
		t.Errorf("outputsNames = %v, want [Console File]", parsed["outputsNames"])
	}

	processorsNames, ok := parsed["processorsNames"].([]interface{})
	if !ok || len(processorsNames) != 1 {
		t.Errorf("processorsNames = %v, want [Prefixer]", parsed["processorsNames"])
	}

	if parsed["customField"] != "customValue" {
		t.Errorf(`customField = %v, want "customValue"`, parsed["customField"])
	}

	// A nil-valued field must be dropped.
	if _, ok := parsed["nilField"]; ok {
		t.Error("nilField should have been dropped")
	}
}

func TestJSON_FullyLoadedMessage(t *testing.T) {
	m := fullyLoadedMessage()

	if err := JSON().Run(m); err != nil {
		t.Fatalf("JSON() error: %v", err)
	}

	assertFullyLoadedJSON(t, m, unmarshalProcessed(t, m))
}

func TestJSONPretty_FullyLoadedMessage(t *testing.T) {
	m := fullyLoadedMessage()

	if err := JSONPretty().Run(m); err != nil {
		t.Fatalf("JSONPretty() error: %v", err)
	}

	assertFullyLoadedJSON(t, m, unmarshalProcessed(t, m))

	// Prettified means indented, multi-line JSON.
	if !strings.Contains(m.GetContent().GetProcessed(), "\n") {
		t.Error("JSONPretty() should produce multi-line JSON")
	}
}

func TestJSON_MinimalMessage(t *testing.T) {
	// No tags, no flag, no outputs/processors names, no fields - every
	// conditional key must be absent.
	m := message.New(level.Error, shared.DefaultContentOutput)

	if err := JSON().Run(m); err != nil {
		t.Fatalf("JSON() error: %v", err)
	}

	parsed := unmarshalProcessed(t, m)

	if parsed["level"] != "error" {
		t.Errorf(`level = %v, want "error"`, parsed["level"])
	}

	for _, key := range []string{"tags", "flag", "outputsNames", "processorsNames"} {
		if _, ok := parsed[key]; ok {
			t.Errorf("Key %q should be absent for a minimal message, got %v", key, parsed[key])
		}
	}
}

func TestText_TagsAndNilFields(t *testing.T) {
	m := message.New(level.Info, shared.DefaultContentOutput)

	m.SetComponentName(shared.DefaultComponentNameOutput)
	m.SetOutputName("Console")
	m.AddTags("alpha")
	m.SetFields(fields.Fields{
		"presentField": "value",
		"nilField":     nil,
	})

	if err := Text().Run(m); err != nil {
		t.Fatalf("Text() error: %v", err)
	}

	got := m.GetContent().GetProcessed()

	if !strings.Contains(got, "tags=[alpha]") {
		t.Errorf("Text() = %q, missing tags=[alpha]", got)
	}

	if !strings.Contains(got, "output=console") {
		t.Errorf("Text() = %q, missing output=console (lowercased)", got)
	}

	if !strings.Contains(got, "presentField=value") {
		t.Errorf("Text() = %q, missing presentField=value", got)
	}

	if strings.Contains(got, "nilField") {
		t.Errorf("Text() = %q, nilField should have been dropped", got)
	}
}

func TestText_NoTagsNoFields(t *testing.T) {
	m := message.New(level.Info, shared.DefaultContentOutput)

	if err := Text().Run(m); err != nil {
		t.Fatalf("Text() error: %v", err)
	}

	got := m.GetContent().GetProcessed()

	if strings.Contains(got, "tags=") {
		t.Errorf("Text() = %q, tags should be absent", got)
	}

	if !strings.Contains(got, "message="+shared.DefaultContentOutput) {
		t.Errorf("Text() = %q, missing the message", got)
	}
}
