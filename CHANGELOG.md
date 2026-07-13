# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## Roadmap

- Add LowerCase processor
- Add badges to README.md:
  - GoDoc
  - Go Report Card
  - Build Status
  - Coverage Status
  - License
  - Release
- Flag should be an array.
- Move `Options`' methods from `Message` to `Options`
- Improve documentation:
  - Add `doc.go` for all packages

Refs. for badges:

- http://github.com/wayneashleyberry/terminal-dimensions
- https://github.com/golangci/golangci-lint

## [2.0.0] - 2026-07-13

SEMVER-MAJOR release: exactly three breaking changes. See
[MIGRATION-V2.md](MIGRATION-V2.md) for the old→new tables, worked examples,
and the mechanical sed one-liners.

### Breaking
- **Module path is now `github.com/thalesfsp/sypl/v2`** (Go semantic import
  versioning). Every import gains the `/v2` segment.
- **ElasticSearch support moved into the nested `es/` submodule**
  (`github.com/thalesfsp/sypl/es/v2`): the former `elasticsearch` package
  (client, bulk, utils) plus the ES output factories formerly in `output`
  (`ElasticSearch` → `es.Output`, `ElasticSearchWithDynamicIndex` →
  `es.OutputWithDynamicIndex`, `ElasticSearchWithTagMap` →
  `es.OutputWithTagMap`, `ElasticSearchBulk` → `es.BulkOutput`,
  `ElasticSearchBulkWithDynamicIndex` → `es.BulkOutputWithDynamicIndex`, and
  the `Config`/`TagMap`/`TagMapItem`/`NewTagMapItem`/`BulkOption` types).
  The core module now depends only on `fatih/color`, `google/uuid`, and
  `acarl005/stripansi` — a non-ES consumer compiles 145 packages instead of
  952. Runtime output names (`GetName`) are unchanged.
- **Conventional level ordering**: `Warn(3)` now sits below `Info(4)` —
  `SetMaxLevel(Info)` shows warnings, and `FromInt(3)`/`FromInt(4)` mean
  `Warn`/`Info`. Name-based lookups (`FromString`/`String`) are unaffected.
- **Dead API removal** (zero known callers): `IMessage.SetContent`,
  `IMessage.SetLevel`, `IOutput.GetProcessor`, `IOutput.SetBuiltinLogger`,
  `IMeta.SetName` (and all impls), `Sypl.AnyMaxLevel`,
  `debug.MatchL/MatchOL/MatchCOL` (unexported), the `shared.Default*` test
  fixtures, and the unused stdlib-logger machinery inside
  `internal/builtin`.

### Added
- `output.Proxy`/`output.NewProxy`: the forwarding proxy (self-returning
  chainable setters) is exported, so external packages — like `es` — can
  build capability-carrying output wrappers.
- `es/` ships as its own Go module: `go get github.com/thalesfsp/sypl/es/v2`
  only when you log to ElasticSearch. CI lints, and tests it separately;
  releases tag both `v2.x.y`, and `es/v2.x.y`.
- MIGRATION-V2.md: complete old→new mapping, level table, `AnyMaxLevel`
  replacement snippet, and sed one-liners covering the mechanical 90%.

## [1.21.0] - 2026-07-13
### Added
- Opt-in hot-path fast gate: `SetFastGate(true)` makes filtered-out levels
  cost ~38ns / 0 allocs (default off; Fatal, forced flags, and `SYPL_LEVEL`/
  `SYPL_FILTER` always take the full path).
- `With(fields)`: cheap derived logger with merged fields and unshared
  containers; inherits outputs, error handler, gate, and context extractor.
- Key-value sugar: `Infow`, `Debugw`, `Tracew`, `Warnw`, `Errorw`, `Fatalw`,
  `Logw` (slog-style pairs; never panics on malformed pairs).
