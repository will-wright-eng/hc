# Design: `hc risk` Subcommand

## Overview

`hc risk` cross-references files changed on a branch (vs a base ref) with hotspot analysis to surface which modified files are high-risk. This enables PR-level risk assessment: before merging, developers can see which changed files sit in dangerous quadrants.

## Motivation

`hc analyze` answers "what are the riskiest files in this repo?" but doesn't connect that to active development. A developer reviewing a branch wants to know: "of the files I touched, which ones are historically volatile and complex?" This closes that gap.

## Design Principle: Pipe-Based Composition

`hc risk` follows the same pattern as `hc report` — it reads analyze JSON from stdin rather than re-running the analysis pipeline internally. This keeps the command focused on a single responsibility (intersect + summarize) and lets users control analysis parameters upstream.

```
hc analyze --format json | hc risk --base main
```

This means `risk` has no analysis flags (`--since`, `--decay`, `--indentation`, `--ignore`). Those belong to `analyze`. The `risk` command only needs to know:

1. What files changed (from git diff via `--base`)
2. What the hotspot scores are (from stdin JSON)

## Usage

```bash
# Basic: pipe analyze output, compare current branch against main
hc analyze --format json | hc risk

# Explicit base ref
hc analyze --format json | hc risk --base origin/develop

# With decay + indentation (flags live on analyze, not risk)
hc analyze -D -i --format json | hc risk --base main

# JSON output for CI integration
hc analyze -D --format json | hc risk --base main --format json

# Read analysis from file instead of stdin
hc risk --base main --input analysis.json

# CI gating: exit non-zero if HotCritical files were changed
hc analyze --format json | hc risk --base main --exit-code
```

## CLI Interface

```
NAME:
   hc risk - Assess risk of branch changes against hotspot analysis

USAGE:
   hc analyze --format json | hc risk [options]

FLAGS:
   --base value, -b value       Base ref to diff against (default: "main")
   --input value                Read analysis JSON from file instead of stdin
   --format value, -f value     Output format: table, json, csv (default: "table")
   --exit-code, -e              Exit non-zero if any changed file is HotCritical (default: false)
```

Only 4 flags. Compare this to `analyze` which has 10+. The complexity stays where it belongs.

## Pipeline

```
stdin (JSON)  ──→  parse []fileEntry
                         │
git diff <base>...HEAD  ─┤
                         ▼
                    intersect by path
                         │
                         ▼
                    build RiskReport
                         │
                         ▼
                    format + output
```

## Input: Analyze JSON

`risk` consumes the same JSON format that `analyze --format json` produces and that `report` already reads:

```json
[
  {
    "path": "internal/git/git.go",
    "commits": 42,
    "weighted_commits": 38.5,
    "lines": 245,
    "complexity": 245,
    "authors": 3,
    "quadrant": "hot-critical",
    "metric": "loc"
  }
]
```

The `risk` command reuses the `fileEntry` struct pattern from `internal/report/report.go` for deserialization.

## Data Types

### New types in `internal/risk/risk.go`

```go
// FileEntry mirrors the JSON output of analyze.
type FileEntry struct {
    Path            string  `json:"path"`
    Commits         int     `json:"commits"`
    WeightedCommits float64 `json:"weighted_commits,omitempty"`
    Lines           int     `json:"lines"`
    Complexity      int     `json:"complexity"`
    Authors         int     `json:"authors"`
    Quadrant        string  `json:"quadrant"`
    Metric          string  `json:"metric"`
}

// RiskReport summarizes branch risk.
type RiskReport struct {
    BaseRef      string      `json:"base_ref"`
    ChangedFiles int         `json:"changed_files"`
    Matched      int         `json:"matched"`
    NewFiles     int         `json:"new_files"`
    Summary      RiskSummary `json:"summary"`
    Files        []FileEntry `json:"files"`
}

type RiskSummary struct {
    HotCritical int `json:"hot_critical"`
    HotSimple   int `json:"hot_simple"`
    ColdComplex int `json:"cold_complex"`
    ColdSimple  int `json:"cold_simple"`
}
```

### New git helper

```go
// internal/git/diff.go

// DiffFiles returns file paths changed between baseRef and HEAD.
func DiffFiles(repoPath string, baseRef string) ([]string, error)
```

