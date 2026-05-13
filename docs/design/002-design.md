# hc — Hot/Cold Codebase Analysis Tool

## Purpose

`hc` is a command-line tool that classifies files in a git repository by churn (how often they change) and complexity (how large/complex they are). The primary output is the hotspot matrix — a ranked, quadrant-classified view that answers: *which files are both complex and frequently changing?*

This is based on Adam Tornhill's churn × complexity methodology, designed as a lightweight, tunable alternative to full-featured platforms like CodeScene.

---

## CLI Interface

```
hc [path]                             # sugar for `hc analyze [path]`
hc analyze [path]                     # hotspot matrix (default: current directory)
hc analyze --since "6 months"         # restrict churn window
hc analyze --output json|csv|table    # output format (default: table)
hc analyze --json                     # shorthand for --output json
hc analyze --exclude '<glob>'         # repeatable; .hcignore file also honored
hc analyze --no-decay                 # raw commit counts (decay on by default)
hc analyze --no-min-age               # disable the 14-day file age floor
hc analyze --files-from FILE|-        # restrict output to listed paths (PR projection)

hc md report [--input FILE] [--output FILE | --upsert FILE] [--collapsible]
hc md ignore                          # emit LLM prompt for .hcignore generation
hc md comment [--input FILE] [--output FILE] [--quadrant Q ...]
```

### Framework

urfave/cli v3 — declarative struct-based CLI with zero external dependencies. See [CLI Framework Analysis](../info/003-cli-framework-analysis.md) for rationale.

---

## Data Model

### Core Types

```go
// FileChurn represents git history analysis for a single file.
type FileChurn struct {
    Path            string
    Commits         int
    WeightedCommits float64    // recency-weighted via exponential decay
    Authors         int        // unique contributors
    FirstSeen       time.Time  // bounded by --since window
}

// FileComplexity represents static analysis for a single file.
type FileComplexity struct {
    Path       string
    Lines      int // non-blank, non-comment lines
    Complexity int // indent-sum across the same lines (drives classification)
}

// FileScore is the combined analysis result for a single file.
type FileScore struct {
    Path            string
    Commits         int
    WeightedCommits float64
    Lines           int
    Complexity      int
    Authors         int
    Quadrant        Quadrant
    FirstSeen       time.Time
}

// Quadrant classifies a file by churn × complexity.
type Quadrant int

const (
    ColdSimple  Quadrant = iota // low churn, low complexity — ignore
    ColdComplex                 // low churn, high complexity — document & protect
    HotSimple                   // high churn, low complexity — monitor
    HotCritical                 // high churn, high complexity — refactor target
)
```

Note: directory-level rollup (`DirScore`, `--by-dir`) was scoped out before shipping. File-level scoring is the only mode; consolidation strategies are tracked in [004-consolidation-strategies.md](004-consolidation-strategies.md).

### Quadrant Classification

The hotspot matrix from the reference document:

```
                    LOW CHURN              HIGH CHURN
                ┌───────────────────┬──────────────────────┐
HIGH COMPLEXITY │  Cold Complex     │  Hot Critical        │
                │  Stable liability │  Refactor target     │
                ├───────────────────┼──────────────────────┤
LOW COMPLEXITY  │  Cold Simple      │  Hot Simple          │
                │  Leave alone      │  Monitor for growth  │
                └───────────────────┴──────────────────────┘
```

**Threshold: median split.** Files above the median churn are "hot"; files above the median complexity are "complex". This adapts to any repository without requiring user configuration.

---

## Project Structure

```
cmd/hc/main.go                  # entrypoint, CLI definition
internal/
    app/app.go                  # shared analyze pipeline + options (Analyze)
    git/                        # git log → []FileChurn (decay.go, rename.go)
    complexity/                 # file walking, indent-sum → []FileComplexity
    analysis/                   # merge + median-split → []FileScore
    output/                     # table, JSON, CSV formatters
    ignore/                     # .hcignore + --exclude pattern matching
    md/                         # markdown renderers (report, ignore prompt, comment)
        templates/              # embedded markdown templates
go.mod
```

Uses `internal/` to avoid premature API surface. Packages can be promoted to public if library use cases emerge.

