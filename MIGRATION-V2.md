# Migrating from sypl v1 to v2

v2 ships exactly three breaking changes:

1. **Module path `/v2` + ElasticSearch isolated into the nested `es/` submodule** — the
   core module no longer depends on `go-elasticsearch` (a hello-world consumer compiles
   145 packages instead of 952).
2. **Conventional level ordering** — `Warn` and `Info` swapped numeric places, so
   verbosity nests conventionally (`SetMaxLevel(Info)` now **shows** warnings).
3. **Dead API removal** — a handful of setters/lookups with no known callers were
   deleted (drop-in replacements below).

---

## 1. Imports: old → new

| v1 import | v2 import |
|---|---|
| `github.com/thalesfsp/sypl` | `github.com/thalesfsp/sypl/v2` |
| `github.com/thalesfsp/sypl/level` (and every other non-ES subpackage) | `github.com/thalesfsp/sypl/v2/level` (same name, `/v2/` inserted) |
| `github.com/thalesfsp/sypl/elasticsearch` | `github.com/thalesfsp/sypl/v2/es` (separate Go module — `go get github.com/thalesfsp/sypl/v2/es`) |
| `github.com/thalesfsp/sypl/output` *(only the `ElasticSearch*` symbols)* | `github.com/thalesfsp/sypl/v2/es` |

ES symbols: old → new (runtime output **names** — `GetName()`/`GetOutput(...)` — are unchanged):

| v1 symbol | v2 symbol |
|---|---|
| `output.ElasticSearch(...)` | `es.Output(...)` |
| `output.ElasticSearchWithDynamicIndex(...)` | `es.OutputWithDynamicIndex(...)` |
| `output.ElasticSearchWithTagMap(...)` | `es.OutputWithTagMap(...)` |
| `output.ElasticSearchBulk(...)` | `es.BulkOutput(...)` |
| `output.ElasticSearchBulkWithDynamicIndex(...)` | `es.BulkOutputWithDynamicIndex(...)` |
| `output.ElasticSearchConfig` | `es.Config` |
| `output.ElasticSearchDynamicIndexFunc` | `es.DynamicIndexFunc` |
| `output.ElasticSearchTagMap` | `es.TagMap` |
| `output.ElasticSearchTagMapItem` | `es.TagMapItem` |
| `output.NewElasticSearchTagMapItem` | `es.NewTagMapItem` |
| `output.ElasticSearchBulkOption` | `es.BulkOption` |
| `elasticsearch.New` / `NewWithDynamicIndex` / `NewBulk` / `NewBulkWithDynamicIndex` | `es.New` / `es.NewWithDynamicIndex` / `es.NewBulk` / `es.NewBulkWithDynamicIndex` |
| `elasticsearch.Config`, `elasticsearch.DynamicIndexFunc`, `elasticsearch.BulkOption`, `elasticsearch.BulkWith*`, `elasticsearch.ErrBulkClosed`, `elasticsearch.ElasticSearch`, `elasticsearch.ElasticSearchBulk` | same names, package `es` |

### Worked example: the shared `internal/logging` template

The near-universal v1 wiring (`sypl.NewDefault` + tag-mapped ES outputs):

```diff
 import (
-	"github.com/thalesfsp/sypl"
-	"github.com/thalesfsp/sypl/level"
-	"github.com/thalesfsp/sypl/output"
-	"github.com/thalesfsp/sypl/processor"
+	"github.com/thalesfsp/sypl/v2"
+	"github.com/thalesfsp/sypl/v2/es"
+	"github.com/thalesfsp/sypl/v2/level"
+	"github.com/thalesfsp/sypl/v2/processor"
 )

 func NewLogger(name string) *sypl.Sypl {
-	esConfig := output.ElasticSearchConfig{Addresses: []string{esAddress}}
+	esConfig := es.Config{Addresses: []string{esAddress}}

 	l := sypl.NewDefault(name, level.Info)

-	l.AddOutputs(output.ElasticSearchWithTagMap(
-		map[string]output.ElasticSearchTagMapItem{
-			"audit": output.NewElasticSearchTagMapItem(level.Info, func() string { return "audit-" + today() }),
-			"*":     output.NewElasticSearchTagMapItem(level.Debug, func() string { return "app-" + today() }),
+	l.AddOutputs(es.OutputWithTagMap(
+		es.TagMap{
+			"audit": es.NewTagMapItem(level.Info, func() string { return "audit-" + today() }),
+			"*":     es.NewTagMapItem(level.Debug, func() string { return "app-" + today() }),
 		},
 		esConfig,
 		processor.Tagger("app"),
 	)...)

 	return l
 }
```

