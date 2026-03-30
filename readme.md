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
hc analyze --since "6 months" --top 20

# Aggregate by directory
hc analyze --by-dir

# Output as JSON or CSV
hc analyze --format json
hc analyze --format csv
```