---

## Analysis Pipeline

```
git log --since=<window> --format=... --name-status --follow-renames
    → parse → apply decay → []FileChurn

walk file tree, count lines + indent-sum per file
    → []FileComplexity

merge on file path
    → drop files younger than min-age floor
    → compute thresholds (median split on weighted commits + indent-sum)
    → classify into quadrants
    → []FileScore

sort by quadrant priority (HotCritical first), then by weighted commits
    → format and output
```

### Churn Extraction

- Source: `git log` via `os/exec`
- Default window: all history
- Configurable via `--since` (passed directly to git)
- Counts commits per file path; tracks renames so history isn't split across paths
- Counts unique authors per file
- Applies exponential recency decay by default (half-life adapts to the analyzed window); disable with `--no-decay`

### Complexity Measurement

- **Indent-sum** is the classification metric: each non-blank, non-comment line contributes its indent depth
- LOC is still computed and displayed as a column but does not drive classification
- Cyclomatic complexity and pluggable external metrics remain open (see [proposals/001-cyclomatic-analysis.md](../proposals/001-cyclomatic-analysis.md))

### Threshold Strategy

- **Median:** above-median weighted commits = hot, above-median indent-sum = complex
- Self-adaptive — no user-tunable threshold flags

### File Age Floor

- Files whose first commit is younger than 14 days are excluded (median-split is unfair to brand-new files)
- Auto-disables when `--since` is 30 days or less
- Override with `--no-min-age`

---

## Output Formats

### Table (default)

```
QUADRANT        PATH                          COMMITS  LINES  COMPLEXITY  SCORE
Hot Critical    src/engine/parser.go              87    1240        612   42.1
Hot Critical    src/engine/evaluator.go           63     980        478   31.4
Hot Simple      src/api/handlers.go               45     120         48   18.7
Cold Complex    src/legacy/transformer.go          2    2100       1190    0.8
...
```

The `SCORE` column is added when decay is enabled (the default).

### JSON

```json
[
  {
    "path": "src/engine/parser.go",
    "commits": 87,
    "weighted_commits": 42.1,
    "lines": 1240,
    "complexity": 612,
    "authors": 6,
    "quadrant": "hot-critical",
    "first_seen": "2023-02-14T00:00:00Z"
  }
]
```

### CSV

Standard CSV with headers matching the table columns.

### Markdown

`hc analyze --json | hc md report` renders the JSON pipeline into markdown suitable for embedding in agent docs (`HOTSPOTS.md`, `AGENTS.md`). `--upsert FILE` injects between marker comments and preserves surrounding content.

---

## Status Against Original Stretch Goals

| # | Goal | Status |
|---|---|---|
| 1 | Change coupling analysis (`hc coupling`) | Not shipped |
| 2 | Renamed/moved file tracking | **Shipped** (`internal/git/rename.go`) |
| 3 | Complexity beyond LOC | **Shipped** as indent-sum default; cyclomatic still open (proposal 001) |
| 4 | Weighted recency (exponential decay) | **Shipped** (default; `--no-decay` to disable) |
| 5 | Ignore patterns (`.hcignore` / `--exclude`) | **Shipped** (`internal/ignore/`) |
| 6 | Pluggable external complexity tools (`--complexity-cmd`) | Not shipped |
| 7 | User-tunable threshold flags | Not shipped (median split is fixed) |
| 8 | Deleted file handling | **Shipped** (excluded from output) |

Additional features that landed beyond the original stretch list:

- Markdown renderers (`hc md report`, `hc md ignore`, `hc md comment`)
- PR projection (`--files-from`) for per-PR hotspot subsets
- File age floor (14-day default; `--no-min-age` to disable)
- Release pipeline via release-please + GoReleaser

Open follow-ups live in [docs/proposals/](../proposals/).

---

## Dependencies

| Dependency | Purpose |
|---|---|
| `github.com/urfave/cli/v3` | CLI framework |
| Go stdlib (`os/exec`, `bufio`, `encoding/json`, `text/template`) | Git interaction, file I/O, output, markdown templates |

No other runtime dependencies.
