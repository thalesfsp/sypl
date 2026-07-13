// Package es provides Sypl's ElasticSearch support: the low-level client,
// and the ready-to-use `output.IOutput` factories. It lives in its own Go
// module (github.com/thalesfsp/sypl/es/v2) so the core sypl module carries
// no ElasticSearch dependency - import this module only if you log to
// ElasticSearch.
//
// Client features:
// - Message's content by default is JSON-formatted.
// - Provides multiple ways to set the index name: static, or dynamic - which
// is evaluated at index time.
// - Two indexing strategies: `ElasticSearch` (one request per document),
// and `ElasticSearchBulk` (documents batched into _bulk requests via
// esutil's BulkIndexer - see `NewBulk`, and `NewBulkWithDynamicIndex`).
// The bulk client indexes asynchronously: failures are delivered through
// `BulkWithOnError`, and `Flush`/`Close` drain it.
//
// Output factories (formerly in the `output` package):
// - `Output` (was `output.ElasticSearch`)
// - `OutputWithDynamicIndex` (was `output.ElasticSearchWithDynamicIndex`)
// - `OutputWithTagMap` (was `output.ElasticSearchWithTagMap`)
// - `BulkOutput` (was `output.ElasticSearchBulk`)
// - `BulkOutputWithDynamicIndex` (was `output.ElasticSearchBulkWithDynamicIndex`)
//
// NOTE: It's the caller's responsibility to create the index, define its
// mapping, and settings.
package es
