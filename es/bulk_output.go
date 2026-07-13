// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package es

import (
	"fmt"

	"github.com/thalesfsp/sypl/v2/formatter"
	"github.com/thalesfsp/sypl/v2/level"
	"github.com/thalesfsp/sypl/v2/output"
	"github.com/thalesfsp/sypl/v2/processor"
)

//////
// Consts, vars, and types.
//////

// bulkOutput is an ElasticSearch-bulk-backed `output.IOutput` carrying the
// Flush, and Close capabilities.
type bulkOutput struct {
	*output.Proxy

	es *ElasticSearchBulk
}

// Flush drains the bulk indexer - see `ElasticSearchBulk.Flush`. After
// Close it's a no-op.
func (o *bulkOutput) Flush() error {
	return o.es.Flush()
}

// Close drains, and shuts the bulk indexer down. It's idempotent. Writes
// after Close return `ErrBulkClosed`.
func (o *bulkOutput) Close() error {
	return o.es.Close()
}

//////
// Factory.
//////

// bulkOutputFactory builds ElasticSearch bulk outputs.
func bulkOutputFactory(
	outputName string,
	dynamicIndexFunc DynamicIndexFunc,
	esConfig Config,
	maxLevel level.Level,
	bulkOpts []BulkOption,
	processors ...processor.IProcessor,
) output.IOutput {
	es := NewBulkWithDynamicIndex(dynamicIndexFunc, esConfig, bulkOpts...)

	// NOTE: `output.New` defensively clones the processors slice - the
	// caller's backing array is never aliased.
	//
	// NOTE: INLINE JSON - not JSONPretty like the sync factory: the _bulk
	// NDJSON protocol requires each document on a single line; an indented
	// document would corrupt the batched payload.
	inner := output.New(outputName, maxLevel, es, processors...).SetFormatter(formatter.JSON())

	o := &bulkOutput{es: es}

	o.Proxy = output.NewProxy(inner, o)

	return o
}

//////
// Builtins.
//////

// BulkOutput is a built-in `output` - named `ElasticSearchBulk`,
// that writes to ElasticSearch batching documents into _bulk requests -
// the high-throughput sibling of `Output`.
//
// Capabilities: `Flush() error` (drains the indexer), and idempotent
// `Close() error`. Indexing is asynchronous: per-item failures are
// delivered through `BulkWithOnError`.
//
// NOTE: Formerly `output.ElasticSearchBulk`.
// NOTE: By default, data is JSON-formatted.
// NOTE: It's the caller's responsibility to create the index, define its
// mapping, and settings.
func BulkOutput(
	indexName string,
	esConfig Config,
	maxLevel level.Level,
	bulkOpts []BulkOption,
	processors ...processor.IProcessor,
) output.IOutput {
	return bulkOutputFactory(
		"ElasticSearchBulk",
		func() string { return indexName },
		esConfig,
		maxLevel,
		bulkOpts,
		processors...,
	)
}

// BulkOutputWithDynamicIndex is a built-in `output` - named
// `ElasticSearchBulkWithDynamicIndex-{index}` that writes to ElasticSearch
// batching documents into _bulk requests. It allows to define a function
// that returns the index name to be used, evaluated at the index time.
//
// Capabilities: `Flush() error` (drains the indexer), and idempotent
// `Close() error`. Indexing is asynchronous: per-item failures are
// delivered through `BulkWithOnError`.
//
// NOTE: Formerly `output.ElasticSearchBulkWithDynamicIndex`.
// NOTE: By default, data is JSON-formatted.
// NOTE: It's the caller's responsibility to create the index, define its
// mapping, and settings.
func BulkOutputWithDynamicIndex(
	dynamicIndexFunc DynamicIndexFunc,
	esConfig Config,
	maxLevel level.Level,
	bulkOpts []BulkOption,
	processors ...processor.IProcessor,
) output.IOutput {
	return bulkOutputFactory(
		fmt.Sprintf("ElasticSearchBulkWithDynamicIndex-%s", dynamicIndexFunc()),
		dynamicIndexFunc,
		esConfig,
		maxLevel,
		bulkOpts,
		processors...,
	)
}