- Context helpers: `NewContext`, `FromContext`, `FromContextOrDefault`,
  `SetContextExtractor`, and `*WithContext` printers (bring-your-own tracing
  extractor; no OpenTelemetry dependency).
- Lifecycle: `Flush()` / `Close()` on the logger with capability detection
  (`Flush() error` / `io.Closer`) and `errors.Join` aggregation. `Fatal`
  flushes buffered outputs before exiting, bounded by a 10s timeout so the
  process always terminates.
- `SetErrorHandler(func(error))`: output write errors (previously silently
  discarded) are delivered wrapped with the output name.
- `output.Async(...)`: bounded buffered wrapper for any output — Block /
  DropNewest / DropOldest policies, drop notifications, periodic flush,
  snapshot-semantics `Flush` (everything enqueued before the call), panic
  containment for the wrapped output, graceful `Close`, no goroutine leaks.
- `output.ElasticSearchBulk(...)` (+ dynamic-index variant) built on
  `esutil.BulkIndexer`: batched `_bulk` indexing with per-item error
  callbacks; payloads are kept single-line NDJSON-safe (multi-line JSON is
  compacted; invalid payloads are rejected instead of corrupting the
  stream).
- `output.RotatingFile(...)`: native size-based rotation with backup count
  and age pruning; self-heals after mid-rotation failures (writes keep
  landing in the original file and errors stay visible — never silent loss).
- `output.Recorder(...)`: structured test-assertion output capturing level,
  content, fields, tags, and names with defensive copies.
- `processor.Sample(...)`: zap-style first-N-then-every-Mth sampling per key
  and window, bounded internal state.
- `processor.RateLimit(...)`: global per-window limiter with overflow
  callback.
- `processor.Dedup(...)`: log-once-per-window with suppression counters,
  keyed by the (now lazy) content hash by default.
- `syplslog`: bidirectional `log/slog` bridge — `NewHandler` (passes stdlib
  `slogtest`; slog-speaking libraries log through sypl pipelines) and
  `Output` (sypl logs into any `slog.Handler`); slog records can never
  trigger sypl's `Fatal` exit.

### Changed
- Performance: single-output prints ~−44%, `message.New` ~−64%, muted-level
  slow path ~−40% (UUID and content-hash generation now lazy and memoized
  per message family; single-receiver writes run inline without goroutine
  fan-out). Benchmarks live in-repo; numbers are benchstat-verified.
- Edge-case note: the unexported message struct's `ID` and
  `ContentBasedHashID` fields became lazy internals; code reaching through
  `GetMessage().ID` must use `GetID()` / `GetContentBasedHashID()` instead
  (no known consumers were affected).

## [1.20.1] - 2026-07-12
### Changed
- Test suite and directives conform to the repository's golangci-lint gate
  (no library behavior changes).

## [1.20.0] - 2026-07-12
### Fixed
- Data race: `Sypl` and `output` are now safe for concurrent reconfiguration
  (`SetMaxLevel`, `SetTags`, `SetFields`, `AddOutputs`, `SetStatus`, ...) while
  logging. Includes the parent/child logger family: child loggers created via
  `New(name)` no longer share tag/field/output containers with the parent.
- Data race: `message.Copy` now deep-copies fields; per-output goroutines no
  longer share one map. Double-`Fatal` no longer races on the exit flag.
- `flag.SkipAndMute` is now honored: the message is neither processed nor
  printed, matching its documentation (was fully inert for non-empty messages).
- CRLF (`\r\n`) line endings survive `Strip`/`Restore` (were restored as `\n\r`).
- Output and processor dispatch match names exactly (case-insensitive) instead
  of by substring: targeting output `es-backup` no longer also writes to `es`.
  `SYPL_FILTER` entries likewise match component names exactly.
- `ChangeFirstCharCase` no longer corrupts multi-byte first characters (`élan`
  → `Élan`, was mojibake).
- `SYPL_DEBUG` entries no longer leak level filters across components/outputs
  via prefix matches (e.g. `infosvc:...` no longer acts as global `info`).
