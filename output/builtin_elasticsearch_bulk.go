// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package output

import (
	"fmt"

	"github.com/thalesfsp/sypl/elasticsearch"
	"github.com/thalesfsp/sypl/formatter"
	"github.com/thalesfsp/sypl/level"
	"github.com/thalesfsp/sypl/processor"
)

//////
// Consts, vars, and types.
//////

// ElasticSearchBulkOption configures the underlying bulk indexer - see the
// `elasticsearch.BulkWith*` options.
type ElasticSearchBulkOption = elasticsearch.BulkOption

// elasticSearchBulkOutput is an ElasticSearch-bulk-backed `IOutput`
// carrying the Flush, and Close capabilities.
type elasticSearchBulkOutput struct {
	*proxyOutput

	es *elasticsearch.ElasticSearchBulk
}

// Flush drains the bulk indexer - see `ElasticSearchBulk.Flush`. After
// Close it's a no-op.
func (o *elasticSearchBulkOutput) Flush() error {
	return o.es.Flush()
}

// Close drains, and shuts the bulk indexer down. It's idempotent. Writes
// after Close return `elasticsearch.ErrBulkClosed`.
func (o *elasticSearchBulkOutput) Close() error {
	return o.es.Close()
}

//////
// Factory.
//////

// elasticSearchBulkFactory builds ElasticSearch bulk outputs.
func elasticSearchBulkFactory(
	outputName string,
	dynamicIndexFunc ElasticSearchDynamicIndexFunc,
	esConfig ElasticSearchConfig,
	maxLevel level.Level,
	bulkOpts []ElasticSearchBulkOption,
	processors ...processor.IProcessor,
) IOutput {
	es := elasticsearch.NewBulkWithDynamicIndex(dynamicIndexFunc, esConfig, bulkOpts...)

	// NOTE: `New` defensively clones the processors slice - the caller's
	// backing array is never aliased.
	//
	// NOTE: INLINE JSON - not JSONPretty like the sync factory: the _bulk
	// NDJSON protocol requires each document on a single line; an indented
	// document would corrupt the batched payload.
	inner := New(outputName, maxLevel, es, processors...).SetFormatter(formatter.JSON())

	o := &elasticSearchBulkOutput{es: es}

	o.proxyOutput = newProxyOutput(inner, o)

	return o
}

//////
// Builtins.
//////

// ElasticSearchBulk is a built-in `output` - named `ElasticSearchBulk`,
// that writes to ElasticSearch batching documents into _bulk requests -
// the high-throughput sibling of `ElasticSearch`.
//
// Capabilities: `Flush() error` (drains the indexer), and idempotent
// `Close() error`. Indexing is asynchronous: per-item failures are
// delivered through `elasticsearch.BulkWithOnError`.
//
// NOTE: By default, data is JSON-formatted.
// NOTE: It's the caller's responsibility to create the index, define its
// mapping, and settings.
func ElasticSearchBulk(
	indexName string,
	esConfig ElasticSearchConfig,
	maxLevel level.Level,
	bulkOpts []ElasticSearchBulkOption,
	processors ...processor.IProcessor,
) IOutput {
	return elasticSearchBulkFactory(
		"ElasticSearchBulk",
		func() string { return indexName },
		esConfig,
		maxLevel,
		bulkOpts,
		processors...,
	)
}

// ElasticSearchBulkWithDynamicIndex is a built-in `output` - named
// `ElasticSearchBulkWithDynamicIndex-{index}` that writes to ElasticSearch
// batching documents into _bulk requests. It allows to define a function
// that returns the index name to be used, evaluated at the index time.
//
// Capabilities: `Flush() error` (drains the indexer), and idempotent
// `Close() error`. Indexing is asynchronous: per-item failures are
// delivered through `elasticsearch.BulkWithOnError`.
//
// NOTE: By default, data is JSON-formatted.
// NOTE: It's the caller's responsibility to create the index, define its
// mapping, and settings.
func ElasticSearchBulkWithDynamicIndex(
	dynamicIndexFunc ElasticSearchDynamicIndexFunc,
	esConfig ElasticSearchConfig,
	maxLevel level.Level,
	bulkOpts []ElasticSearchBulkOption,
	processors ...processor.IProcessor,
) IOutput {
	return elasticSearchBulkFactory(
		fmt.Sprintf("ElasticSearchBulkWithDynamicIndex-%s", dynamicIndexFunc()),
		dynamicIndexFunc,
		esConfig,
		maxLevel,
		bulkOpts,
		processors...,
	)
}
