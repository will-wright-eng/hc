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

# Last 6 months, top 20 results
hc -s "6 months" -n 20

# Aggregate by directory
hc -d

# Output as JSON or CSV
hc --json
hc -o csv

# Use indentation-based complexity instead of LOC
hc -i

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
| `--by-dir` | `-d` | Aggregate results by directory |
| `--output` | `-o` | Output format: table, json, csv (default: table) |
| `--json` |  | Shortcut for `--output json` |
| `--limit` | `-n` | Limit to top N results |
| `--indentation` | `-i` | Use indentation-based complexity instead of LOC |
| `--exclude` | `-e` | Glob pattern to exclude (repeatable, .gitignore syntax) |
| `--no-decay` |  | Disable recency weighting (use raw commit counts) |

#### `report`

| Flag | Short | Description |
|------|-------|-------------|
| `--input` | `-i` | Path to JSON file (default: stdin) |
| `--output` | `-o` | Write report to FILE, overwriting (default: stdout) |
| `--upsert` |  | Inject report into existing markdown file (preserves surrounding content) |

### Generating a `.hcignore`

`hc prompt ignore` emits an LLM prompt that includes your repo's structure. Pipe it into any LLM CLI to generate a `.hcignore`:

```sh
hc prompt ignore | claude -p > .hcignore
```

| Flag | Description |
|------|-------------|
| `--max-files` | Cap file listing in repo summary (default: 200) |
| `--no-summary` | Omit the repo summary from the prompt |
