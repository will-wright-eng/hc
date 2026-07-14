# hc

Hot/Cold codebase analysis — finds hotspots by combining git churn with file complexity.

`hc` is built for CI. Run it on pull requests — from a GitHub Actions workflow or behind a GitHub App — and it annotates the diff with the historical state of the code being touched: a warning when a changed file is hot (high churn) and critical (complex), a notice when it is cold but complex. Reviewers see inline, on the "Files changed" tab, whether a change lands in code with a risky history. The same binary works locally for ad-hoc analysis.

## Install

```sh
go install github.com/will-wright-eng/hc/cmd/hc@latest
```

## CI usage

### PR annotations

[`.github/workflows/pr-annotations.yml`](.github/workflows/pr-annotations.yml) is a working example: on every PR it analyzes the base branch and annotates changed files that were already `hot-critical` (`::warning`) or `cold-complex` (`::notice`). The pipeline is three make targets:

```yaml
- run: make pr-changed-files   # changed.txt + anchors.txt (first changed line per file)
- run: make pr-hotspots-json   # hc analyze --json --files-from changed.txt on the base branch
- run: make pr-annotations     # hc annotate --input hotspots.json --anchor-lines anchors.txt
```

`hc annotate` consumes `hc analyze --json` output and emits [GitHub Actions workflow-command annotations](https://docs.github.com/en/actions/reference/workflows-and-actions/workflow-commands) to stdout, where the runner picks them up — **no token or `pull-requests: write` permission is needed**. Annotations render inline on the PR diff only when anchored to a changed line; `--anchor-lines` (a `path<TAB>line` TSV) supplies that anchor, otherwise they fall back to line 1 and appear on the Checks tab only.

| Flag | Short | Description |
|------|-------|-------------|
| `--input` | `-i` | Path to JSON file (default: stdin) |
| `--anchor-lines` |  | TSV file (`path<TAB>line`) anchoring each annotation to a changed line (default: line 1) |
| `--quadrant` |  | Restrict to one or more quadrants (repeatable; default: `hot-critical`, `cold-complex`) |

### PR comment report

For a whole-repo summary instead of (or alongside) inline annotations, render the analysis as markdown and post it as a sticky PR comment. See [`.github/workflows/hotspots.yml`](.github/workflows/hotspots.yml) for a working example — it builds `hc`, analyzes the repo, renders a collapsible markdown report, and upserts a comment on the PR via [`scripts/post-pr-comment.sh`](scripts/post-pr-comment.sh).

```yaml
- run: ./hc analyze --json > hotspots.json
- run: ./hc md report --collapsible --input hotspots.json --output report.md
```

## Local usage

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

### Generating a `.hcignore`

`hc md ignore` emits an LLM prompt that includes your repo's structure. Pipe it into any LLM CLI to generate a `.hcignore`:

```sh
hc md ignore | claude -p > .hcignore
```

## License

[GNU General Public License v3.0](LICENSE).
