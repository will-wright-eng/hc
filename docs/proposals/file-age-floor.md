# File Age Floor — Proposal

## Context

Hotspot classification uses a median-split on commit count to decide hot vs. cold. The split is fair when every file has had a comparable opportunity to accumulate churn — but it isn't fair to files that were created last week. A 3-day-old file with two commits is mechanically "cold" regardless of how active it would otherwise be, and if it's also long or deeply indented it gets flagged as cold-complex. That's a false signal: the cold designation is meaningless when the file hasn't existed long enough to earn a churn count.

In codebases where this kind of analysis is valuable, code is generated at a high rate, and feature development often takes the form of stacked pull requests over the course of a sprint. A young file can legitimately be in flight without that being a quality concern.

This proposal adds a minimum file age as an inclusion criterion, with a default floor of **14 days** (one sprint). It is distinct from `consolidation-strategies.md`: that proposal solves a presentation-budget problem on already-classified entries, while this one solves a correctness problem upstream of classification. The two compose — a young file is excluded from analysis output, and whatever consolidation strategy runs afterward never sees it.

---

## Design

### Floor

Default: **14 days**, measured as `now - firstCommitTouchingFile`.

Rationale for two weeks: a typical sprint, long enough that a file in active feature development has had a fair chance to accumulate commits relative to peers, short enough that it doesn't hide genuinely concerning new code for long. An adaptive floor (e.g. "25% of the analysis window") was considered and rejected — it over-engineers a knob the user is unlikely to tune, and the answer is roughly the same on the windows people actually run (`--since "3 months"` through full history).

### Behavior

Files whose first commit is younger than the floor are **excluded from analysis output entirely**. They are still scanned, weighted, and counted toward the median computations for other files (excluding them from the median would shift thresholds for reasons unrelated to the age question). They simply don't appear as rows in the table / JSON / CSV.

This is the simplest correct behavior. It deliberately drops information — a 5-day-old 800-line file that is genuinely worth flagging will not appear in the report. That callout is the natural job of a future "new & complex" section, which this proposal sets up but does not implement (see [Follow-up](#follow-up)).

### CLI surface

One flag on `analyze` (and therefore on the bare `hc` form):

| Flag | Default | Description |
|------|---------|-------------|
| `--min-age` | `"14 days"` | Minimum file age (by first commit) for inclusion. `0` disables the floor. |

Accepts the same human-readable durations as `--since` (`"2 weeks"`, `"30 days"`, `"1 month"`). Reusing the parser keeps the surface consistent.

`--min-age 0` disables the floor entirely, restoring today's behavior. No separate `--no-min-age` toggle — the zero value is unambiguous and matches how other duration flags behave.

### Pipeline placement

The age data lives in `internal/git`: `FileChurn` gains a `FirstSeen time.Time` field, populated from the earliest commit touching each path during `git.Log`. Rename tracking already merges churn across renames; `FirstSeen` should follow the same path, taking the earliest first-seen across the merged ancestry.

Filtering on age happens in `internal/analysis`, after classification, before output. Two reasons to filter post-classification rather than pre:

1. The median computation should reflect the whole repository's distribution, not just files older than the floor. Filtering pre-classification would shift thresholds in a way that depends on how many young files exist, which is noisy.
2. `FileScore` is the natural place to carry an `Age` field forward (computed from `FirstSeen`), so downstream consumers — including the future "new & complex" callout — can read it without re-deriving from raw churn data.

---

## Scope

**In scope.**

- Add `FirstSeen time.Time` to `git.FileChurn`, populated during `git.Log`.
- Carry `FirstSeen` through rename merging in `internal/git/rename.go`.
- Add `Age time.Duration` (or `FirstSeen`) to `analysis.FileScore`.
- Add `--min-age` flag (default `"14 days"`) to `analyzeFlags()`.
- Filter young files out of analysis output post-classification.
- Update `CLAUDE.md` and `readme.md`.

**Out of scope.**

- Surfacing excluded files in a "new & complex" report section. Belongs to a separate proposal that consumes `Age` from `FileScore` and renders it via `internal/report`. Likely composes with the `outliers` / `hybrid` strategies in `consolidation-strategies.md` rather than living entirely in `report`.
- A `--by-dir` interpretation of age. Directory rollups don't have a meaningful "first seen" — defer until a real use case shows up.
- Configurable behavior (exclude vs. annotate vs. demote). Start with exclude; add modes only if needed.

---

## Touch Points

- `internal/git/git.go` — `FileChurn` gets `FirstSeen`; `Log` records earliest commit date per path.
- `internal/git/rename.go` — merge `FirstSeen` (take the earliest) when collapsing renamed paths.
- `internal/analysis/` — `FileScore` gets `FirstSeen` (or `Age`); add a post-classification filter step keyed on the `--min-age` value.
- `cmd/hc/main.go` (`analyzeFlags`, `runAnalyze`) — flag definition; pass parsed duration into analysis.
- `CLAUDE.md`, `readme.md` — document the floor and the flag.

---

## Edge Cases

- **Empty repo / no commits**: nothing to filter.
- **`--min-age 0`**: floor disabled; behavior identical to today.
- **`--min-age` larger than oldest commit**: every file filtered out. Acceptable — the user asked for it. Output will be empty rather than erroring.
- **Renamed files**: `FirstSeen` is the earliest first-seen across the merged rename chain, so a long-lived file renamed last week is not treated as new.
- **`--since` interaction**: `--since "1 week"` truncates the log to the last week, which makes every visible file look ~1 week old. The age floor is computed from `FirstSeen` *within the analyzed window*, which is the only data we have. Document this: narrowing `--since` below `--min-age` is self-defeating and will produce empty results. Don't try to be clever — the user wrote both flags.

The `--since` interaction is the one rough edge. The honest framing: `--since` defines what "exists" for the purposes of analysis, so a file's first commit before `--since` is unobservable. Two ways to handle this:

1. Accept the limitation — document it, let the user pick consistent values.
2. Run a second `git log` query without `--since` to recover true first-seen.

Option 1 is simpler and avoids a second git invocation per analysis. Recommend starting there; revisit if users hit it.

---

## Verification

```bash
make build && make lint && make test
./hc                                  # default 14-day floor; brand-new files absent
./hc --min-age 0                      # floor off; matches today
./hc --min-age "30 days"              # stricter floor
./hc --min-age "30 days" --json       # JSON path also respects floor
./hc --since "1 week" --min-age "14 days"   # documented self-defeat: empty output OK
```

Manual check: create a file in the repo, commit it, run `./hc` — file should be absent. Wait two weeks (or use a test fixture with a backdated commit), re-run — file should appear.

---

## Tradeoff: Hidden young hotspots

A young, complex, actively-developed file is exactly the kind of thing a reviewer might want to see — and this proposal hides it for two weeks. The mitigation is the future "new & complex" callout, which can surface excluded-but-notable files in a dedicated section without polluting the cold quadrants. Until that lands, `--min-age 0` is the escape hatch.

---

## Suggested Commit

> `analysis: exclude files younger than --min-age (default 14 days)`
>
> Hotspot classification's median-split is unfair to files that haven't
> existed long enough to accumulate churn. Adds a 14-day inclusion floor
> by default, computed from each file's first commit (rename-aware).
> Disable with --min-age 0.
