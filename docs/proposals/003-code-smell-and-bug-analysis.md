# Code Smell and Bug Analysis

## Overview

This document records the open issues from a focused review of the `hc` CLI
codebase. Items that have already been fixed (subdirectory churn merge, unknown
output format validation, the CLI/orchestration split into `internal/app`,
gitignore-style semantics for `.hcignore` via the internal matcher ported from
`sabhiram/go-gitignore`, and internal options structs for analysis, git
extraction, and complexity scanning) have been removed; what remains are the
partial and unaddressed items.

The main remaining growth risks are git extraction, CLI-level configurability
around complexity scanning, and an implicit JSON contract shared between
`output` and `report`.

---

## Growth Risks and Code Smells

### Git access is hard to cancel, test, and scale

Stderr is now captured for `git rev-parse --show-toplevel` in
`internal/git/repo_root.go`, and `git.LogWithOptions` now carries extraction
settings plus an injectable `Now` for decay weighting. The command execution
path itself is still unchanged.

Current pattern:

- `git log --name-only` for file churn.
- `git log --name-only` again for authors.
- `git log --diff-filter=R --name-status` for renames.
- `exec.Command` instead of `exec.CommandContext` in `RepoRoot`,
  `gitLogFiles`, `gitLogAuthors`, `DetectRenames`, and the test helper
  `CountAuthors`.
- `cmd.Output()` reads the full output before parsing.
- `app.Analyze` accepts a `context.Context` but does not propagate it — the
  doc comment in `internal/app/app.go` calls this out explicitly.

Recommended direction:

- Thread `context.Context` through `RepoRoot`, `git.LogWithOptions`,
  `gitLogFiles`, `gitLogAuthors`, and `DetectRenames`, and use
  `exec.CommandContext`.
- Capture stderr on the remaining git invocations so user-facing errors are
  actionable.
- Consider streaming parsers for large repositories.
- Consider combining git extraction into fewer passes if performance becomes
  an issue.

This is not urgent for small repositories, but it is one of the first places
large-repo users will feel friction.

### Complexity policy is internal-only

The complexity scanner now has `complexity.Options`, so callers can override
ignore matching, directory skipping, source-file detection, and the scanner
function. The default policy still lives in `internal/complexity`:

- Which directories are skipped.
- Which file extensions count as source-like files.
- Which comment prefixes are ignored.
- Indent-sum as the default complexity driver.

This is pragmatic for the current tool, but external configurability is still
not exposed. That creates friction if the project adds:

- Cyclomatic complexity.
- Language-specific analyzers.
- Configurable source file detection.
- Different treatment for docs, tests, generated files, or vendored code.

Recommended direction:

- Decide which complexity options, if any, should become CLI/config surface.
- Keep the default policy conservative until there is a concrete user-facing
  need for configuration.
- If cyclomatic or language-specific analyzers are added, route them through
  the existing scanner option rather than branching inside `WalkWithOptions`.

This does not require a plugin system yet. The internal options object is
enough for the next increment.

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

### Report rendering embeds the current date

The min-age policy decision was lifted into `internal/app` via
`EffectiveMinAge`, and `Now` is now injectable for git decay and analysis via
`git.LogOptions` and `analysis.Options`. The remaining hard-coded time source is
report rendering:

- `report.Render` embeds the current date.

Recommended direction:

- For report rendering, consider passing generation metadata explicitly if
  reproducible output matters.

This is mostly a reproducibility issue, not a functional bug.

---

## Recommended Fix Order

1. Harden report markdown escaping.
2. Thread `context.Context` through git extraction and capture stderr on the
   remaining invocations; revisit streaming when large repos become a target.
3. Inject generation metadata into `report.Render` if reproducible reports
   become important.
4. Promote the JSON output to a shared DTO with a schema version.

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