And in `go.mod` (the es module is versioned, and tagged separately):

```
require (
	github.com/thalesfsp/sypl/v2 v2.0.0
	github.com/thalesfsp/sypl/v2/es v2.0.0
)
```

> **Note (repo developers only):** `es/go.mod` carries a development-time
> `replace github.com/thalesfsp/sypl/v2 => ../` so both modules build from the working
> tree. It has no effect on consumers — `replace` directives only apply to the main
> module. Releases must tag **both** `v2.0.0` *and* `es/v2.0.0`.

---

## 2. Level ordering

| Level | v1 value | v2 value |
|---|---|---|
| `None` | 0 | 0 |
| `Fatal` | 1 | 1 |
| `Error` | 2 | 2 |
| `Warn` | **4** | **3** |
| `Info` | **3** | **4** |
| `Debug` | 5 | 5 |
| `Trace` | 6 | 6 |

Only `Warn` and `Info` swapped. Consequences:

- **Visibility:** an output capped at `Info` now **shows** `Warn` (v1 hid it); an output
  capped at `Warn` now **hides** `Info`. Each cap admits every level at or below it —
  the conventional nesting.
- **`FromInt`:** `level.FromInt(3)` is now `Warn` (was `Info`); `level.FromInt(4)` is now
  `Info` (was `Warn`). Audit any persisted numeric levels (DB columns, env vars, wire
  formats) before upgrading.
- **Unaffected:** `FromString`, `MustFromString`, and `String()` — names still map to
  themselves. If you configure levels by name (`"info"`, `"warn"`, `SYPL_LEVEL`,
  `SYPL_DEBUG`), only the *visibility* consequence above applies.

---

## 3. Removed APIs: old → new

| Removed | Replacement |
|---|---|
| `IMeta.SetName` (on `Sypl`, outputs, processors) | Construct with the right name: `sypl.New("name", ...)`, `output.New("name", maxLevel, w, ...)`, `processor.New("name", fn)` |
| `IOutput.GetProcessor(name)` | Iterate `GetProcessors()` (match with `strings.EqualFold`), or check `GetProcessorsNames()` |
| `IOutput.SetBuiltinLogger(...)` | Redirect the existing one: `o.GetBuiltinLogger().SetOutput(w)` — or construct the output with the right writer |
| `IMessage.SetContent(...)` | Construct a new message (`message.New(level, content)`); to mutate in-pipeline text use `m.GetContent().SetProcessed(...)` |
| `IMessage.SetLevel(...)` | Construct the message at the right level |
| `Sypl.AnyMaxLevel(l)` | Iterate `GetMaxLevel()` — snippet below |
| `debug.MatchL` / `MatchOL` / `MatchCOL` | Use `debug.Debug.Level()` (the matchers are now internal) |
| `shared.DefaultComponentNameOutput` / `DefaultContentOutput` / `DefaultPrefixValue` / `DefaultTimestampFormat` | They were sypl's own test fixtures — inline your own constants |

### `AnyMaxLevel` replacement (the `GetLogger().AnyMaxLevel(...)` guard)

v1 pattern:

```go
if p.GetLogger().AnyMaxLevel(level.Debug) || p.GetLogger().AnyMaxLevel(level.Trace) {
	// verbose-mode work...
}
```

