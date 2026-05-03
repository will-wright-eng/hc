# File Age Floor — Proposal

> **Status:** Phase 1 implemented on branch `file-age-floor` (2026-05-01). `FirstSeen` plumbed through `git.FileChurn` and `analysis.FileScore`; `--no-min-age` flag added; auto-disable on `--since ≤ 30d` prints a one-line stderr note; `git.ParseHalfLife` extended with `hour`/`week`. Phase 2 (unbounded `git log` for true `FirstSeen` when `--since` is narrow) and Phase 3 ("new & complex" report section) deferred — see [Follow-up](#follow-up).

## Context

Hotspot classification uses a median-split on commit count to decide hot vs. cold. The split is fair when every file has had a comparable opportunity to accumulate churn — but it isn't fair to files that were created last week. A 3-day-old file with two commits is mechanically "cold" regardless of how active it would otherwise be, and if it's also long or deeply indented it gets flagged as cold-complex. That's a false signal: the cold designation is meaningless when the file hasn't existed long enough to earn a churn count.

In codebases where this kind of analysis is valuable, code is generated at a high rate, and feature development often takes the form of stacked pull requests over the course of a sprint. A young file can legitimately be in flight without that being a quality concern.

This proposal adds a minimum file age as an inclusion criterion, fixed at **14 days** (one sprint). It is distinct from `consolidation-strategies.md`: that proposal solves a presentation-budget problem on already-classified entries, while this one solves a correctness problem upstream of classification. The two compose — a young file is excluded from analysis output, and whatever consolidation strategy runs afterward never sees it.

---

## Design

### Floor

Fixed: **14 days**, measured as `now - firstCommitTouchingFile`. Not user-tunable.

Rationale for two weeks: a typical sprint, long enough that a file in active feature development has had a fair chance to accumulate commits relative to peers, short enough that it doesn't hide genuinely concerning new code for long. The CLI is opinionated here on purpose — exposing a duration knob invites bikeshedding over a value the right answer for which is roughly the same on the windows people actually run (`--since "3 months"` through full history). One escape hatch (`--no-min-age`) covers the cases where the user genuinely wants the floor off.

### Behavior

Files whose first commit is younger than 14 days are **excluded from analysis output entirely**. They are still scanned, weighted, and counted toward the median computations for other files (excluding them from the median would shift thresholds for reasons unrelated to the age question). They simply don't appear as rows in the table / JSON / CSV.

This is the simplest correct behavior. It deliberately drops information — a 5-day-old 800-line file that is genuinely worth flagging will not appear in the report. That callout is the natural job of a future "new & complex" section, which this proposal sets up but does not implement (see [Follow-up](#follow-up)).

### Auto-disable on narrow `--since`

When `--since` is set to **30 days or less**, the floor disables automatically. A 14-day floor inside a 14-day window produces empty output; a 14-day floor inside a 30-day window leaves only ~16 days of "old enough" history to classify against, which is too thin for the median-split to be meaningful. 30 days is the smallest window where the floor still leaves room for signal, so below that we drop it rather than producing degenerate results.

A one-line stderr note announces the auto-disable (e.g. `age floor disabled: --since window ≤ 30d`). Not a warning, not a lecture — just enough that two runs differing only in `--since` don't produce mysteriously different file sets.

#### Parsing `--since` for the auto-disable check

`--since` is passed verbatim to `git log`, which accepts a much wider grammar than the existing `git.ParseHalfLife` (day/month/year). The verification cases below include `--since "2 weeks"` and `--since "1 week"`, so at minimum the parser must understand `week`. The plan:

1. Extend `git.ParseHalfLife` to accept `hour`, `week` (and their plurals) alongside the existing units. Cheap, mechanical, and useful for half-life parsing in its own right.
2. For values it still can't parse (`"yesterday"`, `"last Tuesday"`, absolute dates), **leave the floor on**. This is the conservative choice: the floor's behavior on a parseable narrow window is predictable, and the alternative (silently disabling on any string we don't recognize) is the worse failure mode.

The auto-disable lives in `cmd/hc/main.go` next to flag parsing: try to parse `--since`; if parsed and ≤ 30 days, set the effective floor to zero before handing off to analysis.

### CLI surface

One flag on `analyze` (and therefore on the bare `hc` form):

| Flag | Description |
|------|-------------|
| `--no-min-age` | Disable the 14-day age floor. |

Mirrors `--no-decay`. No duration argument, no other variants.

### Pipeline placement

The age data lives in `internal/git`: `FileChurn` gains a `FirstSeen time.Time` field, populated from the earliest commit touching each path during `git.Log`. The merge step that already collapses stats across renames (the loop in `git.go:Log` that rewrites the `raw` map's keys through `renames.Resolve` and folds entries into `m`) gains one more line: `FirstSeen = min(existing.FirstSeen, s.FirstSeen)`. `rename.go` itself is unchanged — it only resolves chains; the per-stat merge has always lived in `git.go`.

`FileScore` carries `FirstSeen` forward (not a derived `Age`, since age depends on when you ask). Filtering happens inside `analysis.Analyze` via a new `minAge time.Duration` parameter (zero disables), applied after classification and before the result is returned. Two reasons to filter post-classification rather than pre:

1. The median computation should reflect the whole repository's distribution, not just files older than the floor. Filtering pre-classification would shift thresholds in a way that depends on how many young files exist, which is noisy.
2. Carrying `FirstSeen` on `FileScore` lets downstream consumers — including the future "new & complex" callout — read it without re-deriving from raw churn data.

Doing the filter inside `Analyze` (rather than in `runAnalyze` after the call) keeps the age-floor rule upstream of every downstream consumer. Future consolidation strategies will receive the cleaned file set without extra plumbing.

The auto-disable check sits in `cmd/hc/main.go` next to flag parsing: if `--since` parses to ≤ 30 days, set the effective `minAge` to zero before handing off to analysis.

---

## Scope

**In scope.**

- Add `FirstSeen time.Time` to `git.FileChurn`, populated during `git.Log`.
- Take the earliest `FirstSeen` across the rename-merge step in `git.go:Log` (the existing per-stat merge loop, not `rename.go`).
- Add `FirstSeen time.Time` to `analysis.FileScore`.
- Add a `minAge time.Duration` parameter to `analysis.Analyze`; filter young files post-classification when it's non-zero.
- Add `--no-min-age` flag to `analyzeFlags()`.
- Extend `git.ParseHalfLife` to accept `hour` and `week` so the auto-disable check can parse `--since "2 weeks"`.
- Auto-disable the floor when `--since` parses to ≤ 30 days; emit a one-line stderr note when it triggers.
- Update `CLAUDE.md` and `readme.md`.

**Out of scope.**

- A user-tunable floor duration. The whole point of this refactor is to not ship that knob.
- Surfacing excluded files in a "new & complex" report section. Belongs to a separate proposal that consumes `FirstSeen` from `FileScore` and renders it via `internal/report`. Likely composes with the `outliers` / `hybrid` strategies in `consolidation-strategies.md` rather than living entirely in `report`.
- A directory-rollup interpretation of age. Directory summaries don't have a meaningful "first seen" — defer until mixed-granularity consolidation has a real use case.
- Configurable behavior (exclude vs. annotate vs. demote). Start with exclude; add modes only if needed.

---

## Touch Points

- `internal/git/git.go` — `FileChurn` gets `FirstSeen`; `Log` records the earliest commit date per path during the raw-stats build, and the existing rename-merge loop folds `FirstSeen` with `min(...)` when collapsing renamed paths.
- `internal/git/decay.go` — extend `ParseHalfLife` to accept `hour` and `week` (and plurals).
- `internal/analysis/analysis.go` — `FileScore` gets `FirstSeen`; `Analyze` gains a `minAge time.Duration` parameter and drops files with `now - FirstSeen < minAge` after classification (zero disables).
- `cmd/hc/main.go` (`analyzeFlags`, `runAnalyze`) — add `--no-min-age`; compute effective `minAge` (zero if `--no-min-age` is set, or if `--since` parses to ≤ 30 days; else 14 days); pass into `analysis.Analyze`. Print the one-line stderr note when auto-disable fires.
- `CLAUDE.md`, `readme.md` — document the floor, the flag, and the auto-disable rule.

`internal/git/rename.go` is intentionally unchanged: it resolves rename chains, but the per-file stats merge has always lived in `git.go`, which is where the new `FirstSeen` reduction belongs.

---

## Edge Cases

- **Empty repo / no commits**: nothing to filter.
- **`--no-min-age`**: floor disabled; behavior identical to today.
- **`--since` ≤ 30 days**: floor auto-disabled; equivalent to `--no-min-age` for that run.
- **Renamed files**: `FirstSeen` is the earliest first-seen across the merged rename chain, so a long-lived file renamed last week is not treated as new.
- **Files with no in-window commits**: not present in `commitFiles`, so their `FileChurn` entry is empty and `FirstSeen` is the zero value (`time.Time{}`). `now - zero` is ~2026 years, well above any floor — these files pass through cleanly. Only files that actually have commits in the window are candidates for exclusion.
- **`--since` between the auto-disable threshold and full history**: this is the real failure mode. Because `FirstSeen` is computed only from in-window commits, a long-lived file whose first *in-window* commit happens to be 10 days ago is treated as new and excluded — even though it's been in the repo for years. On `--since "3 months"` this isn't rare. Users who hit this can drop `--since` or pass `--no-min-age`. The proper fix is sketched below as a Phase 2 follow-up; calling this out as an honest limitation rather than ignoring it.

---

## Verification

```bash
make build && make lint && make test
./hc                                  # 14-day floor on; brand-new files absent
./hc --no-min-age                     # floor off; matches today
./hc --since "6 months"               # floor on (window > 30 days)
./hc --since "2 weeks"                # floor auto-off (window ≤ 30 days)
./hc --since "2 weeks" --json         # auto-off respected in JSON path too
```

Manual check: create a file in the repo, commit it, run `./hc` — file should be absent. Run `./hc --no-min-age` — file should appear. Run `./hc --since "1 week"` — file should appear (auto-off).

---

## Tradeoff: Hidden young hotspots

A young, complex, actively-developed file is exactly the kind of thing a reviewer might want to see — and this proposal hides it for two weeks. The mitigation is the future "new & complex" callout, which can surface excluded-but-notable files in a dedicated section without polluting the cold quadrants. Until that lands, `--no-min-age` is the escape hatch, and narrowing `--since` to the last month is the implicit one.

---

## Follow-up

**Phase 2 — true first-seen for `--since` runs.** The window-bounded `FirstSeen` is correct for unbounded runs and wrong for narrow ones. Fix: for the candidate set of files that *would* be excluded (in-window first commit < 14 days old), run a second, unbounded `git log --diff-filter=A --format=%cI -- <path> | head -1` to recover the true creation date. This is one extra `git log` per candidate, and the candidate set is small by construction (only files with very recent in-window first-touches qualify). Cheap; eliminates the edge case described in [Edge Cases](#edge-cases). Defer until Phase 1 ships and the false-exclusion rate is measurable.

**Phase 3 — "new & complex" report section.** Consumes `FirstSeen` from `FileScore` and surfaces excluded files that would otherwise be notable (long, deeply indented, multi-author). Composes with the consolidation strategies in `consolidation-strategies.md`. Out of scope here; this proposal lays the data plumbing.

---

## Suggested Commit

> `analysis: exclude files younger than 14 days from output`
>
> Hotspot classification's median-split is unfair to files that haven't
> existed long enough to accumulate churn. Adds a fixed 14-day inclusion
> floor computed from each file's first commit (rename-aware). Auto-
> disables when --since is 30 days or less. Disable explicitly with
> --no-min-age.
