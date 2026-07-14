# Change Coupling Analysis (`hc analyze --coupling`)

## Overview

Add change coupling — file pairs that are repeatedly modified in the same
commit — as an opt-in section of the `hc analyze --json` envelope, and teach
`hc annotate` to emit a **missing co-change partner** annotation on PRs:

```text
::notice file=internal/git/git.go,line=12,title=hc: Frequent co-change partner not in this PR::internal/git/git.go changes together with internal/git/git_test.go in 80% of its commits (12 co-changes), but this PR does not touch internal/git/git_test.go. Check whether it needs a matching change.
```

This closes stretch goal #1 from [002-design.md](../design/002-design.md)
("Change coupling analysis — Not shipped") in a CI-first shape: no standalone
`hc coupling` command, no table rendering, no report section. The envelope is
the only producer surface and `hc annotate` is the only consumer surface in
Phase 1. Methodology background lives in
[001-hot-cold-codebase-analysis.md](../design/001-hot-cold-codebase-analysis.md)
(Method 2).

The extraction is cheap: `internal/git` already parses
`git log --name-only` into per-commit file sets (`commitInfo{Date, Files}`)
before aggregating them into per-file `FileChurn`. Coupling is a second
aggregation over the same pass — no new git invocation.

## Design decisions (resolved)

| Axis | Decision |
| --- | --- |
| CLI surface | `--coupling` flag on `analyze` only; no `hc coupling` command |
| Metric | Support (co-change count) + asymmetric confidence in both directions |
| Recency | Decay-weighted, same half-life as churn; `--no-decay` applies uniformly |
| Co-change unit | Same commit; commits touching > 50 files skipped (`--max-commit-files`) |
| Noise floors | Fixed defaults, no override flags |
| Scope | `--files-from` projection; `.hcignore`/`--exclude` apply |
| CI surface | `hc annotate` partner annotations only |

## Background — current state

- `LogWithOptions` (`internal/git/git.go`) builds `[]commitInfo` from one
  `git log --format=__DATE__%cI --name-only` pass, then flattens it into
  per-file stats. The per-commit file sets — exactly what coupling needs — are
  discarded after aggregation.
- Rename resolution (`RenameMap.Resolve`) and decay weighting (`DecayWeight`,
  window-adaptive half-life) already exist and apply per commit.
- The envelope (`internal/schema`) is versioned; new **optional** fields are
  additive and keep `schema_version: "1"`.
- The PR pipeline is `make pr-changed-files` → `make pr-hotspots-json`
  (`hc analyze --json --files-from changed.txt ../hc-base`) →
  `make pr-annotations` (`hc annotate --input hotspots.json --anchor-lines
  anchors.txt`). `anchors.txt` already lists every changed file.

## Design

### Metric

For a pair (A, B) co-changing means both paths appear in the same commit,
after rename resolution. Per pair:

- **support** — raw co-change commit count.
- **weighted_support** — sum of `DecayWeight(commitDate)` over co-change
  commits, using the same half-life as churn. With `--no-decay` the weight is
  1, so weighted values equal raw values — no special casing.
- **confidence_a_b** = weighted co-changes ÷ weighted changes of A (and
  symmetrically **confidence_b_a**). Asymmetric on purpose: "when `a.go`
  changes, `b.go` changes 80% of the time" and its reverse are different
  facts, and the annotation wording uses the direction anchored on the
  changed file.

Pairs are ranked by weighted support descending, tie-broken by max confidence,
then lexicographically by path.

### Noise control

- **Commit size cap:** commits touching more than `--max-commit-files` files
  (default **50**) are skipped for pair extraction — a mass rename or
  format-everything commit would otherwise couple every file to every other
  file. The cap applies **only** to coupling; churn counting is unchanged.
  This is the one new tunable; it earns a flag because repo commit styles
  genuinely differ (squash-merge monorepos vs. atomic commits).
