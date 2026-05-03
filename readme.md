# hc

Hot/Cold codebase analysis -- finds hotspots by combining git churn with file complexity.

## Install

```sh
go install github.com/will-wright-eng/hc/cmd/hc@latest
```

## Usage

`hc [path]` is sugar for `hc analyze [path]` — bare `hc` analyzes the current repo. Use the explicit `hc analyze ...` form when piping or scripting.

```sh
# Analyze current repo
hc

# Analyze a specific path
hc internal/

# Last 6 months
hc -s "6 months"

# Output as JSON or CSV
hc --json
hc -o csv

# Exclude files by pattern (repeatable)
hc -e "*.pb.go" -e "testdata/**"

# Generate a markdown report from JSON output
hc analyze --json | hc report -o report.md

# Or upsert into an existing markdown file (preserves surrounding content)
hc analyze --json | hc report --upsert HOTSPOTS.md
```

### Flags

#### `analyze`

| Flag | Short | Description |
|------|-------|-------------|
| `--since` | `-s` | Restrict churn window (e.g. "6 months") |
| `--output` | `-o` | Output format: table, json, csv (default: table) |
| `--json` |  | Shortcut for `--output json`. Cannot combine with `--output`. |
| `--exclude` | `-e` | Glob pattern to exclude (repeatable, .gitignore syntax) |
| `--no-decay` |  | Disable recency weighting (use raw commit counts) |
| `--no-min-age` |  | Disable the 14-day file age floor |

#### File age floor

Files whose first commit is younger than 14 days are excluded from analysis output. The median-split classifier is unfair to files that haven't existed long enough to accumulate churn — a 3-day-old file with two commits is mechanically "cold" regardless of how active it would otherwise be. The floor filters those out so they don't pollute the cold quadrants.

The floor auto-disables when `--since` is 30 days or less (a one-line stderr note announces it), since a narrow window doesn't leave enough "old enough" history for the median-split to be meaningful. Use `--no-min-age` to disable explicitly.

#### `report`

| Flag | Short | Description |
|------|-------|-------------|
| `--input` | `-i` | Path to JSON file (default: stdin) |
| `--output` | `-o` | Write report to FILE, overwriting (default: stdout) |
| `--upsert` |  | Inject report into existing markdown file (preserves surrounding content) |

### GitHub Actions

Run `hc` on every PR and post a sticky comment with the report. See [`.github/workflows/hotspots.yml`](.github/workflows/hotspots.yml) for a working example — it builds `hc`, analyzes the repo, renders a collapsible markdown report, and upserts a comment on the PR via [`scripts/post-pr-comment.sh`](scripts/post-pr-comment.sh).

```yaml
- run: ./hc analyze --json > hotspots.json
- run: ./hc report --collapsible --input hotspots.json --output report.md
```

This repo also includes [`.github/workflows/pr-file-comments.yml`](.github/workflows/pr-file-comments.yml), which analyzes the PR base branch and posts file-level review comments for changed files that were already `hot-critical` or `cold-complex`. The workflow calls `make pr-changed-files`, `make pr-hotspot-matches`, and `make pr-file-comments`; the comparison logic lives in [`scripts/filter-pr-hotspots.py`](scripts/filter-pr-hotspots.py), the comment text lives in [`scripts/templates/`](scripts/templates/), and the posting logic lives in [`scripts/post-pr-file-comments.sh`](scripts/post-pr-file-comments.sh).

Requires `pull-requests: write` permission so the workflow can comment.

### Generating a `.hcignore`

`hc prompt ignore` emits an LLM prompt that includes your repo's structure. Pipe it into any LLM CLI to generate a `.hcignore`:

```sh
hc prompt ignore | claude -p > .hcignore
```

| Flag | Description |
|------|-------------|
| `--max-files` | Cap file listing in repo summary (default: 200) |
| `--no-summary` | Omit the repo summary from the prompt |
