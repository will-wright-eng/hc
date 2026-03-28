# hc — Hot/Cold Codebase Analysis Tool

## Purpose

`hc` is a command-line tool that classifies files and directories in a git repository by churn (how often they change) and complexity (how large/complex they are). The primary output is the hotspot matrix — a ranked, quadrant-classified view that answers: *which files are both complex and frequently changing?*

This is based on Adam Tornhill's churn × complexity methodology, designed as a lightweight, tunable alternative to full-featured platforms like CodeScene.

---

## CLI Interface

```
hc analyze [path]                     # hotspot matrix (default: current directory)
hc analyze --since "6 months"         # restrict churn window
hc analyze --by-dir                   # directory-level rollup
hc analyze --authors                  # include author concentration
hc analyze --format json|csv|table    # output format (default: table)
hc analyze --top N                    # limit to top N results

hc coupling [path]                    # change coupling analysis (stretch goal)
hc coupling --min-coupling 0.5        # minimum coupling threshold
```

### Framework

urfave/cli v3 — declarative struct-based CLI with zero external dependencies. See [CLI Framework Analysis](cli-framework-analysis.md) for rationale.

---

## Data Model

### Core Types

```go
// FileChurn represents git history analysis for a single file.
type FileChurn struct {
    Path    string
    Commits int
    Authors int    // unique contributors (optional, with --authors)
}

// FileComplexity represents static analysis for a single file.
type FileComplexity struct {
    Path  string
    Lines int
}

// FileScore is the combined analysis result for a single file.
type FileScore struct {
    Path     string
    Commits  int
    Lines    int
    Authors  int
    Quadrant Quadrant
}

// DirScore is an aggregated analysis result for a directory.
type DirScore struct {
    Path         string
    Files        int
    TotalLines   int
    TotalCommits int
    Quadrant     Quadrant
}

// Quadrant classifies a file or directory by churn × complexity.
type Quadrant int

const (
    ColdSimple  Quadrant = iota // low churn, low complexity — ignore
    ColdComplex                 // low churn, high complexity — document & protect
    HotSimple                   // high churn, low complexity — monitor
    HotCritical                 // high churn, high complexity — refactor target
)
```

### Quadrant Classification

The hotspot matrix from the reference document:

```
                    LOW CHURN              HIGH CHURN
                ┌──────────────────┬──────────────────────┐
HIGH COMPLEXITY │  Cold Complex     │  Hot Critical         │
                │  Stable liability │  Refactor target      │
                ├──────────────────┼──────────────────────┤
LOW COMPLEXITY  │  Cold Simple      │  Hot Simple           │
                │  Leave alone      │  Monitor for growth   │
                └──────────────────┴──────────────────────┘
```

**Default threshold: median split.** Files above the median churn are "hot"; files above the median complexity are "complex". This adapts to any repository without requiring user configuration.

---

## Project Structure

```
cmd/hc/main.go                  # entrypoint, CLI definition
internal/
    git/git.go                  # git log parsing → []FileChurn
    complexity/complexity.go    # file walking, LOC counting → []FileComplexity
    analysis/analysis.go        # merge churn + complexity → []FileScore, threshold logic
    output/output.go            # table, JSON, CSV formatters
go.mod
```

Uses `internal/` to avoid premature API surface. Packages can be promoted to public if library use cases emerge.

---

## Analysis Pipeline

```
git log --since=<window> --format=format: --name-only
    → parse → []FileChurn

walk file tree, count lines per file
    → []FileComplexity

merge on file path
    → compute thresholds (median split)
    → classify into quadrants
    → []FileScore

sort by quadrant priority (HotCritical first), then by commit count
    → format and output
```

### Churn Extraction

- Source: `git log` via `os/exec`
- Default window: all history
- Configurable via `--since` (passed directly to git)
- Counts commits per file path
- Optional: unique author count per file (`--authors`)

### Complexity Measurement

- Default: lines of code (LOC), counted by reading files directly
- Excludes blank lines and comment-only lines where practical
- Future: optional `--complexity-cmd` flag to plug in external tools (scc, lizard, gocyclo)

### Threshold Strategy

- **Default (median):** above-median churn = hot, above-median LOC = complex
- **Percentile flag:** `--churn-threshold p75` or `--complexity-threshold p90`
- **Absolute flag:** `--min-commits 10 --min-lines 500`

---

## Output Formats

### Table (default)

```
QUADRANT        PATH                          COMMITS  LINES  AUTHORS
Hot Critical    src/engine/parser.go              87    1240    6
Hot Critical    src/engine/evaluator.go           63     980    4
Hot Simple      src/api/handlers.go               45     120    3
Cold Complex    src/legacy/transformer.go          2    2100    1
...
```

### JSON

```json
[
  {
    "path": "src/engine/parser.go",
    "commits": 87,
    "lines": 1240,
    "authors": 6,
    "quadrant": "hot-critical"
  }
]
```

### CSV

Standard CSV with headers matching the table columns.

---

## Stretch Goals

Listed in rough priority order:

1. **Change coupling analysis** (`hc coupling`) — identify file pairs that frequently co-change, per Method 2 in the reference document
2. **Renamed/moved file tracking** — use `git log --follow` or `--diff-filter=R` to avoid splitting churn across old and new paths
3. **Complexity beyond LOC** — integrate cyclomatic complexity via external tools or built-in AST analysis for supported languages
4. **Weighted recency** — weight recent commits higher than old ones in the churn calculation (exponential decay)
5. **Ignore patterns** — `--ignore` flag or `.hcignore` file to exclude vendor, generated, or test files

---

## Dependencies

| Dependency | Purpose |
|---|---|
| `github.com/urfave/cli/v3` | CLI framework |
| Go stdlib (`os/exec`, `bufio`, `encoding/json`) | Git interaction, file I/O, output |

No other dependencies anticipated for the initial version.
