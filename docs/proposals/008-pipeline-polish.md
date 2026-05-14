# Pipeline Polish: Escaping, Cancellation, and a JSON Contract

## Overview

Three small, mostly independent items extracted from
[003-code-smell-and-bug-analysis.md](003-code-smell-and-bug-analysis.md). Each
reduces a sharp edge in the existing pipeline without expanding the feature
surface:

1. Escape markdown table cells in `hc md report`.
2. Thread `context.Context` through git extraction and use
   `exec.CommandContext`.
3. Promote the `hc analyze --json` shape to a shared DTO so `output` and
   `md/report` cannot drift.

All three are pre-requisites for downstream work that consumes
`hc analyze --json` (PR comment automation, agent-facing reports, third-party
tooling).

---

## 1. Markdown table escaping in `hc md report`

### Problem

`hc md report` writes file paths directly into markdown table cells. Paths
containing `|`, backticks, or backslashes break table formatting silently —
the rendered table looks fine until a path collides with markdown syntax.

### Recommended direction

- Add a small escape helper in `internal/md` that handles `|`, backticks, and
  backslashes for table-cell contexts.
- Apply it everywhere a path or other free-form string is written into a
  markdown table.
- Add tests for paths containing pipe characters, backticks, and backslashes.

### Risk

Low. Self-contained change; templates already centralise rendering.

---

## 2. `context.Context` through git extraction

### Problem

`app.Analyze` accepts a `context.Context` but does not propagate it — the doc
comment in `internal/app/app.go` calls this out explicitly. The git
invocations themselves use `exec.Command` instead of `exec.CommandContext`,
so a slow `git log` on a large repo cannot be cancelled by the caller (CI
timeout, signal handler, future server use).

Affected sites:

- `RepoRoot` in `internal/git/repo_root.go`
- `git.LogWithOptions` / `gitLogFiles` / `gitLogAuthors` in `internal/git/git.go`
- `DetectRenames` in `internal/git/rename.go`
- The test helper `CountAuthors`

`cmd.Output()` also reads the full output before parsing, which is fine for
small repos but pairs naturally with the cancellation fix.

### Recommended direction

- Thread `context.Context` through `RepoRoot`, `git.LogWithOptions`,
  `gitLogFiles`, `gitLogAuthors`, and `DetectRenames`. Pass it down from
  `app.Analyze`.
- Switch every `exec.Command` to `exec.CommandContext`.
- Capture stderr on the remaining git invocations so cancellation/permission
  errors surface with the actual git message instead of `exit status 128`.
- Streaming parsers and combined-pass extraction are explicitly **out of
  scope** for this proposal — revisit when large-repo performance is a real
  complaint.

### Risk

Low to medium. The change is mechanical but touches every git call site.
Existing tests cover the happy path; add at least one test that cancels the
context mid-extraction and asserts the error propagates.

---

## 3. Shared JSON DTO for `hc analyze --json`

### Problem

The JSON shape is produced by `internal/output` (`fileJSON`) and decoded by
`internal/md/report` using a separate local struct. The two structs are kept
in sync manually. A field rename or type change on the producer side will
compile cleanly and only surface as a silent drop on the consumer side.

This is acceptable for an in-process CLI today, but `hc analyze --json` is
already consumed by:

- `hc md report` (markdown rendering for `HOTSPOTS.md` / `AGENTS.md`)
- `hc md comment` (PR-comment NDJSON)
- The `pr-file-comments` GitHub workflow

…and is on the path to being consumed by external automation.

### Recommended direction

- Add a small `internal/schema` (or `internal/dto`) package that defines the
  on-the-wire types for `hc analyze --json`.
- Have both `internal/output` and `internal/md` import that package — no more
  parallel structs.
- Include a top-level envelope with metadata, not just a bare array:
  - `schema_version` (string, e.g. `"1"`)
  - `generated_at` (RFC3339)
  - `repo_root`, `analyzed_path`
  - `options` snapshot (since, decay on/off, min-age, exclude patterns)
  - `thresholds` (median commit / median complexity used for the split)
  - `files`: the existing array
- Bump `schema_version` whenever the envelope changes in a non-additive way;
  additive changes (new optional fields) keep the same version.
- Add a golden-file test that locks the JSON shape so future renames are
  caught at review time.

### Migration

`hc analyze --json` currently emits a bare array. Two options:

- **(a) Breaking change:** emit the envelope unconditionally. `hc md report`
  / `hc md comment` are the only known consumers and ship in the same
  binary, so we can update them in lockstep.
- **(b) Opt-in:** keep the bare array as the default; add `--json-envelope`
  (or similar) for the new shape until external consumers exist.

Recommend **(a)** — there are no external consumers yet, and locking the
contract is the whole point of this work. Note the breaking change in the
release-please changelog.

### Risk

Medium. Touches the public-ish JSON surface. Mitigated by the lockstep
update of in-tree consumers and the golden-file test.

---

## Suggested order

1. **Markdown escaping** — smallest, no cross-package coupling.
2. **JSON DTO** — unblocks confident changes to `analyze --json` downstream.
3. **Context threading** — mechanical sweep; easiest once the DTO work has
   settled the producer-side surface.

Items 1 and 2 are independent; item 3 is independent of both but benefits
from being last so the git/extract surface is touched only once.

---

## Out of scope

These were considered and explicitly deferred:

- Streaming git-log parsers and single-pass extraction (only matters for
  large repos; revisit when there is a real complaint).
- Exposing complexity-scanner options to the CLI (tracked by
  [001-cyclomatic-analysis.md](001-cyclomatic-analysis.md)).
- Injecting generation metadata into report rendering for reproducibility
  (covered implicitly by the JSON envelope's `generated_at`; the report
  itself can keep using the current date until a real reproducibility need
  appears).

---

## Test coverage to add

- Report rendering: paths with `|`, backticks, and backslashes.
- Git extraction: a cancelled context aborts mid-run with a useful error.
- JSON DTO: a golden-file test pinning the envelope shape.
- App/CLI: round-trip `hc analyze --json | hc md report` with an
  envelope-shaped input.
