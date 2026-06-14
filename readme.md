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
hc analyze --json | hc md report -o report.md

# Or upsert into an existing markdown file (preserves surrounding content)
hc analyze --json | hc md report --upsert HOTSPOTS.md

# Emit SARIF 2.1.0 for GitHub code scanning (Security tab + PR check)
hc analyze --json | hc sarif > results.sarif
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
| `--files-from` |  | Restrict output to paths listed in FILE (one per line; `-` reads stdin). Thresholds are still computed on the full corpus — only the rows shrink. |

#### File age floor

Files whose first commit is younger than 14 days are excluded from analysis output. The median-split classifier is unfair to files that haven't existed long enough to accumulate churn — a 3-day-old file with two commits is mechanically "cold" regardless of how active it would otherwise be. The floor filters those out so they don't pollute the cold quadrants.

The floor auto-disables when `--since` is 30 days or less (a one-line stderr note announces it), since a narrow window doesn't leave enough "old enough" history for the median-split to be meaningful. Use `--no-min-age` to disable explicitly.

#### `md report`

| Flag | Short | Description |
|------|-------|-------------|
| `--input` | `-i` | Path to JSON file (default: stdin) |
| `--output` | `-o` | Write report to FILE, overwriting (default: stdout) |
| `--upsert` |  | Inject report into existing markdown file (preserves surrounding content) |
| `--collapsible` |  | Wrap hotspot categories in a `<details>` block |

#### `sarif`

Renders `hc analyze --json` as [SARIF 2.1.0](https://docs.oasis-open.org/sarif/sarif/v2.1.0/sarif-v2.1.0.html) for upload to GitHub code scanning. Findings surface in the repository **Security tab** and as a **pull-request check**. They are file-level by design (anchored at line 1), so — unlike inline review comments from `hc md comment` — they generally won't render as inline annotations on the diff; that is intentional, since the specific hunk a PR touches isn't what `hc` flags.

| Flag | Short | Description |
|------|-------|-------------|
| `--input` | `-i` | Path to JSON file (default: stdin) |
| `--output` | `-o` | Write SARIF to FILE (default: stdout) |
| `--quadrant` |  | Restrict to one or more quadrants (default: `hot-critical`, `cold-complex`; repeatable) |

Severity comes from the SARIF `level` (`hot-critical` → warning, `cold-complex`/`hot-simple` → note) — informational, not gating. `cold-simple` is healthy and never emitted. A clean run emits a valid empty SARIF, which clears previously reported alerts on upload.

### GitHub Actions

Run `hc` on every PR and post a sticky comment with the report. See [`.github/workflows/hotspots.yml`](.github/workflows/hotspots.yml) for a working example — it builds `hc`, analyzes the repo, renders a collapsible markdown report, and upserts a comment on the PR via [`scripts/post-pr-comment.sh`](scripts/post-pr-comment.sh).

```yaml
- run: ./hc analyze --json > hotspots.json
- run: ./hc md report --collapsible --input hotspots.json --output report.md
```

This repo also includes [`.github/workflows/pr-file-comments.yml`](.github/workflows/pr-file-comments.yml), which analyzes the PR base branch and posts file-level review comments for changed files that were already `hot-critical` or `cold-complex`. The workflow calls `make pr-changed-files`, `make pr-hotspots-json`, and `make pr-file-comments`; the projection filter uses `hc analyze --files-from changed.txt`, comment bodies are rendered by `hc md comment` (templates in [`internal/md/templates/comment/`](internal/md/templates/comment/)), and the posting logic lives in [`scripts/post-pr-file-comments.sh`](scripts/post-pr-file-comments.sh).

Requires `pull-requests: write` permission so the workflow can comment.

For code scanning, [`.github/workflows/hotspots-sarif.yml`](.github/workflows/hotspots-sarif.yml) runs `hc analyze --json | hc sarif` and uploads the result with [`github/codeql-action/upload-sarif`](https://github.com/github/codeql-action), so hotspots appear in the Security tab and as a PR check. It needs `security-events: write` (code scanning is free on public repos; private repos require GitHub Advanced Security).

```yaml
- run: ./hc analyze --json | ./hc sarif > results.sarif
- uses: github/codeql-action/upload-sarif@<sha>  # v4
  with:
    sarif_file: results.sarif
    category: hc
```

### Generating a `.hcignore`

`hc md ignore` emits an LLM prompt that includes your repo's structure. Pipe it into any LLM CLI to generate a `.hcignore`:

```sh
hc md ignore | claude -p > .hcignore
```
