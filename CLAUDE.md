# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

`hc` — a CLI tool that identifies code hotspots by combining git churn (commit frequency) with file complexity (LOC or indentation depth). Files are classified into a 2x2 quadrant matrix (Hot/Cold × Critical/Simple) using median-split thresholds. Supports decay weighting to prioritize recent activity. Based on Adam Tornhill's churn × complexity methodology.

## Commands

```bash
make build          # Build ./hc binary
make test           # Run all tests (go test ./...)
make lint           # Run go vet
make install        # Install to $GOPATH/bin
make clean          # Remove build artifacts
make e2e            # Run e2e: analyze with decay+indentation piped to report

# Run a single test
go test -v -run TestAnalyze_QuadrantClassification ./internal/analysis/

# Run the tool — `hc [path]` is sugar for `hc analyze [path]`
./hc                                              # analyze cwd (default)
./hc --since "6 months" --limit 20 --json --by-dir
./hc analyze -D -i --json | ./hc report           # decay + indentation → markdown report
./hc analyze --json | ./hc report --upsert HOTSPOTS.md  # inject into existing markdown
./hc prompt ignore | claude -p > .hcignore        # generate a .hcignore via LLM
```

Pre-commit hooks are configured (.pre-commit-config.yaml): go-fmt, go-vet, go-build, go-mod-tidy, golangci-lint, markdownlint.

## Architecture

Pipeline: **git history → complexity scan → classification → output** (+ optional report rendering)

```
cmd/hc/main.go          CLI entry (urfave/cli v3). Subcommands: analyze, report, prompt.
                        Root command shares analyze's flags + Action via analyzeFlags()
                        helper, so bare `hc [flags] [path]` is sugar for `hc analyze ...`.
internal/git/            Parses git log → []FileChurn {Path, Commits, WeightedCommits, Authors}
                         Supports decay weighting (decay.go) and rename tracking (rename.go)
internal/complexity/     Walks file tree, counts LOC or indentation depth → []FileComplexity {Path, Lines}
internal/analysis/       Merges on path, median-split thresholds, classifies → []FileScore
internal/output/         Formats results as table/JSON/CSV (adds SCORE column when decay enabled)
internal/ignore/         Gitignore-style pattern matching; loads .hcignore files
internal/report/         Renders analysis JSON as markdown; report.UpsertFile injects into
                         existing markdown via marker comments
internal/prompt/         Renders LLM prompts (currently: .hcignore generation prompt)
```

- **Threshold strategy**: median (p50) of commits and lines across all files — self-adaptive, no configuration needed.
- **Quadrant priority order**: HotCritical → HotSimple → ColdComplex → ColdSimple, then by weighted commits descending.
- **Deleted files** (in git history but not on disk) are excluded from results.
- **Directory mode** (`--by-dir/-d`) aggregates file scores into `[]DirScore` with summed metrics.
- **Decay** (`--decay/-D`): exponential decay weighting on commits; configurable half-life via `--decay-half-life` (default "6 months").
- **Complexity metrics**: LOC (default) or indentation depth (`--indentation/-i`).
- **Output format** (`--output/-o`): `table` (default), `json`, `csv`. `--json` is shorthand for `--output json`.
- **Limit** (`--limit/-n`): cap result count.
- **Exclude patterns** (`--exclude/-e`): repeatable flag, plus `.hcignore` file support.
- **Report writes**: `hc report --output FILE` overwrites; `--upsert FILE` injects between marker comments and preserves surrounding content. The two flags are mutually exclusive.
- **Rename tracking**: merges churn stats across git renames so renamed files aren't split.
- Only dependency beyond stdlib is `github.com/urfave/cli/v3`.

## Pending design work

`docs/design/cli-ergonomics.md` is the source of truth for in-flight CLI changes. Tier 1 has one item left: collapse `--decay` + `--decay-half-life` into a single `--decay[=HALFLIFE]` flag (#4). Tier 2 and Tier 3 items are not yet started.
