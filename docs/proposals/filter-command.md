# Filter Command — Proposal

## Context

`hc` is a pipeline: `analyze` collects data and classifies files, `report` renders markdown. But `analyze` currently mixes two concerns — data collection (`--since`, `--decay`, `--ignore`) and output shaping (`--top`, `--by-dir`, `--format`). As we add consolidation logic for large repos, this gets worse. A report targeting ~20 entries from a repo with thousands of files needs a dedicated stage for deciding *what to show*.

The filter command sits between analyze and report as an explicit shaping stage:

```
hc analyze [flags] | hc filter [flags] | hc report [flags]
```

Each command has a single job:

- **analyze** — data collection + classification. Always outputs full JSON. Flags: `--since`, `--decay`, `--decay-half-life`, `--indentation`, `--ignore`.
- **filter** — shaping, consolidation, budget allocation. Flags: `--strategy`, `--budget`, `--by-dir`, `--top`.
- **report** — rendering. Flags: `--input`, `--output`, `--format`.

This separation means each stage is independently testable, composable (pipe to `jq` after any stage), and the consolidation logic has room to grow without overloading analyze.

---

## The Problem: Large Repos

A repo with thousands of files and hundreds of directories produces too many entries for a human-readable report. We need to consolidate results down to roughly 20 entities while preserving the signal that makes hotspot analysis useful — a hot file buried in a cold directory should not disappear.

This is fundamentally a filtering and grouping problem, not an analysis problem and not a rendering problem. It belongs in its own stage.

---

## Filtering Strategies

The `--strategy` flag selects how filter consolidates entries. All strategies accept `--budget N` (default 20) to control total output size.

### Strategy 1: `rollup` — Adaptive Tree Pruning

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

### Strategy 2: `top` — Top-N with Remainder

Show the top N individual files by weighted commits, group everything else into directory-level summaries.

**Algorithm:**

1. Sort all files by weighted commits descending.
2. Take the top `budget - R` files as individual entries (R = reserved slots for remainder groups).
3. Group remaining files by their top-level directory.
4. Emit each remainder group as a directory entry with aggregated metrics and quadrant = majority quadrant of children.

**Strengths:** Simple, predictable. The most actionable items always appear. Similar to what `--top` does today but with a remainder summary instead of silent truncation.

**Weaknesses:** Remainder groups may hide important cold-complex files that rank low on churn but carry significant risk.

**Best for:** Quick overviews where "show me the riskiest files" is the primary question.

### Strategy 3: `quadrant` — Quadrant-Budgeted Selection

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

### Strategy 4: `cluster` — Entropy-Based Clustering

Group files by similarity of their (weighted commits, complexity) vectors using hierarchical clustering, with the number of clusters tuned to the budget.

**Algorithm:**

