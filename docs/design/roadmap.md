# hc — Implementation Roadmap

Prioritized list of features beyond the initial release. Ordered by impact and alignment with the core churn × complexity methodology.

---

## 1. Change Coupling Analysis

New subcommand: `hc coupling [path]`

Identifies file pairs that frequently change together in the same commit. This surfaces hidden dependencies that aren't visible in the code structure — two files that always co-change may be tightly coupled even if they don't import each other. Based on Method 2 in the reference document.

**Approach:** Parse `git log --name-only` to extract the set of files changed per commit. For each commit, generate all pairwise combinations. Count co-change frequency across the history window. Output the top N pairs ranked by co-change count, with a coupling ratio (co-changes / max individual changes).

**Flags:** `--since`, `--top N`, `--format`, `--min-commits` (minimum co-changes to report).

---

## 2. Renamed/Moved File Tracking

Churn is currently split across old and new paths when a file is renamed or moved. A file renamed halfway through its history appears as two low-churn entries instead of one high-churn entry, potentially hiding a hotspot.

**Approach:** Use `git log --follow --diff-filter=R` to detect renames. Build a mapping of old path → current path. When aggregating churn, collapse rename chains so all commits attribute to the file's current path. Only the current path (the one that exists on disk) appears in the output.

---

## 3. Ignore Patterns

Allow users to exclude files and directories from analysis. Vendor code, generated files, and test fixtures add noise to the hotspot matrix without being actionable.

**Approach:** Support an `--ignore` flag accepting glob patterns (e.g. `--ignore "vendor/**" --ignore "*.pb.go"`). Also support a `.hcignore` file in the repository root, one pattern per line, following `.gitignore` syntax. Command-line patterns are additive with `.hcignore`. Patterns apply to both churn and complexity analysis.

---

## 4. Weighted Recency

Treat recent commits as more significant than old ones. A file that was heavily changed two years ago but hasn't been touched since is less interesting than one actively being modified. Flat commit counting can't distinguish these cases.

**Approach:** Apply exponential decay to commit weights based on commit date. Each commit's weight is `e^(-λ * age_in_days)` where λ is a decay constant derived from a configurable half-life (default: 6 months). The weighted sum replaces the raw commit count for threshold calculation and sorting. Raw commit count remains available in the output for reference.

**Flags:** `--decay-half-life` (e.g. `"90 days"`, `"6 months"`), `--no-decay` to disable.

---

## 5. Complexity Beyond LOC

Lines of code is a rough proxy for complexity. Two 500-line files can have very different complexity profiles — one may be a flat data mapping while the other has deep nesting and branching.

**Approach:** Add cyclomatic complexity calculation for Go files using the `go/ast` package. Count decision points (if, for, switch cases, &&, ||) per function and sum per file. For non-Go files, fall back to LOC. Expose the complexity metric used in the output. This can be extended to other languages over time by adding language-specific AST parsers.

**Flags:** `--complexity-metric loc|cyclomatic` (default: `loc`).

---

## 6. Threshold Flags

The default median split works well for most repositories but is not always the right cut. Users analyzing a large monorepo may want to focus on the 90th percentile, or set absolute thresholds based on team standards.

**Approach:** Add `--churn-threshold` and `--complexity-threshold` flags. Accept either a percentile (e.g. `p75`, `p90`) or an absolute value (e.g. `50` commits, `500` lines). When provided, these override the default median split. Both flags are optional and can be used independently — specifying only one leaves the other at the median default.
