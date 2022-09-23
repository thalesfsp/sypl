package elasticsearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/elastic/go-elasticsearch/v8/esapi"
)

// Config is the ElasticSearch configuration.
type Config = elasticsearch.Config

// ElasticSearch `Output` definition.
type ElasticSearch struct {
	// Client is the ElasticSearch client.
	Client *elasticsearch.Client

	// Config is the ElasticSearch configuration.
	Config Config

	// IndexName is the name of the index to be used.
	//
	// NOTE: It's the caller's responsibility to create the index, define its
	// mapping, and settings.
	Index string
}

// Write conforms to the `io.Writer` interface.
//
// NOTE: By default, data is JSON-formatted.
// NOTE: `DocumentID` is automatically generated.
func (es *ElasticSearch) Write(data []byte) (int, error) {
	// Set up the request object.
	req := esapi.IndexRequest{
		Index: es.Index,
		Body:  bytes.NewReader(data),
	}

	// Perform the request with the client.
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	res, err := req.Do(ctx, es.Client)
	if err != nil {
		return 0, fmt.Errorf("failed getting response: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return 0, fmt.Errorf("[%s] failed indexing document", res.Status())
	}

	// Deserialize the response into a map.
	var r map[string]interface{}

	if err := json.NewDecoder(res.Body).Decode(&r); err != nil {
		return 0, fmt.Errorf("failed parsing the response body: %w", err)
	}

	// Verify if document was really created.
	if r["result"].(string) == "created" {
		return len(data), nil
	}

	return 0, fmt.Errorf("unexpected result: %+v", r)
}

// New is a built-in `output` - named `ElasticSearch`, that writes to
// ElasticSearch.
//
// NOTE: By default, data is JSON-formatted.
// NOTE: `DocumentID` is automatically generated.
// NOTE: It's the caller's responsibility to create the index, define its
// mapping, and settings.
// and defined.
func New(
	indexName string,
	esConfig Config,
) *ElasticSearch {
	es, err := elasticsearch.NewClient(esConfig)
	if err != nil {
		log.Fatalf("Error creating the ElasticSearch client: %s", err)
	}

	res, err := es.Info()
	if err != nil {
		log.Fatalf("Error pinging Elasticsearch: %s", err)
	}

	if _, err := io.Copy(io.Discard, res.Body); err != nil {
		log.Fatalf("Error consuming the response body: %s", err)
	}

	// NOTE: It is critical to both close the response body and to consume it,
	// in order to re-use persistent TCP connections in the default HTTP
	// transport. If you're not interested in the response body, call
	// `io.Copy(ioutil.Discard, res.Body).`
	defer res.Body.Close()

	return &ElasticSearch{
		Client: es,
		Config: esConfig,
		Index:  indexName,
	}
}
