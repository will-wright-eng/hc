# File Age Floor — Proposal

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

This is silent — no warning, no flag interaction. The user wrote `--since "2 weeks"` because they want to look at the last two weeks; honoring that is more useful than printing a lecture.

### CLI surface

One flag on `analyze` (and therefore on the bare `hc` form):

| Flag | Description |
|------|-------------|
| `--no-min-age` | Disable the 14-day age floor. |

Mirrors `--no-decay`. No duration argument, no other variants.

### Pipeline placement

The age data lives in `internal/git`: `FileChurn` gains a `FirstSeen time.Time` field, populated from the earliest commit touching each path during `git.Log`. Rename tracking already merges churn across renames; `FirstSeen` should follow the same path, taking the earliest first-seen across the merged ancestry.

`FileScore` carries `FirstSeen` forward (not a derived `Age`, since age depends on when you ask). Filtering happens in `internal/analysis` after classification, before output. Two reasons to filter post-classification rather than pre:

1. The median computation should reflect the whole repository's distribution, not just files older than the floor. Filtering pre-classification would shift thresholds in a way that depends on how many young files exist, which is noisy.
2. Carrying `FirstSeen` on `FileScore` lets downstream consumers — including the future "new & complex" callout — read it without re-deriving from raw churn data.

The auto-disable check sits in `cmd/hc/main.go` next to flag parsing: if `--since` parses to ≤ 30 days, set the effective floor to zero before handing off to analysis.

---

## Scope

**In scope.**

- Add `FirstSeen time.Time` to `git.FileChurn`, populated during `git.Log`.
- Carry `FirstSeen` through rename merging in `internal/git/rename.go`.
- Add `FirstSeen time.Time` to `analysis.FileScore`.
- Add `--no-min-age` flag to `analyzeFlags()`.
- Auto-disable the floor when `--since` ≤ 30 days.
- Filter young files out of analysis output post-classification.
- Update `CLAUDE.md` and `readme.md`.

**Out of scope.**

- A user-tunable floor duration. The whole point of this refactor is to not ship that knob.
- Surfacing excluded files in a "new & complex" report section. Belongs to a separate proposal that consumes `FirstSeen` from `FileScore` and renders it via `internal/report`. Likely composes with the `outliers` / `hybrid` strategies in `consolidation-strategies.md` rather than living entirely in `report`.
- A `--by-dir` interpretation of age. Directory rollups don't have a meaningful "first seen" — defer until a real use case shows up.
- Configurable behavior (exclude vs. annotate vs. demote). Start with exclude; add modes only if needed.

---

## Touch Points

- `internal/git/git.go` — `FileChurn` gets `FirstSeen`; `Log` records earliest commit date per path.
- `internal/git/rename.go` — merge `FirstSeen` (take the earliest) when collapsing renamed paths.
- `internal/analysis/` — `FileScore` gets `FirstSeen`; add a post-classification filter step that drops files with `now - FirstSeen < 14 days`, gated on the effective-floor flag.
- `cmd/hc/main.go` (`analyzeFlags`, `runAnalyze`) — add `--no-min-age`; compute effective floor (zero if `--no-min-age` or `--since` ≤ 30 days, else 14 days); pass into analysis.
- `CLAUDE.md`, `readme.md` — document the floor, the flag, and the auto-disable rule.

---

## Edge Cases

- **Empty repo / no commits**: nothing to filter.
- **`--no-min-age`**: floor disabled; behavior identical to today.
- **`--since` ≤ 30 days**: floor auto-disabled; equivalent to `--no-min-age` for that run.
- **Renamed files**: `FirstSeen` is the earliest first-seen across the merged rename chain, so a long-lived file renamed last week is not treated as new.
- **`--since` between 30 days and 14 days of floor**: floor active. A file's `FirstSeen` is bounded by the window, so a long-lived file whose first in-window commit happens to be 10 days ago is treated as new and excluded. This is the honest limitation: `--since` defines what "exists" for analysis, and we don't run a second unbounded `git log` to recover true first-seen. Users who hit this can drop `--since` or pass `--no-min-age`. Revisit if it shows up in practice.

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

A "new & complex" report section that consumes `FirstSeen` from `FileScore` and surfaces excluded files that would otherwise be notable (long, deeply indented, multi-author). Composes with the consolidation strategies in `consolidation-strategies.md`. Out of scope here; this proposal lays the data plumbing.

---

## Suggested Commit

> `analysis: exclude files younger than 14 days from output`
>
> Hotspot classification's median-split is unfair to files that haven't
> existed long enough to accumulate churn. Adds a fixed 14-day inclusion
> floor computed from each file's first commit (rename-aware). Auto-
> disables when --since is 30 days or less. Disable explicitly with
> --no-min-age.