1. Normalize weighted commits and complexity to [0, 1] range.
2. Hierarchical agglomerative clustering (Ward's method) on the 2D vectors.
3. Cut the dendrogram at the level that produces `budget` clusters.
4. For each cluster, emit a representative entry: centroid file (closest to cluster center) as the label, with aggregated metrics.
5. Assign quadrant to each cluster based on centroid position.

**Strengths:** Statistically principled. Naturally groups "files that behave similarly." Can reveal patterns that quadrant boundaries miss (e.g., a cluster of medium-churn, medium-complexity files that straddle the median).

**Weaknesses:** Harder to explain to humans. Output labels ("cluster containing parser.go and 11 similar files") are less intuitive than directory paths. Requires a clustering implementation (no external deps, but nontrivial code).

**Best for:** Exploratory analysis, teams comfortable with statistical grouping, very large repos where directory structure doesn't map to risk boundaries.

### Strategy 5: `outliers` — Significance Filtering

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

### Strategy 6: `hybrid` — Outliers + Pruned Tree (Recommended Default)

Combines outlier detection with adaptive tree pruning in two passes.

**Algorithm:**

1. **Pass 1 — Extract outliers:** Identify statistical outliers (same as strategy 5). These always appear as standalone file entries regardless of directory structure.
2. **Pass 2 — Prune the rest:** Remove outlier files from the tree. Apply adaptive tree pruning (strategy 1) to the remainder, using `budget - outlier_count` as the target.
3. **Merge:** Outlier files first (sorted by quadrant priority), then pruned directory entries.

**Strengths:** The files that demand individual attention are always visible. The rest of the codebase is summarized at the appropriate directory granularity. Produces the most insightful output — "here are the files to watch, and here's how risk distributes across everything else."

**Weaknesses:** Most complex to implement (combines two algorithms). Output has mixed granularity (files and directories together) which may look inconsistent.

**Best for:** Documentation-embedded reports where readers need both specific callouts and a structural overview. Recommended as the default strategy.

---

## CLI Design

```
hc filter [--strategy <name>] [--budget <n>] [--by-dir]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--strategy, -S` | `hybrid` | Consolidation strategy: `rollup`, `top`, `quadrant`, `cluster`, `outliers`, `hybrid` |
| `--budget, -b` | `20` | Target number of entries in output |
| `--by-dir` | `false` | Aggregate to directory level before filtering (moved from analyze) |
| `--top, -n` | `0` | Simple top-N truncation (shortcut for `--strategy top --budget N`; moved from analyze) |
| `--input, -I` | stdin | Path to JSON file |

### Input/Output

- **Input:** JSON array of file scores or directory scores (same schema analyze produces today).
- **Output:** JSON array, same schema, but consolidated. Entries that represent rolled-up directories gain a `"type": "dir"` field and aggregated metrics.

### Backward Compatibility

`--top` and `--by-dir` remain on `analyze` as convenience flags that apply filtering internally, but they are documented as shorthands. The canonical pipeline uses `filter`.

---

## Pipeline Examples

```bash
# Full pipeline with default consolidation
hc analyze --since "6 months" --decay | hc filter | hc report --output CLAUDE.md

# Quick top-10, table to terminal
hc analyze --since "6 months" | hc filter --strategy top --budget 10 --format table

# Quadrant-budgeted report with custom allocation
hc analyze --decay | hc filter --strategy quadrant --budget 25 | hc report

# Outlier detection only, pipe to jq
hc analyze | hc filter --strategy outliers | jq '.[] | .path'

# Backward-compatible (no filter stage, analyze applies --top internally)
hc analyze --since "6 months" --top 20
```

---

## Internal Package: `internal/filter`

```
internal/filter/
    filter.go       — Strategy interface, dispatch, shared types
    rollup.go       — Adaptive tree pruning
    top.go          — Top-N with remainder
    quadrant.go     — Quadrant-budgeted selection
    cluster.go      — Hierarchical clustering
    outliers.go     — Statistical outlier detection
    hybrid.go       — Outliers + pruned tree
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

Each strategy is a struct implementing `Filter`. The CLI dispatches based on `--strategy` to the right implementation.

---

## Migration Path

### Phase 1: Introduce filter command

- Implement `internal/filter` with `top` and `quadrant` strategies (simplest two).
- Add `hc filter` subcommand.
- `analyze` retains `--top` and `--by-dir` for backward compatibility.
- `report` works with both raw analyze output and filtered output.

### Phase 2: Add tree-based strategies

- Implement `rollup`, `outliers`, and `hybrid`.
- Set `hybrid` as the default strategy.
- Document the full pipeline in README.

### Phase 3: Clustering (optional)

- Implement `cluster` strategy.
- This is the most complex and least likely to be needed immediately. Worth revisiting after the other strategies are in use and there's feedback on what's missing.

### Phase 4: Deprecate shaping flags on analyze

- Mark `--top`, `--by-dir`, `--format` on analyze as deprecated.
- `analyze` always outputs full JSON (or optionally formatted for direct terminal use, but filtering is the canonical path).

---

## Risks

### Pipeline discoverability

Three commands is more to learn than two. Users may not realize `filter` exists.

**Mitigation:** `analyze | report` continues to work (report handles unfiltered input by truncating per quadrant as it does today). `filter` is opt-in for better results. Help text for `report` suggests piping through `filter`.

### JSON schema as contract

Three commands communicating via JSON means the schema is a public API. Breaking changes in analyze's output break filter and report.

**Mitigation:** Filter and report decode into their own local structs with lenient parsing (unknown fields ignored, missing optional fields default to zero). Add integration tests that pipe through all three stages.

### Strategy choice paralysis

Six strategies may overwhelm users.

**Mitigation:** `hybrid` is the default and the only one most users need. The others are documented for advanced use. `--strategy` flag is optional.

### Clustering complexity

Strategy 4 (cluster) requires implementing hierarchical clustering without external dependencies.

**Mitigation:** Defer to phase 3. Ward's method on 2D data is ~100 lines of code, but it's not worth writing until the simpler strategies prove insufficient.
