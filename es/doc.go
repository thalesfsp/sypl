// Package elasticsearch provides a client for the Elasticsearch. Features:
// - Message's content by default is JSON-formatted.
// - Provides multiple ways to set the index name: static, or dynamic - which
// is evaluated at index time.
// - Two indexing strategies: `ElasticSearch` (one request per document),
// and `ElasticSearchBulk` (documents batched into _bulk requests via
// esutil's BulkIndexer - see `NewBulk`, and `NewBulkWithDynamicIndex`).
// The bulk client indexes asynchronously: failures are delivered through
// `BulkWithOnError`, and `Flush`/`Close` drain it.
//
// NOTE: It's the caller's responsibility to create the index, define its
// mapping, and settings.
package elasticsearch