- ElasticSearch output returns errors instead of panicking on non-string
  document IDs and unexpected response shapes.
- `WithField` no longer panics after `WithFields(nil)`.
- `ElasticSearchWithTagMap` and `NewDefault` no longer alias caller-provided
  processors slices (spare-capacity slices corrupted output filtering).
- `JSONPretty` formatter registers under its own name (was `JSON`).

### Changed
- Performance: messages are copied only for outputs that will actually write;
  copies skip UUID/SHA-1 regeneration; tags use a plain map instead of a
  red-black tree; level-filter sets are precomputed; PID is cached; `SYPL_DEBUG`
  regexes are compiled once per component/output pair (bounded cache).

### Removed
- Dependencies: `emirpasic/gods`, `golang.org/x/sync`, `spf13/afero`,
  `go-test/deep` (direct deps 8 → 4, zero behavior change).
- Dead package-level functions in `internal/builtin` (not importable
  externally).

## [1.9.14] - 2023-03-23
### Changed
- Upgraded lint to v1.51.2.
- Now, IBasicPrinter interface contains only native Print{f|ln} methods.

## [1.9.13] - 2022-11-08
### Changed
- Now `Breakpoint` prints the message independently of the level.

## [1.9.12] - 2022-11-08
### Added
- Added `WithID` `Option`.

## [1.9.11] - 2022-11-08
### Changed
- Only fields with values will be printed.

## [1.9.10] - 2022-11-08
### Added
- `Flagger` `Processor` which flags messages based on a given `Flag`.

## [1.9.3] - 2022-10-27
### Added
- `File` `Output` should only log message if no path is provided.

## [1.9.2] - 2022-10-27
### Added
- `Tagger` processor, and example.
- `File` `Output` will create a temp file in the OS's temp directory if the no `path` provided.

## [1.9.1] - 2022-10-27
### Changed
- `File` `Output` now tries to create the directory if it doesn't exist.

## [1.9.0] - 2022-10-27
### Added
- `JSON` formatter which outputs inline non-prettified JSON.

### Changed
- Renames the `JSON` formatter to `JSONPretty`.
- Fix bug in `generateID` which added a trailing newline to the generated ID.

## [1.8.0] - 2022-10-17
### Changed
- Renames `SYPL_DEBUG` to `SYPL_LEVEL`.

## [1.7.4] - 2022-10-04
### Changed
- Only sets ElasticSearch document ID if any.

## [1.7.3] - 2022-10-04
### Added
- Only process global fields, and tags if any.
- Added tests which ensures tags aren't duplicated.

## [1.7.2] - 2022-10-04
### Added
- Added the ability to specify global tags.

## [1.7.1] - 2022-09-27
### Changed
- Removed CodeQL.

## [1.7.0] - 2022-09-27
### Changed
- Removed `Print{f|lnf}WithOptions` in favor of `PrintWithOptions` (functional).

## [1.6.8] - 2022-09-25
### Changed
- Added guard for the `ChangeFirstCharCase` processor.

## [1.6.7] - 2022-09-25
### Changed
- Cleaned `text` formatter.

## [1.6.6] - 2022-09-25
### Changed
- General improvement for the ES output.
- `SYPL_ELASTICSEARCH_TEST_ADDRESS` sets the ES address for the integration test.

## [1.6.5] - 2022-09-23
### Added
- Added the ability to specify options in a functional way.

## [1.6.4] - 2022-09-23
### Added
- Added `output.ElasticSearchWithDynamicIndex`.

## [1.6.3] - 2022-09-23
### Added
- Added `elasticsearch.NewWithDynamicIndex` that allows specifying a function that defines the name of the index, evaluated at the time of index. An important feature, part of the index naming strategy.

## [1.6.2] - 2022-09-23
### Added
- `ElasticSearch` `output` built-in

## [1.6.1] - 2022-09-03
### Changed
- Fixed documentation.