Exact drop-in equivalent of v1 `AnyMaxLevel` semantics (some output's cap **equals** `l`,
or `SYPL_LEVEL` names it):

```go
// anyMaxLevel reports whether any output's maxLevel equals l, or the
// SYPL_LEVEL env var names it - v1 Sypl.AnyMaxLevel semantics.
func anyMaxLevel(l *sypl.Sypl, target level.Level) bool {
	for _, ml := range l.GetMaxLevel() { // map[outputName]level.Level
		if ml == target {
			return true
		}
	}

	return os.Getenv(shared.LevelEnvVar) == target.String()
}
```

For the guard above, the v2-idiomatic form is usually what was *meant* — "is any output
verbose enough to admit Debug?" — which under v2's conventional nesting is a single
`>=` check (and subsumes the `|| Trace` half):

```go
// anyOutputAdmits reports whether any output would print messages at `target`.
func anyOutputAdmits(l *sypl.Sypl, target level.Level) bool {
	for _, ml := range l.GetMaxLevel() {
		if ml >= target {
			return true
		}
	}

	return false
}

// v1: p.GetLogger().AnyMaxLevel(level.Debug) || p.GetLogger().AnyMaxLevel(level.Trace)
// v2: anyOutputAdmits(p.GetLogger(), level.Debug)
```

---

## The mechanical 90%: sed one-liners

Run from a consumer repo root (GNU sed; on macOS use `sed -i ''`):

```sh
# 1. Module path: every sypl import -> /v2 (idempotent-safe: run once).
find . -name '*.go' -exec sed -i 's|github.com/thalesfsp/sypl|github.com/thalesfsp/sypl/v2|g' {} +

# 2. ES imports: the old elasticsearch package -> the es module.
find . -name '*.go' -exec sed -i 's|github.com/thalesfsp/sypl/v2/elasticsearch|github.com/thalesfsp/sypl/v2/es|g; s|elasticsearch\.|es.|g' {} +

# 3. ES output factories, and types: output.ElasticSearch* -> es.*.
find . -name '*.go' -exec sed -i 's|output\.NewElasticSearchTagMapItem|es.NewTagMapItem|g; s|output\.ElasticSearchTagMapItem|es.TagMapItem|g; s|output\.ElasticSearchTagMap|es.TagMap|g; s|output\.ElasticSearchConfig|es.Config|g; s|output\.ElasticSearchDynamicIndexFunc|es.DynamicIndexFunc|g; s|output\.ElasticSearchBulkOption|es.BulkOption|g' {} +

# 4. ES output factory calls: WithX variants first (longest match first).
find . -name '*.go' -exec sed -i 's|output\.ElasticSearchBulkWithDynamicIndex(|es.BulkOutputWithDynamicIndex(|g; s|output\.ElasticSearchBulk(|es.BulkOutput(|g; s|output\.ElasticSearchWithDynamicIndex(|es.OutputWithDynamicIndex(|g; s|output\.ElasticSearchWithTagMap(|es.OutputWithTagMap(|g; s|output\.ElasticSearch(|es.Output(|g' {} +

# 5. Removed output APIs: builtin-logger redirect keeps working via SetOutput.
find . -name '*.go' -exec sed -i 's|\.SetBuiltinLogger(builtin\.NewBuiltin(\(&[A-Za-z0-9_]*\), "", 0))|.GetBuiltinLogger().SetOutput(\1)|g' {} +

# 6. Then fetch the new modules, and tidy.
go get github.com/thalesfsp/sypl/v2@v2.0.0 github.com/thalesfsp/sypl/v2/es@v2.0.0 && go mod tidy
```

What the seds do **not** cover (audit by hand):

- `FromInt`, or any persisted **numeric** level (section 2).
- Behavior relying on `SetMaxLevel(Info)` hiding warnings (section 2).
- `SetName`/`GetProcessor`/`SetContent`/`SetLevel`/`AnyMaxLevel` call sites (section 3).
