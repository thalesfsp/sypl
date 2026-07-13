// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

// Package output implements Sypl outputs: an output processes, and writes a
// message to its writer - anything implementing io.Writer.
//
// Built-ins: Console (stdout), StdErr (error levels to stderr), File, and
// FileBased (files), SafeBuffer (an in-memory concurrent-safe buffer),
// ElasticSearch (one request per document), and the reliability set:
//
//   - Async: wraps ANY output into a bounded, buffered, asynchronous one -
//     a single worker drains the buffer preserving FIFO order, with Block,
//     DropNewest, or DropOldest full-buffer policies.
//   - ElasticSearchBulk (and ...WithDynamicIndex): batches documents into
//     _bulk requests via esutil's BulkIndexer - the high-throughput sibling
//     of ElasticSearch.
//   - RotatingFile: a file output with native size-based rotation, backup
//     timestamping, and count/age pruning.
//   - Recorder: captures structured snapshots of everything written - a
//     test-assertion helper for Sypl consumers.
//
// Outputs that buffer implement `Flush() error`; outputs owning resources
// implement `Close() error` (io.Closer). Close is idempotent, and writes
// after Close return a typed, per-output sentinel error - never a panic.
// Flush after Close is a no-op.
package output
