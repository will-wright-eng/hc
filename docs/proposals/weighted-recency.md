# Weighted Recency — Implementation Plan

## Context

The hotspot matrix treats every commit equally. A file that was heavily modified two years ago but hasn't been touched since scores the same as a file actively being changed today. Both show the same commit count, but they represent very different risk profiles — the actively changing file is far more likely to introduce bugs or require attention.

Weighted recency applies exponential decay to commit scores based on commit age. Recent commits contribute more to a file's churn score than old ones. The effect is that actively maintained code surfaces as "hot" while historically busy but now-stable code drops toward "cold" — without losing any data.

---

## How It Works

Each commit's contribution to a file's churn score is weighted by its age:

```
weight = e^(-λ × age_in_days)
```

Where `λ = ln(2) / half_life_days`. With a 180-day (6 month) half-life:

- A commit from today contributes `1.0`
- A commit from 6 months ago contributes `0.5`
- A commit from 12 months ago contributes `0.25`
- A commit from 2 years ago contributes `0.06`

A file's weighted churn is the sum of all its commit weights. This replaces the raw commit count for threshold calculation, quadrant classification, and sorting. The raw commit count is preserved in the output for reference.

### Default behavior

Decay is **off by default** — the tool behaves exactly as it does today unless the user opts in. This avoids surprising existing users and keeps the zero-flag experience simple.

### CLI usage

```bash
# Enable decay with default half-life (6 months)
hc analyze --decay

# Custom half-life
hc analyze --decay --decay-half-life "90 days"
hc analyze --decay --decay-half-life "1 year"
```

### Half-life parsing

The `--decay-half-life` value is a duration string: a number followed by a unit. Supported units: `days`, `months`, `years` (and singular forms). Months are normalized to 30 days, years to 365 days. This avoids importing a duration parser for three cases.

---

## Points of Integration

### Modified: `internal/git/git.go`

#### `gitLogFiles()` — extract commit dates

The current git log format is `--format=format:` (empty, relying on `--name-only` for file lists). Change to `--format=format:__DATE__%cI` to emit an ISO 8601 commit date before each commit's file list.

```go
func gitLogFiles(repoPath string, since string) ([]commitInfo, error)
```

Replace the return type from `[][]string` (list of file lists) to a new struct:

```go
type commitInfo struct {
    Date  time.Time
    Files []string
}
```

The parser reads `__DATE__` lines to extract the timestamp, then collects subsequent file lines until the next blank line. When decay is disabled, the date is unused but still parsed — the cost is negligible (one `time.Parse` per commit).

#### `FileChurn` — add weighted score

```go
type FileChurn struct {
    Path            string
    Commits         int
    WeightedCommits float64
    Authors         int
}
```

#### `Log()` — compute weighted sums

Extend the `stats` struct to track weighted commits:

```go
type stats struct {
    commits         int
    weightedCommits float64
    authors         map[string]struct{}
}
```

During churn accumulation, for each file in each commit:

```go
s.commits++
s.weightedCommits += decayWeight(commitDate, now, halfLifeDays)
```

When decay is disabled (`halfLifeDays <= 0`), `decayWeight` returns `1.0`, making `weightedCommits` equal to `commits` as a float — no branching needed in the accumulation loop.

#### `Log()` signature change

The half-life configuration must reach `Log()`. Add it as a parameter:

```go
func Log(repoPath string, since string, ig *ignore.Matcher, halfLifeDays float64) ([]FileChurn, error)
```

A `halfLifeDays` of `0` means no decay (weight = 1.0 for all commits).

### New: `internal/git/decay.go`

A small file with the decay math:

```go
// decayWeight returns the exponential decay weight for a commit
// at commitTime, evaluated from now with the given half-life in days.
// Returns 1.0 if halfLifeDays <= 0 (decay disabled).
func decayWeight(commitTime, now time.Time, halfLifeDays float64) float64

// parseHalfLife converts a human-readable duration string (e.g. "90 days",
// "6 months", "1 year") into a number of days. Returns 0 if the string is empty.
func parseHalfLife(s string) (float64, error)
```

### Modified: `internal/analysis/analysis.go`

#### `FileScore` — add weighted churn

```go
type FileScore struct {
    Path            string
    Commits         int
    WeightedCommits float64
    Lines           int
    Complexity      int
    Authors         int
    Quadrant        Quadrant
}
```

#### `Analyze()` — use weighted churn for thresholds

When building scores from churn data, copy `WeightedCommits` from `FileChurn`:

```go
scores = append(scores, FileScore{
    Path:            cx.Path,
    Commits:         ch.Commits,
    WeightedCommits: ch.WeightedCommits,
    ...
})
```

Replace the threshold calculation:

```go
churnThreshold := medianWeightedCommits(scores)
```

Where `medianWeightedCommits` computes the median of `WeightedCommits` (float64 median).

Classification changes from:

```go
classify(scores[i].Commits, scores[i].Complexity, churnThreshold, complexityThreshold)
```

To:

```go
classifyWeighted(scores[i].WeightedCommits, scores[i].Complexity, churnThreshold, complexityThreshold)
```

Where the churn comparison uses `float64 >` instead of `int >`.

#### `sortScores()` — sort by weighted churn

The tie-breaking within a quadrant switches from `Commits` to `WeightedCommits`:

```go
return scores[i].WeightedCommits > scores[j].WeightedCommits
```

#### `DirScore` — add weighted total

```go
type DirScore struct {
    Path                 string
    Files                int
    TotalLines           int
    TotalComplexity      int
    TotalCommits         int
    TotalWeightedCommits float64
    Quadrant             Quadrant
}
```

`AnalyzeByDir` sums `WeightedCommits` alongside `Commits`, and uses the weighted sum for directory-level threshold and sorting.