## [1.6.0] - 2022-09-02
### Changed
- Removed Lumberjack dependency

## [1.5.12] - 2022-09-02
### Changed
- Add status badge
- Update dependencies, and documentation.

## [1.5.11] - 2022-02-25
### Changed
- Fix missing field in copy fields to child logger.

## [1.5.10] - 2022-02-22
### Changed
- Fixed `New`, was missing setting `defaultIoWriterLevel`, `fields`, `status`.
- Changed default `io.Writer` level to `None`.

## [1.5.9] - 2022-02-21
### Added
- For convenience, conforms with `io.Writer` interface. Default level: `error`. `SetIoWriterLevel` changes the default level.

### Changed
- Fixed chained example.
- Lowercased all levels.
- It now warns when an application tries to write to a closed writer.

## [1.5.8] - 2021-11-08
### Changed
- All `SetXYZ` methods returns its proper interface allowing method chaining.
- `Breakpoint` is now variadic.
- Properly handle cases where sypl writes to a piped output, but it's broken.

## [1.5.7] - 2021-11-02
### Changed
- Fixed `ExampleNew_globalFields` test.

## [1.5.6] - 2021-11-02
### Added
Added the ability to set breakpoints. If a `Breakpoint` is set it'll stop execution waiting the user press `/n` (**"enter"**) to continue. It helps users doing quick, and effective log-to-console debug. A message with the breakpoint `name`, and `PID` of the process will be printed using the `debug` level. Arbitrary `data` can optionally be set - if set, it'll be printed. Errors are printed using the standard `error` level. Set logging level to `trace` for more.

Previously, flow would look like:
- Log markers are set, e.g.: `logger.Debugln("Here 1", whatever)`
- Application runs
- Scan visually `output`, or a `file` - via `grep` for the markers.

Now:
- Named `Breakpoint`s are set
- Application runs
- Breakpoint is hit. Information about it is printed.
- Runtime is paused, allowing analysis of `data` - if any, right way. Additionally, an external and more advanced debugger can be attached.
- Dev controls the flow, pressing `enter` at any time, continue.

## [1.5.5] - 2021-10-29
### Changed
- Exported `sypl.Name` to deal with https://github.com/golang/go/issues/5819.

## [1.5.4] - 2021-10-13
### Added
In a application with many loggers, and child loggers, sometimes more fine control is needed, specially when debugging applications. Sypl offers two powerful ways to achieve that: `SYPL_FILTER`, and `SYPL_DEBUG` env vars.

`SYPL_FILTER` allows to specify the name(s) of the component(s) that should be logged, for example, for a given application with the following loggers: `svc`, `pv`, and `cm`, if a developer wants only to see `svc`, and `pv` logging, it's achieved just setting `SYPL_FILTER="svc,pv"`.

`SYPL_DEBUG` allows to specify the max level, for example, for a given application with the following loggers: `svc`, `pv`, and `cm`, if a developer sets:

- `SYPL_DEBUG="debug"`: any application running using Sypl, any component, any output, will log messages bellow the `debug` level
- `SYPL_DEBUG="console:debug"`: any application running using Sypl with an output called `console`, will log messages bellow the `debug` level
- `SYPL_DEBUG="warn,console:debug"`: any application running using Sypl, any component, any output, will log messages bellow the `warn` level, AND any application running using Sypl with an output called `console`, will log messages bellow the `debug` level.

_NOTE: `warn` is specified first. Only for this case - **global scope**, it's a requirement.
`SYPL_DEBUG="console:debug,warn"`: In this case `warn` will be **discarded!**._

- `SYPL_DEBUG="svc:console:debug"`: any application running using Sypl with a component called `svc` with an output called `console`, will log messages bellow the `debug` level
- `SYPL_DEBUG="file:warn,svc:console:debug"`: any application running using Sypl with an output called `file` will log messages bellow the `warn` level, and any application running using Sypl with a component called `svc` with an output called `console` will log messages bellow the `debug`.

