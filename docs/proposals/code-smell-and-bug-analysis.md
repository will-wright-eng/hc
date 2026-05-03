# Code Smell and Bug Analysis

## Overview

This document records a focused review of the `hc` CLI codebase as of May 2026. The goal is to identify design decisions that could limit future growth, plus concrete bugs or edge cases worth fixing before the tool grows more features.

Overall, the codebase is in good early shape:

- Packages have clear responsibilities: git history, complexity scan, analysis, output, report rendering, ignore matching, and prompt generation.
- The domain model is small and easy to follow.
- Most pure logic has focused tests.
- The dependency footprint is low: the only non-standard-library dependency is `urfave/cli`.

The main growth risks are path semantics, hard-coded policy, and command orchestration accumulating in the CLI entrypoint.

---

## Confirmed Bugs

### Subdirectory analysis loses churn

Running analysis against a subdirectory can produce valid complexity data with zero churn:

```bash
./hc internal --json --no-min-age
```

Observed behavior:

- Files are discovered as paths relative to `internal`, such as `git/git.go`.
- Git churn is reported as paths relative to the repository root, such as `internal/git/git.go`.
- The merge step joins on exact path string equality, so churn and complexity do not match.

Relevant code:

- `cmd/hc/main.go`: `runAnalyze` passes the same path to git and complexity scanning.
- `internal/complexity/complexity.go`: `Walk` returns paths relative to the scan root.
- `internal/analysis/analysis.go`: `Analyze` merges churn and complexity by exact path.

Recommended fix:

1. Resolve the repository root once, using git.
2. Scan complexity relative to that root.
3. If the user supplied a subdirectory, apply a subtree filter while preserving root-relative paths.
4. Add command-level tests for `hc .`, `hc internal`, and `hc cmd/hc`.

This should be fixed before adding more path-sensitive features such as directory rollups, PR comments, or config discovery.

### Unknown output formats silently fall back to table

Running this command exits successfully and prints table output:

```bash
./hc --output bogus --no-min-age
```

Relevant code:

- `internal/output/output.go`: `FormatFiles` uses `default` for table formatting.

Recommended fix:

- Validate output format before running analysis.
- Return a clear error for unsupported formats.
- Keep table as the default only when the user did not explicitly request a format.

This matters because silent fallback breaks scripts that rely on machine-readable output.

---

## Growth Risks and Code Smells

### CLI entrypoint is becoming the application layer

`cmd/hc/main.go` currently handles:

- CLI flag definitions and parsing.
- Path resolution.
- Ignore file loading.
- Min-age policy.
- Git history extraction.
- Complexity scanning.
- Analysis orchestration.
- Output formatting.
- Stderr status messages.

This is fine for a small CLI, but it will become a bottleneck as features grow. Future additions such as threshold configuration, config files, richer report modes, coupling analysis, and language-specific metrics will all want to plug into the same flow.

Recommended direction:

- Keep `cmd/hc/main.go` as a thin adapter from CLI flags to application options.
- Introduce an orchestration package such as `internal/app` or `internal/run`.
- Add an `Analyze(ctx, Options) ([]analysis.FileScore, error)` style API.
- Let output/report packages consume results rather than controlling analysis behavior.

This will make the CLI easier to test and make future non-CLI usage possible without exposing public packages too early.

### Git access is hard to cancel, test, and scale

The git package shells out multiple times and buffers complete command output in memory.

Current pattern:

- `git log --name-only` for file churn.
- `git log --name-only` again for authors.
- `git log --diff-filter=R --name-status` for renames.
- `exec.Command` instead of `exec.CommandContext`.
- `cmd.Output()` reads the full output before parsing.

Recommended direction:

- Accept `context.Context` in git APIs and use `exec.CommandContext`.
- Capture stderr when git fails, so user-facing errors are actionable.
- Consider streaming parsers for large repositories.
- Consider combining git extraction into fewer passes if performance becomes an issue.

This is not urgent for small repositories, but it is one of the first places large-repo users will feel friction.

### Ignore matching advertises more than it implements

The documentation and flag help describe `.hcignore` as gitignore-style matching. The matcher supports useful basics, including basename globs, directory patterns, and simple `**`, but it is not full gitignore semantics.