- **Fixed floors, no flags:** a pair enters the envelope only when raw
  `support >= 5` **and** `max(confidence_a_b, confidence_b_a) >= 0.5`. The
  floor uses *raw* support (stable and explainable) while ranking uses
  weighted support (recency-aware). Median-split does not transfer here —
  pair frequencies are power-law distributed — so this is a deliberate,
  documented departure from hc's zero-config identity. The defaults sit above
  code-maat's (`min-revs 5`, coupling ≥ 30%) because with no override flags
  the design must err toward precision over recall. If the floors prove wrong
  in practice, adding flags is an additive follow-up.

### File universe

Pairs cover the same file universe as `files`: ignored paths (`.hcignore`,
`--exclude`), deleted files, and age-floored files never appear on either side
of a pair. Self-pairs created by rename resolution are dropped.

Under `--files-from`, a pair is kept when **at least one side** is in the
selection — the other side is usually outside it, which is the point. As with
file rows, coupling is computed on the full corpus; the projection only
shrinks what is emitted.

### Envelope shape (additive, stays v1)

```json
{
  "options": { "coupling": true, "max_commit_files": 50 },
  "coupling": {
    "min_support": 5,
    "min_confidence": 0.5,
    "pairs": [
      {
        "a": "internal/git/git.go",
        "b": "internal/git/git_test.go",
        "support": 12,
        "weighted_support": 7.3,
        "confidence_a_b": 0.8,
        "confidence_b_a": 0.6
      }
    ]
  }
}
```

The floors are echoed in the envelope so consumers can render honest wording
without hardcoding them.

### Flag contract

`--coupling` requires JSON output (`--json` / `--output json`); combining it
with table or csv is an error, mirroring the existing `--json` vs.
`--output <non-json>` exclusivity. Pair-centric data has no Phase 1 rendering
in the file-centric table, and a silently ignored flag would be worse than an
error.

### Annotate: missing co-change partner (the flagship)

`hc annotate` gains a second annotation kind, emitted when the envelope
contains a `coupling` section:

- **Changed set:** the keys of `--anchor-lines` when provided (it already
  lists every changed file from `git diff`), else the envelope's `files`
  paths. Under projection both identify "files this PR touches".
- For each pair with exactly **one** side in the changed set, emit a
  `::notice` anchored on the changed side (its anchor line, else `line=1`),
  naming the partner and the changed-side→partner confidence. Both sides
  changed → silence (the co-change happened). Neither side changed → not in
  the envelope anyway.
- **Level `notice`, always:** coupling can be stale and splits can be
  intentional; a nudge, not an alarm. Hotspot annotations keep their existing
  levels; the two kinds are independent, and a file can receive both.
- Envelopes without a `coupling` section behave exactly as today — the
  feature is inert unless analyze ran with `--coupling`.

## Refactor specifics

### `internal/git`

- `LogOptions` gains `Coupling bool` and `MaxCommitFiles int` (0 → default 50).
- Pair aggregation runs inside the existing `LogWithOptions` pass over
  `commitFiles`: resolve each file through `RenameMap`, drop ignored paths and
  self-pairs, then count each unordered pair once per qualifying commit
  (`map[[2]string]*pairStats`). Bounded at ≤ C(50,2) = 1225 pairs per commit;
  floor pruning happens after aggregation.
- Return shape: a new `LogResult{Churn []FileChurn, Pairs []CouplingPair}`
  from a `LogWithCoupling` variant (or an extra return — decide at
  implementation), keeping `LogWithOptions`'s signature for existing callers.

### `internal/analysis` / `internal/output`

- Analysis filters pairs to the surviving file universe (deletion + age
  floor), applies the floors, computes confidences from each file's weighted
  churn, and sorts.
- The JSON writer emits the `coupling` section; table/csv writers never see
  it (flag contract above).

### `internal/schema`

- Additive types: `Coupling`, `CouplingPair`; `Options` gains
  `coupling` / `max_commit_files` (omitempty). `SchemaVersion` stays `"1"`.

### `internal/annotate`