Possible scopes:

- `{componentName:outputName:level}`: Component, and output scoped.
- `{outputName:level}`: Output scoped.
- `{level}`: Global scope.

The possibilities are endless! Checkout the [`debugAndFilter`](example_test.go) example for more.
### Changed
- Renamed logging component filtering env var from `SYPL_DEBUG` to `SYPL_FILTER`.

## [1.5.3] - 2021-09-21
### Changed
- Fix bug where setting fields for a message would set globally too.

## [1.5.2] - 2021-09-21
### Changed
- Level `FromString`, and `MustFromString` methods validates if `level` param is empty.

## [1.5.1] - 2021-09-10
### Changed
- Sypl `SetFields` is chainable.

## [1.5.0] - 2021-09-10
### Added
- Adds the ability to set global Fields.

## [1.4.6] - 2021-08-30
### Changed
- `FromString` error now prints also available levels.
- `LevelsNames` returns lower-cased levels.

## [1.4.5] - 2021-08-30
### Changed
- `StdErr` now only prints `Error` AND `Fatal` instead of only `Error`.
- `Console` now ignores `Error` AND `Fatal` instead of only `Error`.
- `PrintOnlyAtLevel` now handle multiples levels.
- `FromString` now returns the level, and error instead of level, and bool (ok).
- Internal `sypl.process` is now validated. In case of failure it throws `ErrSyplNotInitialized`.
- All `error.go` files were renamed to `errors.go`, following Go standards.

## [1.4.4] - 2021-08-20
### Added
- Adds `PrintNewLine`.

### Changed
- `Skip` and `SkipAndForce` flags now skips formatters too.

## [1.4.3] - 2021-08-20
### Changed
- Removes unused entries from `Makefile`.
- `sypl.New` now returns `*Sypl`.

## [1.4.2] - 2021-08-19
### Added
- Adds `PrintMessagesToOutputsWithOptions`.

## [1.4.1] - 2021-08-19
### Changed
- Allows to specify the name of `dashHandler` output.
    - Now, when `-` is specified as a path, `dashHandler` is named after the original output.

## [1.4.0] - 2021-08-18
### Changed
- Fixed names of the factories, so it doesn't stutters.

NOTE: Breaking change.

## [1.3.11] - 2021-08-18
### Added
- Adds `LevelsNames`.

## [1.3.10] - 2021-08-18
### Changed
- Improved `FromString`.

## [1.3.9] - 2021-08-18
### Added
- Adds `MustFromString`.

## [1.3.8] - 2021-08-18
### Added
- Adds the ability to get and set outputs' max level.

## [1.3.7] - 2021-08-17
### Changed
- `NewDefault` only prints errors to `stderr`.

## [1.3.6] - 2021-08-17
### Added
- Adds `PrintOnlyIfTagged` built-in processor.

### Changed
- Renames `PrintOnlyLevel` to `PrintOnlyAtLevel`.

## [1.3.5] - 2021-08-17
### Changed
- `StdErr` only prints @ `Error` `Level`.

## [1.3.4] - 2021-08-17
### Added
- Creates `StdErr` built-in `Output`.

### Changed
- Removes `path` (unused) from `FileBased` `Output`.

## [1.3.3] - 2021-08-14
### Changed
- Improved linebreak detection and restoration.

## [1.3.2] - 2021-08-13
### Added
- Adds `PrintMessagerPerOutput` which allows you to concurrently print messages, each one, at the specified level and to the specified output. If the named output doesn't exits, the message will not be printed.
    - Cover with test.

### Changed
- Adds `output` field to `Text` and `JSON` formatters.

## [1.3.1] - 2021-08-11
### Added
- Adds the ability to create child loggers (`New`). The child logger is an accurate, and efficient shallow copy of the parent logger. Changes to internals, such as the state of outputs, and processors, are reflected cross all other loggers.
- Adds `Text`, and `JSON` formatters. It also process fields. See `example_test.go/ExampleNew_textFormatter` and `example_test.go/ExampleNew_jsonFormatter` for examples. Both formatters automatically adds:
    - Component name
    - Level
    - Timestamp (RFC3339).
