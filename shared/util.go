package shared

import (
	"bytes"
	"encoding/json"
	"log"
)

// Prettify encodes data returning its JSON-stringified version.
//
// Note: Only exported fields of the data structure will be printed.
func Prettify(data interface{}) string {
	return jsonfy("", "\t", data)
}

// Inline encodes JSON in line.
//
// Note: Only exported fields of the data structure will be printed.
func Inline(data interface{}) string {
	return jsonfy("", "", data)
}

// InLine encodes JSON in line.
//
// Note: Only exported fields of the data structure will be printed.
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