- Parse the optional `coupling` section; build the changed set from
  `Options.AnchorLines` keys or envelope files; render partner notices after
  hotspot annotations (hotspot warnings first — they matter more under the
  per-step display caps).
- New wording rule + escaping via the existing emitter.

### `cmd/hc/main.go` / Makefile / workflow

- `analyzeFlags()` gains `--coupling` and `--max-commit-files` (shared by the
  root command sugar).
- `pr-hotspots-json` becomes
  `hc analyze --json --coupling --files-from changed.txt ../hc-base`.
- `pr-annotations.yml` is otherwise unchanged — no new steps, tokens, or
  permissions.

## Non-goals (Phase 1)

- A standalone `hc coupling` command, or table/csv rendering of pairs.
- An `hc md report` coupling section — natural Phase 2 once the envelope
  carries the data.
- Logical change-set grouping (author + time window, PR-level) — same-commit
  only; code-maat treats grouping as an advanced mode and it can be layered
  on later without changing the envelope shape.
- Threshold override flags (`--min-support`, `--min-confidence`) — fixed
  floors by decision; additive if needed.
- Hotspot-involving pair filters and cross-directory/distance signals —
  future refinements to ranking, not core extraction.

## Risks

- **Notice-cap crowding:** partner notices share the 10-per-level-per-step
  display cap with `cold-complex` hotspot notices. A large PR could push
  partner notices out of the display (they remain in logs). If it bites,
  split kinds across steps or promote strong-confidence partners to a
  separate step — deferred until observed.
- **Test-file pairs dominate:** `x.go ↔ x_test.go` couples strongly and will
  top many rankings. For the partner annotation this is *signal* ("you
  changed the code, not its test"); for future report surfaces it may need a
  same-stem or same-dir de-emphasis. `.hcignore` is the existing escape
  hatch.
- **Fixed floors misfit some repos:** squash-merge histories inflate
  co-change counts; sparse histories may never reach support 5 inside a
  narrow `--since` window. No escape hatch by design — revisit with data.
- **Window sensitivity:** like churn and `FirstSeen`, coupling is bounded by
  `--since`; confidence denominators shrink with the window. Same caveat as
  the file age floor docs.
- **Memory on monorepos:** the pair map is bounded by the commit-size cap and
  post-aggregation pruning; a pathological history (many distinct ≤50-file
  commits) grows the map linearly with distinct pairs. Acceptable; measure
  before optimizing.

## Test coverage to add

- Pair extraction: same-commit counting, rename resolution merging pair keys,
  self-pair drop, ignored-path drop, commit-size cap boundary (50 vs 51
  files).
- Metric math: weighted support under decay, confidence asymmetry, `--no-decay`
  ⇒ weighted == raw; floor filtering on raw support and max confidence.
- Universe consistency: age-floored and deleted files absent from pairs;
  `--files-from` keeps one-side pairs, drops zero-side pairs.
- Flag contract: `--coupling` with table/csv errors; `--coupling --json`
  emits the section; without the flag the envelope has no `coupling` key.
- Annotate golden: one-side-changed ⇒ notice with correct direction of
  confidence and anchor line; both-sides-changed ⇒ silence; envelope without
  coupling ⇒ byte-identical output to today; escaping of `%`, `:`, `,`,
  newlines in partner paths.

## References

- Tornhill, A. (2018). *Software Design X-Rays* — change coupling
  methodology.
- code-maat: <https://github.com/adamtornhill/code-maat> — prior art for
  support/confidence thresholds (`-a coupling`, `min-revs`).
- [001-hot-cold-codebase-analysis.md](../design/001-hot-cold-codebase-analysis.md)
  — Method 2: Change Coupling Analysis; co-location principle.
- [002-design.md](../design/002-design.md) — stretch goal #1 (unshipped).
- [010-pr-hotspot-annotations.md](010-pr-hotspot-annotations.md) — the
  annotation mechanism, caps, and escaping rules this builds on.
