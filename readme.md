# hc

Hot/Cold codebase analysis -- finds hotspots by combining git churn with file complexity.

## Install

```sh
go install github.com/will/hc/cmd/hc@latest
```

## Usage

```sh
# Analyze current repo
hc analyze

# Last 6 months, top 20 results
hc analyze -s "6 months" -n 20

# Aggregate by directory
hc analyze -d

# Output as JSON or CSV
hc analyze -f json
hc analyze -f csv

# Use indentation-based complexity instead of LOC
hc analyze -i

# Exclude files by pattern (repeatable)
hc analyze -x "*.pb.go" -x "testdata/**"

# Generate a markdown report from JSON output
hc analyze -f json | hc report -o report.md
```

### Flags

#### `analyze`

| Flag | Short | Description |
|------|-------|-------------|
| `--since` | `-s` | Restrict churn window (e.g. "6 months") |
| `--by-dir` | `-d` | Aggregate results by directory |
| `--format` | `-f` | Output format: table, json, csv (default: table) |
| `--top` | `-n` | Limit to top N results |
| `--indentation` | `-i` | Use indentation-based complexity instead of LOC |
| `--ignore` | `-x` | Glob pattern to exclude (repeatable, .gitignore syntax) |

#### `report`

| Flag | Short | Description |
|------|-------|-------------|
| `--input` | `-I` | Path to JSON file (default: stdin) |
| `--output` | `-o` | Markdown file to upsert into (default: stdout) |

### Generating a `.hcignore`

`hc prompt ignore-file-spec` emits an LLM prompt that includes your repo's structure. Pipe it into any LLM CLI to generate a `.hcignore`:

```sh
hc prompt ignore-file-spec | claude -p > .hcignore
```

| Flag | Description |
|------|-------------|
| `--max-files` | Cap file listing in repo summary (default: 200) |
| `--no-summary` | Omit the repo summary from the prompt |