### Modified: `internal/output/output.go`

#### Table format

When decay is active, add a `SCORE` column showing the weighted churn rounded to one decimal place. The raw `COMMITS` column remains unchanged:

```
QUADRANT      PATH              COMMITS  SCORE  LINES  AUTHORS
Hot Critical  src/handler.go    45       38.2   200    3
Cold Simple   lib/legacy.go     40        4.1   180    2
```

When decay is off, the `SCORE` column is omitted — output matches today's format exactly.

To know whether decay is active, the format functions need a flag. Add a `decay bool` parameter to `FormatFiles` and `FormatDirs`, or pass it through an options struct. The simplest approach: add the parameter directly, matching the existing `metric string` pattern.

#### JSON format

Add `weighted_commits` field when decay is active:

```go
type fileJSON struct {
    Path            string  `json:"path"`
    Commits         int     `json:"commits"`
    WeightedCommits float64 `json:"weighted_commits,omitempty"`
    Lines           int     `json:"lines"`
    Complexity      int     `json:"complexity"`
    Authors         int     `json:"authors"`
    Quadrant        string  `json:"quadrant"`
    Metric          string  `json:"metric"`
}
```

Using `omitempty` on `float64` means the field appears only when non-zero (i.e., when decay is active). When decay is off, `WeightedCommits` is 0 and omitted — JSON output is backward-compatible.

#### CSV format

Same approach: add `SCORE` column when decay is active.

### Modified: `cmd/hc/main.go`

Add two flags to the `analyze` command:

```go
&cli.BoolFlag{
    Name:    "decay",
    Aliases: []string{"D"},
    Usage:   "Weight commits by recency (exponential decay)",
},
&cli.StringFlag{
    Name:  "decay-half-life",
    Usage: "Half-life for decay weighting (default: \"6 months\")",
    Value: "6 months",
},
```

In `runAnalyze`:

1. Read `--decay` and `--decay-half-life` flags.
2. If `--decay` is false, set `halfLifeDays = 0` (disables weighting).
3. Otherwise, parse the half-life string via `parseHalfLife()`.
4. Pass `halfLifeDays` to `gitpkg.Log()`.
5. Pass `decay` bool to output formatters.

---

## Flag Considerations

### Why opt-in (`--decay`) instead of always-on?

Weighted recency changes which files appear in each quadrant. Enabling it by default would silently alter output for existing users and scripts that parse `hc` output. An explicit flag lets users opt in when they want recency-aware results and keeps the default behavior stable.

### Why a separate `--decay` flag instead of inferring from `--decay-half-life`?

Presence of `--decay-half-life` without `--decay` could be a mistake (user forgot the flag) or intentional pre-configuration. An explicit `--decay` flag is unambiguous. It also makes `--decay` with no half-life easy — use the default 6-month half-life.

### Interaction with `--since`

`--since` limits which commits are considered. Decay weights the commits that survive the window. These are complementary: `--since "1 year" --decay --decay-half-life "3 months"` considers the last year of history but heavily favors the last 3 months. Neither flag makes the other redundant.

### Interaction with `--by-dir`

Directory aggregation sums `WeightedCommits` from constituent files, just as it sums raw `Commits` today. The directory-level median threshold uses weighted sums. This naturally surfaces directories with recent activity.

### Interaction with `--indentation`

No interaction. Decay affects the churn axis only; complexity is measured independently.

### Interaction with `--ignore`

No interaction. Ignore filtering runs before churn scoring. Excluded files never contribute to weighted or raw counts.

### Interaction with renamed file tracking

Rename resolution runs before churn accumulation. Commits attributed to old paths are resolved to current paths, then weighted by their original commit date. A file renamed 6 months ago with heavy recent activity under its new name will correctly show high weighted churn.

---

## Risks

### Floating-point threshold sensitivity

The median of weighted scores is a float. Two files very close to the threshold could flip quadrants with tiny changes in commit timing. With integer commit counts, the boundary is discrete and stable.

**Mitigation:** This is inherent to continuous scoring and acceptable. The median split is already a rough heuristic — files near the threshold are borderline by definition. Users who need precise control can use threshold flags (future feature #6 on the roadmap).

### Half-life too short hides real hotspots

A very short half-life (e.g., "7 days") makes nearly all historical churn negligible. A file with 200 commits over 6 months but none in the last week appears cold.

**Mitigation:** The default half-life of 6 months is deliberately conservative. Documentation should note that short half-lives are aggressive and best combined with `--since` to narrow the window. Raw commit counts remain in the output for comparison.

### Performance: commit date parsing

Adding `time.Parse` per commit adds overhead. For a repository with 50,000 commits, this is ~50,000 parse calls.

**Mitigation:** `time.Parse` with a fixed layout (`time.RFC3339`) is fast — benchmarks show ~200ns per call, so 50,000 commits adds ~10ms. Negligible compared to the `git log` subprocess itself.

### Output format backward compatibility

Adding a `SCORE` column to table output changes the column layout when `--decay` is active.

**Mitigation:** The column only appears when `--decay` is explicitly passed. Default output is unchanged. JSON uses `omitempty` so the field is absent when decay is off. Scripts parsing default output are unaffected.

### `WeightedCommits` of 0.0 in JSON `omitempty`

Go's `omitempty` for `float64` omits the field when the value is exactly `0.0`. If decay is active but a file truly has a weighted score of `0.0` (all commits are extremely old), the field would be incorrectly omitted.

**Mitigation:** In practice, `e^(-λ × age)` is never exactly zero for finite age. Even a 10-year-old commit with a 6-month half-life has a weight of `~0.00001`. If this edge case matters, a dedicated `decay bool` field in the JSON struct can gate inclusion instead of relying on `omitempty`.
