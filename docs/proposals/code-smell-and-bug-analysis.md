# Code Smell and Bug Analysis

## Overview

This document records the open issues from a focused review of the `hc` CLI
codebase. Items that have already been fixed (subdirectory churn merge, unknown
output format validation, the CLI/orchestration split into `internal/app`, and
full gitignore semantics for `.hcignore` via `sabhiram/go-gitignore`) have been
removed; what remains are the partial and unaddressed items.

The main remaining growth risks are git extraction, hard-coded policy in the
complexity scanner, and an implicit JSON contract shared between `output` and
`report`.

---

## Growth Risks and Code Smells

### Git access is hard to cancel, test, and scale

Stderr is now captured for `git rev-parse --show-toplevel` in
`internal/git/repo_root.go`, but the rest of the git extraction path is
unchanged.

Current pattern:

- `git log --name-only` for file churn.
- `git log --name-only` again for authors.
- `git log --diff-filter=R --name-status` for renames.
- `exec.Command` instead of `exec.CommandContext`.
- `cmd.Output()` reads the full output before parsing.
- `app.Analyze` accepts a `context.Context` but does not propagate it — the
  doc comment in `internal/app/app.go` calls this out explicitly.

Recommended direction:

- Thread `context.Context` through `git.Log`, `gitLogFiles`,
  `gitLogAuthors`, and `DetectRenames`, and use `exec.CommandContext`.
- Capture stderr on the remaining git invocations so user-facing errors are
  actionable.
- Consider streaming parsers for large repositories.
- Consider combining git extraction into fewer passes if performance becomes
  an issue.

This is not urgent for small repositories, but it is one of the first places
large-repo users will feel friction.

### Complexity policy is hard-coded

The complexity scanner currently owns several policies directly:

- Which directories are skipped.
- Which file extensions count as source-like files.
- Which comment prefixes are ignored.
- Indentation as the only complexity driver.

This is pragmatic for the current tool, but it creates friction if the project
adds:

- Cyclomatic complexity.
- Language-specific analyzers.
- Configurable source file detection.
- Different treatment for docs, tests, generated files, or vendored code.

Recommended direction:

- Introduce a small `complexity.Options` struct.
- Move file inclusion and directory skip policy behind explicit options.
- Keep indentation as the default analyzer, but make the analyzer choice
  explicit in the code.

This does not require a plugin system yet. A small options object is enough.

### Output schema is implicit and duplicated

The JSON shape is produced by `internal/output` (`fileJSON`) and decoded by
`internal/report` (`fileEntry`) using a separate local struct. This is
acceptable for a CLI pipeline boundary, but it means schema drift can occur
without compiler help.

Recommended direction:

- Treat the JSON output as a product contract.
- Add a shared DTO package or explicit schema tests.
- Consider including metadata later, such as schema version, analyzed path,
  git root, options, thresholds, and generated time.

This becomes more important if external automation starts consuming
`hc analyze --json`.

### Report rendering should escape markdown table cells

`hc report` writes file paths directly into markdown table cells. Paths
containing `|`, backticks, or unusual characters can break table formatting.

Recommended direction:

- Escape markdown table cells in report rendering.
- Add tests for paths containing pipe characters and backslashes.

This is a small fix with low risk.

### Time and policy are embedded in pure analysis paths

The min-age policy decision was lifted into `internal/app` via
`EffectiveMinAge`, which is good, but the time source is still hard-coded:

- `analysis.Analyze` calls `time.Now()` when applying the min-age floor.
- `report.Render` embeds the current date.

Recommended direction:

- For analysis, pass `now` through options or use a small clock parameter in
  the orchestration layer.
- For report rendering, consider passing generation metadata explicitly if
  reproducible output matters.

This is mostly a testability and reproducibility issue, not a functional bug.

---

## Recommended Fix Order

1. Add options structs for analysis, git extraction, and complexity scanning.
2. Harden report markdown escaping.
3. Thread `context.Context` through git extraction and capture stderr on the
   remaining invocations; revisit streaming when large repos become a target.
4. Inject `now` into `analysis.Analyze` and `report.Render` so time is no
   longer baked into pure paths.
5. Promote the JSON output to a shared DTO with a schema version.

---

## Test Coverage Gaps

The orchestration layer in `internal/app` now has tests for the repo-root and
subdirectory paths, and the CLI has tests for output format validation and the
`--json` / `--output` conflict. Remaining gaps:

- Analyze after a rename and preserve merged churn at the app/CLI level
  (currently only covered in `internal/git/rename_test.go`).
- Respect `.hcignore` and repeated `--exclude` flags together.
- Render report from JSON with markdown-special characters in paths (depends
  on the escaping fix above).

These tests should catch regressions in the pipeline rather than only in
individual helpers.
