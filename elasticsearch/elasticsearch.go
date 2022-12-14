package elasticsearch

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/elastic/go-elasticsearch/v8/esapi"
)

//////
// Const, vars, and types.
//////

var contextTimeout = 5 * time.Second

// DynamicIndexFunc is a function which defines the name of the index, and
// evaluated at the index time.
type DynamicIndexFunc func() string

// Config is the ElasticSearch configuration.
type Config = elasticsearch.Config

// ElasticSearch `Output` definition.
type ElasticSearch struct {
	// Client is the ElasticSearch client.
	Client *elasticsearch.Client

	// Config is the ElasticSearch configuration.
	Config Config

	// DynamicIndex is a function which defines the name of the index, and
	// evaluated at the index time.
	DynamicIndex DynamicIndexFunc
}

//////
// Methods.
//////

// Write conforms to the `io.Writer` interface.
func (es *ElasticSearch) Write(data []byte) (int, error) {
	// Extract message's id which is generated by hashing the data. It'll avoid
	// inserting duplicate documents.
	parsedData, err := parseResponseBody(bytes.NewReader(data))
	if err != nil {
		return 0, err
	}

	// Set up the request object.
	req := esapi.IndexRequest{
		Body:  bytes.NewReader(data),
		Index: es.DynamicIndex(),
	}

	// Check if parsedData as an id.
	if parsedData["id"] != nil {
		req.DocumentID = parsedData["id"].(string)
	}

	// Perform the request with the client.
	ctx, cancel := context.WithTimeout(context.Background(), contextTimeout)
	defer cancel()

	res, err := req.Do(ctx, es.Client)
	if err != nil {
		return 0, fmt.Errorf("failed getting response: %w", err)
	}
	defer res.Body.Close()

	// Verify if an error occurred.
	if res.IsError() {
		errMsg, err := parseResponseBodyError(res)
		if err != nil {
			return 0, err
		}

		return 0, fmt.Errorf("failed indexing document: %s", errMsg)
	}

	// Deserialize the response into a map.
	parsedRespBody, err := parseResponseBody(res.Body)
	if err != nil {
		return 0, err
	}

	// Verify if document was really created/updated.
	parsedRespBodyResult := parsedRespBody["result"].(string)
	if parsedRespBodyResult == "created" || parsedRespBodyResult == "updated" {
		return len(data), nil
	}

	return 0, fmt.Errorf("unexpected result: %+v", parsedRespBody)
}

//////
// Factory.
//////

// New returns a new `ElasticSearch` client.
func New(
	indexName string,
	esConfig Config,
) *ElasticSearch {
	return NewWithDynamicIndex(func() string { return indexName }, esConfig)
}

// NewWithDynamicIndex returns a new `ElasticSearch` client. It allows to define
// is a function which defines the name of the index, and evaluated at the index
// time.
func NewWithDynamicIndex(
	dynamicIndexFunc DynamicIndexFunc,
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
		Client:       es,
		Config:       esConfig,
		DynamicIndex: dynamicIndexFunc,
	}
}