Runs `git diff --name-only --diff-filter=ACMR <baseRef>...HEAD` (excludes deleted files since they won't appear in analysis anyway).

## Core Logic: `Assess()`

```go
// Assess filters analysis entries to only files changed on the branch.
func Assess(entries []FileEntry, changedFiles []string, baseRef string) RiskReport
```

Pure function:

1. Build a set from `changedFiles`
2. Filter `entries` to those whose `Path` is in the set
3. Count quadrant membership for matched files
4. Count `NewFiles` = len(changedFiles) - len(matched)
5. Return `RiskReport`

## Output Formats

### Table (default)

```
Risk Assessment: current branch vs main
Changed files: 12 | Analyzed: 9 | New: 3

RISK  PATH                          COMMITS  COMPLEXITY  QUADRANT
 !!   internal/git/git.go                42         245  Hot Critical
 !!   cmd/hc/main.go                     38         312  Hot Critical
  !   internal/output/output.go          29          87  Hot Simple
  .   internal/complexity/indent.go       4         190  Cold Complex
      internal/ignore/ignore.go           2          45  Cold Simple

Summary: 2 Hot Critical | 1 Hot Simple | 1 Cold Complex | 1 Cold Simple | 3 New
```

Risk indicators: `!!` = HotCritical, `!` = HotSimple, `.` = ColdComplex, ` ` = ColdSimple.

### JSON

```json
{
  "base_ref": "main",
  "changed_files": 12,
  "matched": 9,
  "new_files": 3,
  "summary": {
    "hot_critical": 2,
    "hot_simple": 1,
    "cold_complex": 1,
    "cold_simple": 1
  },
  "files": [
    {
      "path": "internal/git/git.go",
      "commits": 42,
      "weighted_commits": 38.5,
      "lines": 245,
      "complexity": 245,
      "authors": 3,
      "quadrant": "hot-critical",
      "metric": "loc"
    }
  ]
}
```

### CSV

Same columns as the table format, one row per file.

## `--exit-code` Flag

When `--exit-code` / `-e` is set, the command exits with a non-zero status if any changed file falls in the HotCritical quadrant. This enables CI gating:

```yaml
# GitHub Actions example
- name: Analyze hotspots
  run: hc analyze -D --format json > /tmp/hotspots.json

- name: Check PR risk
  run: hc risk --base ${{ github.event.pull_request.base.sha }} --input /tmp/hotspots.json --exit-code
```

Exit codes:

- `0` — no HotCritical files changed
- `1` — one or more HotCritical files changed
- `2` — error (bad ref, git failure, invalid JSON, etc.)

## Package Layout

```
internal/risk/
    risk.go          # Assess() function, RiskReport/RiskSummary/FileEntry types
    risk_test.go     # Unit tests

internal/git/
    diff.go          # DiffFiles() function
    diff_test.go     # Unit tests

internal/output/
    output.go        # Add FormatRisk() function
```

## Implementation Plan

1. **`internal/git/diff.go`** — Add `DiffFiles(repoPath, baseRef string) ([]string, error)`. Thin wrapper around `git diff --name-only`.

2. **`internal/risk/risk.go`** — Add `FileEntry` struct, `Assess(entries []FileEntry, changedFiles []string, baseRef string) RiskReport`. Pure function: filters entries, builds summary.

3. **`internal/output/output.go`** — Add `FormatRisk(w io.Writer, report risk.RiskReport, format string) error`. Handles table/json/csv rendering with the risk-specific layout.

4. **`cmd/hc/main.go`** — Add `risk` command definition and `runRisk()` action. Reads JSON from stdin or `--input`, calls `DiffFiles`, calls `Assess`, calls `FormatRisk`. Handles `--exit-code`.

5. **Tests** — Unit tests for `DiffFiles`, `Assess`, and `FormatRisk`. E2e addition to Makefile.

## Edge Cases

- **No stdin and no `--input`**: Error with usage hint: `hc analyze --format json | hc risk --base main`.
- **No changed files**: Print "No files changed between {base} and HEAD" and exit 0.
- **All changed files are new**: All files appear in "New" count, none matched to analysis. Print summary noting no historical data available.
- **Base ref doesn't exist**: Error with message suggesting valid refs.
- **Detached HEAD / same ref**: `git diff` returns empty; handle same as no changed files.
- **Renamed files on branch**: `git diff --name-only` uses current names; `analyze` already resolves renames via `DetectRenames`, so paths should match.
- **Aggregated JSON**: `risk` only supports file-level JSON. If a future mixed-granularity or aggregate JSON shape is detected, error with a clear message.

## Future Considerations

- **`--new-file-risk`**: Flag to classify new files by complexity alone (no churn data), treating them as "unknown risk" or scoring by complexity percentile against the existing distribution.
- **PR comment mode**: `--format github-comment` outputs markdown suitable for posting as a PR comment via CI.
- **Pipe to report**: `hc analyze --format json | hc risk --base main --format json | hc report` could render a risk-focused report section.
