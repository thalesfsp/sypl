// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

// Package sypl provides a Simple Yet Powerful Logger built on top of the Golang
// logger. A sypl logger can have many `Output`s, and each `Output` is
// responsible for writing to a specified destination. Each Output can have
// multiple `Processor`s, which run in isolation manipulating the log message.
// The order of execution is according to the registering order. The above
// features allow sypl to fit into many different logging flows and needs.
//
// In a application with many loggers, and child loggers, sometimes more fine
// control is needed, specially when debugging applications. Sypl offers two
// powerful ways to achieve that: `SYPL_FILTER`, and `SYPL_DEBUG` env vars.
//
// `SYPL_FILTER` allows to specify the name(s) of the component(s) that should
// be logged, for example, for a given application with the following loggers:
// `svc`, `pv`, and `cm`, if a developer wants only to see `svc`, and `pv`
// logging, it's achieved just setting `SYPL_FILTER="svc,pv"`.
//
// `SYPL_DEBUG` allows to specify the max level, for example, for a given
// application with the following loggers: `svc`, `pv`, and `cm`, if a developer
// sets:
//   - `SYPL_DEBUG="debug"`: any application running using Sypl, any component,
//     any output, will log messages bellow the `debug` level
//   - `SYPL_DEBUG="console:debug"`: any application running using Sypl with an
//     output called `console`, will log messages bellow the `debug` level
//   - `SYPL_DEBUG="warn,console:debug"`: any application running using Sypl, any
//     component, any output, will log messages bellow the `warn` level, AND any
//     application running using Sypl with an output called `console`, will log
//     messages bellow the `debug` level. NOTE that `warn` is specified first.
//     Only for this case - global max level scope, it's a requirement! In this
//     case -> `SYPL_DEBUG="console:debug,warn"`, `warn` will be discarded.
//   - `SYPL_DEBUG="svc:console:debug"`: any application running using Sypl with a
//     component called `svc` with an output called `console`, will log messages
//     bellow the `debug` level
//   - `SYPL_DEBUG="file:warn,svc:console:debug"`: any application running using
//     Sypl with an output called `file` will log messages bellow the `warn`
//     level, and any application running using Sypl with a component called `svc`
//     with an output called `console` will log messages bellow the `debug`.
//
// The possibilities are endless! Checkout the [`debugAndFilter`](example_test.go)
// for more.
//
// # Hot-path performance
//
// Two OPT-IN mechanisms keep the cost of dropped messages near zero:
//
//   - Fast level gate (`SetFastGate(true)`): option-less Print-family calls
//     whose level no enabled output can write return BEFORE any message
//     construction. Processors cannot resurrect a gated-out message - the
//     same contract as slog/zap. Fatal is never gated, and the gate defers
//     to the slow path while `SYPL_LEVEL`/`SYPL_FILTER` are set.
//   - Lazy message identity: the message UUID, and the content-based hash
//     are computed - and memoized - only when first read (e.g. by the JSON
//     formatter). Per-output copies share the computation, observing one
//     identity per message.
//
// Additionally, a message going to a SINGLE output is written inline on the
// calling goroutine - multiple outputs keep the concurrent fan-out.
//
// # Structured logging conveniences
//
//   - `With(fields)` returns a derived logger sharing the parent's outputs,
//     with its own merged copy of the fields, and tags - reconfiguring one
//     never leaks into the other.
//   - `Infow`/`Debugw`/`Tracew`/`Warnw`/`Errorw`/`Fatalw`/`Logw` accept
//     alternating key-value pairs, slog/zap-style - malformed pairs are
//     tolerated, never panicking.
//   - `NewContext`/`FromContext`/`FromContextOrDefault` carry a logger
//     through a `context.Context`; `SetContextExtractor` +
//     `PrintWithContext` (and the leveled `*WithContext` variants) pull
//     structured fields out of one - sypl imports no tracing library, the
//     application wires its own extractor.
//
// # Lifecycle
//
//   - `SetErrorHandler` receives every output write error - wrapped with the
//     failing output's name - instead of the default silent swallow.
//   - `Flush`/`Close` walk the outputs in registration order, calling the
//     ones implementing `interface{ Flush() error }`/`io.Closer`, and
//     aggregate all errors via `errors.Join`. Fatal flushes (best-effort,
//     time-bounded - a hung sink can't keep the process alive) before
//     exiting.
package sypl
