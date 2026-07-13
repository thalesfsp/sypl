// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package elasticsearch

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/elastic/go-elasticsearch/v8/esutil"
)

//////
// Consts, vars, and types.
//////

// defaultBulkCloseTimeout bounds how long Flush, and Close wait for the
// indexer to drain.
const defaultBulkCloseTimeout = 30 * time.Second

// ErrBulkClosed is returned when writing to a closed bulk output.
var ErrBulkClosed = errors.New("elasticsearch bulk output is closed")

// newBulkIndexer is a seam for tests.
var newBulkIndexer = esutil.NewBulkIndexer

// BulkOption configures the `ElasticSearchBulk` output.
type BulkOption func(*ElasticSearchBulk)

// BulkWithFlushBytes sets the flush threshold in bytes. Defaults to
// esutil's 5MB.
func BulkWithFlushBytes(n int) BulkOption {
	return func(es *ElasticSearchBulk) {
		es.flushBytes = n
	}
}

// BulkWithFlushInterval sets the periodic flush interval. Defaults to
// esutil's 30s.
func BulkWithFlushInterval(d time.Duration) BulkOption {
	return func(es *ElasticSearchBulk) {
		es.flushInterval = d
	}
}

// BulkWithNumWorkers sets how many workers flush concurrently. Defaults to
// esutil's runtime.NumCPU().
func BulkWithNumWorkers(n int) BulkOption {
	return func(es *ElasticSearchBulk) {
		es.numWorkers = n
	}
}

// BulkWithOnError sets the callback receiving per-item indexing failures,
// and indexer-level (e.g. flush) failures. The callback may be called
// concurrently.
func BulkWithOnError(cb func(error)) BulkOption {
	return func(es *ElasticSearchBulk) {
		es.onError = cb
	}
}

// BulkWithCloseTimeout bounds how long Flush, and Close wait for the
// indexer to drain. Defaults to 30s.
func BulkWithCloseTimeout(d time.Duration) BulkOption {
	return func(es *ElasticSearchBulk) {
		es.closeTimeout = d
	}
}

// ElasticSearchBulk `Output` definition: it batches documents through
// esutil's BulkIndexer instead of one request per document.
type ElasticSearchBulk struct {
	// Client is the ElasticSearch client.
	Client *elasticsearch.Client

	// Config is the ElasticSearch configuration.
	Config Config

	// DynamicIndex is a function which defines the name of the index, and
	// evaluated at the index time.
	DynamicIndex DynamicIndexFunc

	// Indexer configuration.
	closeTimeout  time.Duration
	flushBytes    int
	flushInterval time.Duration
	numWorkers    int
	onError       func(error)

	// mu guards the indexer - swapped on Flush - and the closed flag.
	mu      sync.Mutex
	closed  bool
	indexer esutil.BulkIndexer
}

//////
// Methods.
//////

// Write conforms to the `io.Writer` interface: it enqueues the document
// into the bulk indexer. Indexing is asynchronous - per-item failures are
// delivered to the `BulkWithOnError` callback, never panics. After Close,
// it returns `ErrBulkClosed`.
func (es *ElasticSearchBulk) Write(data []byte) (int, error) {
	parsedData, err := parseResponseBody(bytes.NewReader(data))
	if err != nil {
		return 0, err
	}

	item := esutil.BulkIndexerItem{
		Action:    "index",
		Body:      bytes.NewReader(data),
		Index:     es.DynamicIndex(),
		OnFailure: es.reportItemFailure,
	}

	// Check if parsedData has an id.
	//
	// NOTE: A non-string id is skipped - not an error. A logging library
	// must never panic the host application on an odd payload.
	if id, ok := parsedData["id"].(string); ok {
		item.DocumentID = id
	}

	es.mu.Lock()
	defer es.mu.Unlock()

	if es.closed {
		return 0, ErrBulkClosed
	}

	// Guard against an uninitialized indexer - never panic the host
	// application.
	if es.indexer == nil {
		return 0, errors.New("elasticsearch bulk indexer isn't initialized")
	}

	if err := es.indexer.Add(context.Background(), item); err != nil {
		return 0, fmt.Errorf("failed adding document to the bulk indexer: %w", err)
	}

	return len(data), nil
}