- Add more tests. Covered `ErrorSimulator` processor.
- Adds ability to filter logging message. See `example_test.go/ExampleNew_childLoggers` for example. Having many loggers can be, sometimes, noisy. Also, sometimes - for debugging reason, you may want to see only `componentA`, and `componentC`. Now, it's possible. Just specify the name of the components (comma-separated list) in the `SYPL_DEBUG` env var.

## [1.3.0] - 2021-08-10
### Added
- Adds support for structured logging.
    - See `example_test.go/ExampleNew_fieldsProcessing`.
- Components are interface(behaviour)-driven (design-pattern).
- Components are Factory built (design-pattern).
- Adds `Buffer` built-in `output`, it's a concurrent-safe buffer.
- Refactored code, components are packaged.

## [1.2.5] - 2021-07-22
### Added
- Adds the `Decolourizer` processor.

## [1.2.4] - 2021-07-16
### Changed
- Go mod checksum.

## [1.2.3] - 2021-07-15
### Added
- Adds `Sprint{f|lnf|ln}`, and `{Level}{f|lnf|ln}` Convenient methods. It's your `Sprint`, or `Sinfo` (example) but also returning the non-processed content.

Before:

```go
// ...
var errMsg := "Some error"

logger.Errorln(errMsg)

return errors.New(errMsg)
```

Now:

```go
// ...
return logger.Serrorln("Some error") // Prints and process like `Errorln`, and returns an error.
```

## [1.2.2] - 2021-07-15
### Changed
- Fixes `Flag`s processing logic.
- Covers `Flag`s with test.

## [1.2.1] - 2021-07-15
### Changed
- Fixes `prettify` not printing the error if it fails.

## [1.2.0] - 2021-07-14
### Added
- Finer-control on message's behaviour with two new `Flags`: `SkipAndForce` and `SkipAndMute`.
- Adds `Printlnf`, and `{Level}{lnf}` Convenient methods. It's your `Printf`, or `Infof` (example) without the need to add `"\n"` to the format - less annoying repetition.

Before:

```go
// ...
exampleContent := "example"
logger.Printf("Something %s\n", exampleContent)
```

Now:

```go
// ...
exampleContent := "example"
logger.Printlnf("Something %s", exampleContent)
```

### Changed
- Improves testability, and maintainability: All "Convenient methods" are based on "Base methods" that are based on the implementation of the interface.
    - Testability: You mock the interface, and have full control over how it works.
    - Maintainability: You change the interface implementation, you change how everything works.

## [1.1.2] - 2021-07-13
### Changed
- Fix typo (`spyl`).

## [1.1.1] - 2021-07-13
# Added
- Adds `Print{ln}Pretty` which allows to print data structures as JSON text.

Now:

```go
// ...
logger.PrintlnPretty(&SomeStruct{
    nonExportedKey: "Value1",
    SomeExportedKey: "Value2",
})

// Prints:
// {
//     "SomeExportedKey": "Value2"
// }
```

### Changed
- Prefixes sypl errors making it easier to identify when happens.
- Fixes a bug in `level.FromString` where invalid string would call `log.Fatal`.

## [1.1.0] - 2021-07-13
### Added
- Adds the ability to tag a message, see new `Print{f,ln}WithOptions` example.
- Adds the ability to flag a message, see new `Skip` flag.
- Adds `Print{f,ln}WithOptions` which allows to specify message's `Options` such as a list of `Output`s and `Processor`s to be used.
- Functional approach: no direct-access to data structure properties.
- Adds more examples.
- Adds more tests.
- Adds more documentation.
- Extracted `Flag`, `Content` and `Level` to packages.

## [1.0.0] - 2021-07-08
### Added
- First release.
