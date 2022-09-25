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

	return b["error"].(map[string]interface{})["reason"].(string), nil
}
