# sypl — critical defect plan and implementation

Base: fbfa24c07311e866727352e249bcb4e2db514769 (origin/master). Module: github.com/thalesfsp/sypl/v2.
Evidence: /Volumes/Dev/Dev/critical-review-20261116/sypl/probes/regexpanic.log, probe at
/Volumes/Dev/Dev/critical-review-20261116/sypl/probes/regexpanic/main.go.

## C1 — logger/output names are compiled as raw regex when SYPL_LEVEL is set → unrecoverable panic kills the host process

Severity rationale: a public logging call (`Info`, `Println`, any Print-family call, `PrintMessage`,
the slog handler) terminates the whole host process with exit 2. The panic happens on a goroutine
the library spawns inside `Sypl.process`, so the caller cannot `recover` it. It needs only the
documented `SYPL_LEVEL` debug env var (often turned on in production during an incident) plus a
component name or output name containing a regex metacharacter that makes an invalid pattern
(`(`, `++`, a leading `*`/`?`/`+`, `\q`, `x{9999}`). Library-generated output names carry
caller-supplied text: `ElasticSearchWithTagMap-<tag>`,
`ElasticSearchBulkWithDynamicIndex-<index>`.

Original source (before the critical fix, after dependency compatibility edits):
- debug/debug.go:191-199 `newMatchers` — `regexp.MustCompile(fmt.Sprintf(oLReMask, outputName, levels))`
  and `regexp.MustCompile(fmt.Sprintf(cOLReMask, componentName, outputName, levels))`, with no `regexp.QuoteMeta`.
- debug/debug.go `New` (called per message) → `newMatchers` on a cache miss.
- sypl.go:891-895 `processOutputs` calls `debug.New(msg.GetComponentName(), msg.GetOutputName())`
  whenever `os.Getenv("SYPL_LEVEL") != ""`.
- sypl.go:714 the work runs in a goroutine started by `wg.Go(func() { ... })` in `process`, so a panic there is fatal to the process.

Triggering scenario (reproduced, exit code 2):
```
SYPL_LEVEL=info ./probe 'api(v2'      # sypl.New("api(v2", output.SafeBuffer(...)).Infoln("hello")
SYPL_LEVEL=info ./probe 'C++ service'
```
Output: `panic: regexp: Compile(...(?:api(v2):(?:Buffer)...): error parsing regexp: missing closing )`
and `invalid nested repetition operator: ++`. Controls: the same names with SYPL_LEVEL unset → exit 0;
name `svc` with SYPL_LEVEL=info → exit 0.

Root cause: user-controlled names are interpolated into regex source without escaping. The same cause
also gives wrong matches: `.` in a name matches any character, so `a.b` also targets an output named `aXb`.

Bounded fix plan (≤ 10 lines):
1. In `newMatchers`, wrap `componentName` and `outputName` in `regexp.QuoteMeta(...)` before `fmt.Sprintf`.
2. No other call site builds a regex from names (checked with `rg MustCompile` — only debug.go).
3. Keep `MustCompile`: after quoting, the pattern is always valid (the levels alternation is a constant).

Regression plan:
- Extend the existing `TestNew` table in debug/debug_test.go (currently plain names only) by calling `debug.New(name, out).Level()` with SYPL_LEVEL set (t.Setenv), for names
  `api(v2`, `C++ service`, `*x`, `a\q`, `x{99999}`; assert no panic, and `COL` matching works for
  `SYPL_LEVEL="api(v2:Buffer:debug"`.
- Wrong-match test: a component `a.b` must NOT pick up `SYPL_LEVEL="aXb:Buffer:debug"`.
- End-to-end test through `sypl.New("api(v2", ...).Infoln(...)` with SYPL_LEVEL set (subprocess or recover
  harness), asserting the message is written.
- Negative control: remove the QuoteMeta calls → the table test must panic or fail; restore → pass.
- Bypass test: a name made only of metacharacters (`()[]{}*+?|^$\.`) must not panic and must match only itself.

## Implementation and regression evidence

C1 is implemented with two `regexp.QuoteMeta` calls in `newMatchers`, before
names enter regex source. Cache keys and stored names remain literal originals.
The patch preserves the existing matcher ordering and entry boundaries.

The existing `TestNew` now checks invalid regex syntax in component/output names,
all metacharacters together, and component/output dot mismatches. A public logger
regression uses a malformed component and output name plus a scoped debug override,
and asserts the emitted line. Existing assertions and checks remain enabled.

Negative controls use a Go source overlay containing the original `debug.go`:
- `native-fix-negative-final-debug.log`: the regression fails on the original regex panic.
- `native-fix-negative-final-public.log`: the public path fails on the asynchronous panic.
- `native-fix-negative-final-boundary.log`: both dot mismatch cases fail for selecting another name.

Those logs, positive targeted/race checks and `fix-public-matrix.log` live in
`/Volumes/Dev/Dev/critical-review-20261116/sypl/`. The final commit, `make ci`
outcome and review checkpoint are recorded in that directory's `fix-report.json`.
No pushes, PRs or releases occur before the parent review checkpoint.

Release proposal: publish root `v2.0.1` before `es/v2.0.1`. The nested module now
requires root v2.0.1; its existing development replacement permits local tests
before publishing, but does not affect consumers. Root publication and clean
consumer checks must precede the es release.
