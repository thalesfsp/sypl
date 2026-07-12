package elasticsearch

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/elastic/go-elasticsearch/v8/esapi"
)

// Parse ES response body.
func parseResponseBody(r io.Reader) (map[string]interface{}, error) {
	var b map[string]interface{}

	if err := json.NewDecoder(r).Decode(&b); err != nil {
		return nil, fmt.Errorf("failed parsing the response body: %w", err)
	}

	return b, nil
}

// Parse ES response body when it's an error.
func parseResponseBodyError(res *esapi.Response) (string, error) {
	b, err := parseResponseBody(res.Body)
	if err != nil {
		return "", err
	}

	// Checked assertions: fall back to a generic representation of the body
	// when the shape is unexpected - never panic.
	errMap, ok := b["error"].(map[string]interface{})
	if !ok {
		return fmt.Sprintf("%+v", b), nil
	}

	reason, ok := errMap["reason"].(string)
	if !ok {
		return fmt.Sprintf("%+v", errMap), nil
	}

	return reason, nil
}
