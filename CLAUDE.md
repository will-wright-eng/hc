# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

`hc` — a CLI tool that identifies code hotspots by combining git churn (commit frequency) with file complexity (LOC). Files are classified into a 2x2 quadrant matrix (Hot/Cold × Critical/Simple) using median-split thresholds. Based on Adam Tornhill's churn × complexity methodology.

## Commands

```bash
make build          # Build ./hc binary
make test           # Run all tests (go test ./...)
make lint           # Run go vet
make install        # Install to $GOPATH/bin
make clean          # Remove build artifacts

# Run a single test
go test -v -run TestAnalyze_QuadrantClassification ./internal/analysis/

# Run the tool
./hc analyze --since "6 months" --top 20 --format json --by-dir
```

Pre-commit hooks are configured (.pre-commit-config.yaml): go-fmt, go-vet, go-build, go-mod-tidy, golangci-lint, markdownlint.

## Architecture

Four-stage pipeline: **git history → complexity scan → classification → output**

```
cmd/hc/main.go          CLI entry point (urfave/cli v3)
internal/git/            Parses git log → []FileChurn {Path, Commits, Authors}
internal/complexity/     Walks file tree, counts LOC → []FileComplexity {Path, Lines}
internal/analysis/       Merges on path, median-split thresholds, classifies → []FileScore
internal/output/         Formats results as table/JSON/CSV
```

- **Threshold strategy**: median (p50) of commits and lines across all files — self-adaptive, no configuration needed.
- **Quadrant priority order**: HotCritical → HotSimple → ColdComplex → ColdSimple, then by commit count descending.
- **Deleted files** (in git history but not on disk) are excluded from results.
- **Directory mode** (`--by-dir`) aggregates file scores into `[]DirScore` with summed metrics.
- Only dependency beyond stdlib is `github.com/urfave/cli/v3`.