Likely gaps include:

- Negation patterns with `!`.
- Leading-slash anchoring.
- Escaped comments.
- Some nuanced `**` behavior.
- Gitignore precedence and ordering rules.

Recommended direction:

- Either document this as simplified glob syntax, or
- Replace the custom matcher with a proven gitignore parser.

The current custom matcher is understandable, but the name "gitignore-style" sets user expectations high.

### Complexity policy is hard-coded

The complexity scanner currently owns several policies directly:

- Which directories are skipped.
- Which file extensions count as source-like files.
- Which comment prefixes are ignored.
- Indentation as the only complexity driver.

This is pragmatic for the current tool, but it creates friction if the project adds:

- Cyclomatic complexity.
- Language-specific analyzers.
- Configurable source file detection.
- Different treatment for docs, tests, generated files, or vendored code.

Recommended direction:

- Introduce a small `complexity.Options` struct.
- Move file inclusion and directory skip policy behind explicit options.
- Keep indentation as the default analyzer, but make the analyzer choice explicit in the code.

This does not require a plugin system yet. A small options object is enough.

### Output schema is implicit and duplicated

The JSON shape is produced by `internal/output` and decoded by `internal/report` using a separate local struct. This is acceptable for a CLI pipeline boundary, but it means schema drift can occur without compiler help.

Recommended direction:

- Treat the JSON output as a product contract.
- Add a shared DTO package or explicit schema tests.
- Consider including metadata later, such as schema version, analyzed path, git root, options, thresholds, and generated time.

This becomes more important if external automation starts consuming `hc analyze --json`.

### Report rendering should escape markdown table cells

`hc report` writes file paths directly into markdown table cells. Paths containing `|`, backticks, or unusual characters can break table formatting.

Recommended direction:

- Escape markdown table cells in report rendering.
- Add tests for paths containing pipe characters and backslashes.

This is a small fix with low risk.

### Time and policy are embedded in pure analysis paths

`analysis.Analyze` calls `time.Now()` when applying the min-age floor. `report.Render` also embeds the current date.

Recommended direction:

- For analysis, pass `now` through options or use a small clock parameter in the orchestration layer.
- For report rendering, consider passing generation metadata explicitly if reproducible output matters.

This is mostly a testability and reproducibility issue, not a functional bug.

---

## Recommended Fix Order

1. Fix root-relative path handling for subdirectory analysis.
2. Validate `--output` values and fail on unknown formats.
3. Extract analysis orchestration out of `cmd/hc/main.go`.
4. Add command-level tests around path handling and JSON output.
5. Decide whether `.hcignore` is simplified glob syntax or full gitignore semantics.
6. Add options structs for analysis, git extraction, and complexity scanning.
7. Harden report markdown escaping.
8. Revisit git performance with streaming and context cancellation when large repos become a target.

---

## Suggested Near-Term Refactor Shape

The smallest useful extraction would look like this:

```go
package app

type AnalyzeOptions struct {
    Path        string
    Since       string
    Excludes    []string
    Decay       bool
    MinAge      time.Duration
}

type AnalyzeResult struct {
    Files []analysis.FileScore
}

func Analyze(ctx context.Context, opts AnalyzeOptions) (AnalyzeResult, error) {
    // Resolve git root and requested subtree.
    // Load .hcignore.
    // Collect churn.
    // Scan complexity.
    // Merge and classify.
}
```

The CLI would then parse flags, build `AnalyzeOptions`, call `app.Analyze`, and hand the result to `output.FormatFiles`.

This keeps behavior unchanged while creating a more stable place for future features.

---

## Test Coverage Gaps

Current package-level tests are useful, but the highest-risk behavior now sits across package boundaries. Add integration-style tests that run the CLI or an app-level API against temporary git repositories.

Recommended cases:

- Analyze repository root.
- Analyze a subdirectory and preserve churn.
- Analyze after a rename and preserve merged churn.
- Validate `--json` and `--output` conflict behavior.
- Reject unsupported output formats.
- Respect `.hcignore` and repeated `--exclude` flags together.
- Render report from JSON with markdown-special characters in paths.

These tests should catch regressions in the pipeline rather than only in individual helpers.
