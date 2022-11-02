package shared

import (
	"bytes"
	"encoding/json"
	"log"

	"github.com/google/uuid"
)

// Prettify encodes data returning its JSON-stringified version.
//
// NOTE: Only exported fields of the data structure will be printed.
func Prettify(data interface{}) string {
	return jsonfy("", "\t", data)
}

// Inline encodes JSON in line.
//
// NOTE: Only exported fields of the data structure will be printed.
func Inline(data interface{}) string {
	return jsonfy("", "", data)
}

// InLine encodes JSON in line.
func jsonfy(prefix string, indent string, data interface{}) string {
	buf := new(bytes.Buffer)

	enc := json.NewEncoder(buf)

	enc.SetIndent(prefix, indent)

	if err := enc.Encode(data); err != nil {
		log.Println(ErrorPrefix, "inline: Failed to encode data.", err)

		return ""
	}

	return buf.String()
}

// GenerateUUID generates a RFC4122 UUID and DCE 1.1: Authentication and
// Security Services.
func GenerateUUID() string {
	return uuid.New().String()
}
