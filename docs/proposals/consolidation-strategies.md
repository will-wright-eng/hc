# Consolidation Strategies — Proposal

## Context

On a repo with thousands of files and hundreds of directories, `hc` produces too many entries for a human-readable report. We need to consolidate down to roughly 20 entities while preserving the signal that makes hotspot analysis useful — a hot file buried in a cold directory should not disappear into a dir-level rollup.

Today the only knobs are `--by-dir` and `-L` (uniform aggregation). They aren't signal-aware: `--by-dir` flattens hot files into their directories regardless of quadrant, and `-L` collapses by tree depth without regard to where the heat actually sits. `--limit` and the per-quadrant truncation that used to live in `report` were removed in anticipation of this proposal — the gap they leave is what the strategies below fill.

This is a filtering and grouping problem worth solving with dedicated algorithms, not a new top-level command. An earlier draft of this proposal (see [history](#history)) introduced `hc filter` as a third pipeline stage between `analyze` and `report`. That framing was rejected — see [Why not a separate command](#why-not-a-separate-command). The substance — strategy interface, consolidation algorithms — survives unchanged; only the surface differs.

---

## Design

### Package: `internal/filter`

Consolidation lives in its own package, called from `runAnalyze` after classification and before formatting. No new commands, no new CLI stages.

```
internal/filter/
    filter.go       — Strategy interface, dispatch, shared types
    rollup.go       — Adaptive tree pruning
    top.go          — Top-N with remainder
    quadrant.go     — Quadrant-budgeted selection
    outliers.go     — Statistical outlier detection
    hybrid.go       — Outliers + pruned tree
    cluster.go      — Hierarchical clustering (deferred)
    filter_test.go
```

### Interface

```go
// Strategy consolidates a set of scored entries down to a budget.
type Strategy interface {
    Filter(entries []Entry, budget int) []Entry
}

// Entry is the common type consumed and produced by filter.
// It can represent a file or a directory.
type Entry struct {
    Path            string
    Type            string   // "file" or "dir"
    Commits         int
    WeightedCommits float64
    Lines           int
    Complexity      int
    Authors         int
    Files           int      // >0 when Type == "dir"
    Quadrant        string   // kebab-case
}
```

`internal/analysis` produces `FileScore` / `DirScore`; a thin adapter in `cmd/hc/main.go` (or a `filter.FromScores` helper) maps to `[]filter.Entry`. Output entries map back to `DirScore`-shaped rows for `internal/output` to render.

### CLI surface

Two new flags on `analyze` (and therefore on the bare `hc` form):

| Flag | Default | Description |
|------|---------|-------------|
| `--strategy, -S` | unset (Phase 1); `hybrid` once proven (Phase 2) | Consolidation strategy: `rollup`, `top`, `quadrant`, `outliers`, `hybrid` |
| `--budget, -b` | `20` | Target number of entries when a strategy is active |

Phase 1: no `--strategy` means no consolidation — analyze emits every classified entry. Output may be long on big repos; that's the explicit cost of "no strategy yet." Phase 2 promotes `hybrid` to the default once it's been exercised on real repos.

`--by-dir` and `-L` stay where they are. They remain useful as cheap, predictable rollups and compose with strategies (e.g. `--by-dir -L 2 --strategy top --budget 10` — aggregate to depth 2, then take top 10 with remainder).

---

## The Consolidation Problem

A repo with thousands of files produces too many entries for a useful report. We want to consolidate to ~20 entities while preserving signal:

- A hot file buried in a cold directory should remain visible.
- Cold-complex files (latent risk) should not be dropped just because they rank low on churn.
- The output should reflect the *shape* of the codebase, not just the top of a sorted list.

The strategies below trade off differently along these axes.

---

## Strategies

All strategies accept `budget int` and return at most `budget` entries.

### `rollup` — Adaptive Tree Pruning

Roll up files into parent directories, but only collapse a subtree when all children share the same quadrant. Mixed-quadrant directories keep their outlier children as individual entries.

**Algorithm:**

1. Build a directory tree from file scores.
2. Bottom-up traversal: at each node, check if all children share a quadrant.
3. If uniform, collapse into a single directory entry with aggregated metrics.
4. If mixed, keep children that differ from the majority as standalone entries; collapse the rest.
5. Repeat until entry count <= budget.

**Strengths:** Preserves the "hot file in cold directory" signal. Output resembles natural project structure.

**Weaknesses:** Entry count isn't precisely controllable — may undershoot the budget. Tree structure can produce uneven levels of detail.

**Best for:** Repos with clear directory boundaries between concerns (monorepos, well-organized projects).

### `top` — Top-N with Remainder

Show the top N individual files by weighted commits, group everything else into directory-level summaries.

**Algorithm:**

1. Sort all files by weighted commits descending.
2. Take the top `budget - R` files as individual entries (R = reserved slots for remainder groups).
3. Group remaining files by their top-level directory.
4. Emit each remainder group as a directory entry with aggregated metrics and quadrant = majority quadrant of children.

**Strengths:** Simple, predictable. The most actionable items always appear. Recovers the ergonomics of the old `--limit` flag without the silent-truncation footgun.

**Weaknesses:** Remainder groups may hide important cold-complex files that rank low on churn but carry significant risk.

**Best for:** Quick overviews where "show me the riskiest files" is the primary question.

### `quadrant` — Quadrant-Budgeted Selection

Allocate budget slots proportionally to quadrant risk, then take the top entries within each quadrant.

**Default allocation for budget=20:**

| Quadrant | Slots | Rationale |
|----------|-------|-----------|
| Hot Critical | 10 | Highest risk, most actionable |
| Hot Simple | 5 | Frequently changing, worth monitoring |
| Cold Complex | 4 | Latent risk, important for awareness |
| Cold Simple | 1 | Summary only |

**Algorithm:**

1. Partition files by quadrant.
2. Allocate slots per quadrant (fixed ratios, redistributing unused slots to higher-priority quadrants).
3. Within each quadrant, take the top N entries by weighted commits.
4. If a quadrant has fewer entries than its allocation, redistribute surplus slots upward.

**Strengths:** Guarantees the report covers the full risk spectrum. Cold-complex files get visibility even though they rank low on churn. Respects the quadrant model that is central to `hc`.

**Weaknesses:** The allocation ratios are opinionated. A repo where hot-critical is empty wastes its budget unless redistribution is implemented.

**Best for:** Reports embedded in documentation where the reader needs a complete risk picture, not just a ranked list.

### `outliers` — Significance Filtering

Only show files that are statistical outliers — churn or complexity exceeds 1.5× IQR above the median. Everything else gets a single summary line.

**Algorithm:**

1. Compute Q1, Q3, IQR for both weighted commits and complexity.
2. A file is an outlier if either metric exceeds Q3 + 1.5 × IQR.
3. Emit all outliers as individual entries, sorted by quadrant priority then weighted commits.
4. Emit one summary entry: "N files within normal range" with aggregate stats.
5. If outlier count exceeds budget, fall back to taking the top `budget - 1` outliers.

**Strengths:** Self-adaptive — the report size reflects how many files genuinely demand attention. Repos with few outliers produce short reports; repos with many produce longer ones (capped by budget). Statistically grounded "what's notable" definition.

**Weaknesses:** Could produce very few entries for repos with uniform distributions. The "1.5× IQR" threshold is a convention, not a universal truth. May miss cold-complex files that are outliers in complexity but not churn.

**Best for:** Repos where the question is "what should I worry about?" rather than "give me a complete picture."

### `hybrid` — Outliers + Pruned Tree (Recommended Default)

Combines outlier detection with adaptive tree pruning in two passes.

**Algorithm:**

1. **Pass 1 — Extract outliers:** Identify statistical outliers (same as `outliers`). These always appear as standalone file entries regardless of directory structure.
2. **Pass 2 — Prune the rest:** Remove outlier files from the tree. Apply adaptive tree pruning (`rollup`) to the remainder, using `budget - outlier_count` as the target.
3. **Merge:** Outlier files first (sorted by quadrant priority), then pruned directory entries.

**Strengths:** The files that demand individual attention are always visible. The rest of the codebase is summarized at the appropriate directory granularity. Produces the most insightful output — "here are the files to watch, and here's how risk distributes across everything else."

**Weaknesses:** Most complex to implement (combines two algorithms). Output has mixed granularity (files and directories together) which may look inconsistent.

**Best for:** Documentation-embedded reports where readers need both specific callouts and a structural overview. Recommended as the eventual default once the algorithm is proven.

### `cluster` — Entropy-Based Clustering (Deferred)

Hierarchical agglomerative clustering on (weighted commits, complexity) vectors, dendrogram cut at the budget. Statistically principled but harder to explain (cluster labels less intuitive than directory paths) and requires writing Ward's method without external deps. Defer until simpler strategies prove insufficient.

---

## Why not a separate command?

An earlier draft proposed `hc analyze | hc filter | hc report` as a three-stage pipeline. Rejected for several reasons:

1. **`analyze` isn't actually bloated.** Seven flags, ~50 lines of `runAnalyze` action body. The "mixing two concerns" framing overstates a small amount of glue — and once `--limit` was removed, the only shaping left in analyze is `--by-dir` / `-L`, which are aggregation knobs, not truncation.
2. **Reverses recently-shipped directions.** `cli-ergonomics.md` #1 makes the bare `hc` form the headline path; `dir-level.md` puts `-L` on analyze and anticipates `--by file|dir|author` there too. Pulling `--by-dir` into a separate command undoes that.
3. **JSON schema becomes a public contract.** Three commands communicating via JSON means `FileScore` / `DirScore` shape becomes harder to evolve. Today it's an internal handshake between two callers in the same binary.
4. **Pipeline composability is preserved.** `hc analyze --strategy hybrid --json | jq` works today's way; users who want filtering between stages can already pipe through `jq`.

The library extraction is the load-bearing idea. The new top-level command was extra surface for marginal benefit.

---

## Migration Path

### Phase 1 — Library + simplest strategies

- Create `internal/filter` with `Strategy` interface and shared `Entry` type.
- Implement `top` and `quadrant` (lowest algorithmic cost). `top` directly replaces the ergonomics of the recently-removed `--limit` flag, with a remainder summary instead of silent truncation.
- Add `--strategy` and `--budget` flags to `analyzeFlags()`. Default unset; analyze emits every classified entry as it does today.
- Tests: unit tests in `internal/filter`; one CLI-level test that `--strategy top --budget 5` produces 5 entries.

### Phase 2 — Tree-based strategies

- Implement `rollup`, `outliers`, `hybrid`.
- Promote `hybrid` to the default `--strategy` once it's been exercised on real repos. This is the point where unset-by-default flips to hybrid-by-default, restoring sane out-of-the-box behavior on large repos that lost it when `--limit` and the report-side truncation were removed.
- Document `hybrid` as the recommended choice for large repos in README.

### Phase 3 — Clustering (optional)

- Implement `cluster` only if Phase 2 strategies leave a real gap. Ward's method is ~100 lines but adds maintenance surface.

### Phase 4 (deferred) — `hc filter` as a top-level verb

If usage shows users genuinely want to apply different strategies to *saved* JSON without re-running analyze (e.g. exploring strategies on a stored snapshot, or chaining `jq` between stages), `hc filter` becomes worth adding. The library is already in place; it's a thin command wrapper. Until that need shows up, defer.

---

## Risks

### Strategy choice paralysis

Five strategies (six counting cluster) is a lot to surface.

**Mitigation:** Most users never set `--strategy`. When they do, `hybrid` is the recommended starting point. The others are documented for advanced use.

### Output size regression until Phase 2

Removing `--limit` and the report-side truncation before the strategies land means `hc` on a large repo currently produces a very long table or report. Users who relied on `--limit` have nothing equivalent until Phase 1 ships `top`.

**Mitigation:** Ship Phase 1 (`top` + `quadrant`) close behind the `--limit` removal. Document in the README that `--strategy top --budget 20` is the replacement for the old `--limit 20`. Keep Phase 2 (and the `hybrid` default) on a short timeline so the out-of-the-box experience on large repos returns to something usable.

### Mixed-granularity output

Strategies like `hybrid` emit files and directories in the same list. Renderers must handle `Type == "dir"` rows.

**Mitigation:** `internal/output` already handles dir rows via the `--by-dir` path; reuse that code path. Add a `Type` column or visual marker if dirs and files mix in one table.

### Schema drift

`filter.Entry` duplicates parts of `FileScore` / `DirScore`.

**Mitigation:** Acceptable cost of keeping `filter` framework-agnostic. The mapping is small (~20 lines). Alternative — making analysis types implement a `filter.Entry` interface — is cleaner but couples the packages.

---

## History

This proposal was originally drafted as a three-stage `analyze | filter | report` pipeline with `hc filter` as a new top-level command. The user pushed back on whether `analyze` was actually bloated enough to justify the split; on review the bloat case was thin and the new command reversed several directions already shipped or planned in `cli-ergonomics.md` and `dir-level.md`. The current version preserves the algorithmic substance (strategy interface, six consolidation strategies) and drops the architectural framing (third pipeline stage, JSON-as-public-contract).

After the architectural pivot, `--limit` and the per-quadrant truncation in `report` were removed in advance of this proposal landing. Both were workarounds for the missing strategy layer; deleting them clears the path for the strategies to own consolidation cleanly, at the cost of a short window where output size is uncapped until Phase 1 ships.

`hc filter` as a top-level verb stays on the table as Phase 4 if the library proves valuable and a "filter saved JSON" use case emerges.
