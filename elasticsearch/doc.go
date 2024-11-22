// Package elasticsearch provides a client for the Elasticsearch. Features:
// - Message's content by default is JSON-formatted.
// - Provides multiple ways to set the index name: static, or dynamic - which
// is evaluated at index time.
//
// NOTE: It's the caller's responsibility to create the index, define its
// mapping, and settings.
package elasticsearch