// Flush drains the bulk indexer - waiting, bounded by the close timeout
// (default: 30s, see `BulkWithCloseTimeout`), until every enqueued document
// is sent - then swaps in a fresh indexer, so the output keeps working.
// After Close it's a no-op.
func (es *ElasticSearchBulk) Flush() error {
	es.mu.Lock()
	defer es.mu.Unlock()

	if es.closed {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), es.closeTimeout)
	defer cancel()

	// esutil's BulkIndexer has no flush primitive: closing it drains it. A
	// fresh indexer is swapped in EVEN on failure - the old one is closed,
	// and would panic on Add.
	err := es.indexer.Close(ctx)

	es.indexer = es.newIndexer()

	if err != nil {
		return fmt.Errorf("failed flushing the bulk indexer: %w", err)
	}

	return nil
}

// Close drains the bulk indexer - waiting, bounded by the close timeout
// (default: 30s, see `BulkWithCloseTimeout`) - and shuts it down. It's
// idempotent. Writes after Close return `ErrBulkClosed` - never panic.
func (es *ElasticSearchBulk) Close() error {
	es.mu.Lock()
	defer es.mu.Unlock()

	if es.closed {
		return nil
	}

	es.closed = true

	ctx, cancel := context.WithTimeout(context.Background(), es.closeTimeout)
	defer cancel()

	if err := es.indexer.Close(ctx); err != nil {
		return fmt.Errorf("failed closing the bulk indexer: %w", err)
	}

	return nil
}

//////
// Helpers.
//////

// reportItemFailure delivers a per-item indexing failure to the error
// callback, if any.
func (es *ElasticSearchBulk) reportItemFailure(
	_ context.Context,
	item esutil.BulkIndexerItem,
	res esutil.BulkIndexerResponseItem,
	err error,
) {
	if es.onError == nil {
		return
	}

	if err != nil {
		es.onError(fmt.Errorf(
			`failed indexing document (index: "%s", id: "%s"): %w`,
			item.Index,
			item.DocumentID,
			err,
		))

		return
	}

	es.onError(fmt.Errorf(
		`failed indexing document (index: "%s", id: "%s", status: %d): %s: %s`,
		item.Index,
		item.DocumentID,
		res.Status,
		res.Error.Type,
		res.Error.Reason,
	))
}

// newIndexer builds a bulk indexer from the stored configuration.
func (es *ElasticSearchBulk) newIndexer() esutil.BulkIndexer {
	bi, err := newBulkIndexer(esutil.BulkIndexerConfig{
		Client:        es.Client,
		FlushBytes:    es.flushBytes,
		FlushInterval: es.flushInterval,
		NumWorkers:    es.numWorkers,
		OnError: func(_ context.Context, err error) {
			if es.onError != nil {
				es.onError(fmt.Errorf("bulk indexer error: %w", err))
			}
		},
	})
	if err != nil {
		log.Fatalf("Error creating the ElasticSearch bulk indexer: %s", err)
	}

	return bi
}

//////
// Factory.
//////

// NewBulk returns a new `ElasticSearchBulk` client indexing into
// `indexName`.
//
// NOTE: Indexing is asynchronous, and batched - deliver failures through
// `BulkWithOnError`, and drain with `Flush`, or `Close`.
func NewBulk(
	indexName string,
	esConfig Config,
	opts ...BulkOption,
) *ElasticSearchBulk {
	return NewBulkWithDynamicIndex(func() string { return indexName }, esConfig, opts...)
}

// NewBulkWithDynamicIndex returns a new `ElasticSearchBulk` client. It
// allows to define a function which defines the name of the index, and
// evaluated at the index time.
//
// NOTE: Indexing is asynchronous, and batched - deliver failures through
// `BulkWithOnError`, and drain with `Flush`, or `Close`.
func NewBulkWithDynamicIndex(
	dynamicIndexFunc DynamicIndexFunc,
	esConfig Config,
	opts ...BulkOption,
) *ElasticSearchBulk {
	// Client creation, and the connectivity ping mirror the sync factory -
	// including its log.Fatalf failure behavior.
	base := NewWithDynamicIndex(dynamicIndexFunc, esConfig)

	es := &ElasticSearchBulk{
		Client:       base.Client,
		Config:       esConfig,
		DynamicIndex: base.DynamicIndex,

		closeTimeout: defaultBulkCloseTimeout,
	}

	for _, opt := range opts {
		opt(es)
	}

	es.indexer = es.newIndexer()

	return es
}
