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
./hc                                                    # analyze cwd (default; decay on)
./hc --since "6 months" --json
./hc --no-decay                                         # raw commit counts, no recency weighting
./hc --no-min-age                                       # disable the 14-day age floor
./hc analyze --json | ./hc md report                    # JSON pipeline → markdown report (e.g. HOTSPOTS.md)
./hc analyze --json | ./hc md report --upsert AGENTS.md # inject into existing markdown (e.g. AGENTS.md)
./hc md ignore | claude -p > .hcignore                  # generate a .hcignore via LLM
```

Pre-commit hooks are configured (.pre-commit-config.yaml): go-fmt, go-vet, go-build, go-mod-tidy, golangci-lint, markdownlint.

## Architecture

Pipeline: **git history → complexity scan → classification → output** (+ optional report rendering)

```text
cmd/hc/main.go          CLI entry (urfave/cli v3). Subcommands: analyze, md (report, ignore).
                        Root command shares analyze's flags + Action via analyzeFlags()
                        helper, so bare `hc [flags] [path]` is sugar for `hc analyze ...`.
internal/git/            Parses git log → []FileChurn {Path, Commits, WeightedCommits, Authors, FirstSeen}
                         Supports decay weighting (decay.go) and rename tracking (rename.go)
internal/complexity/     Walks file tree, counts LOC or indentation depth → []FileComplexity {Path, Lines}
internal/analysis/       Merges on path, median-split thresholds, classifies → []FileScore
internal/output/         Formats results as table/JSON/CSV (LINES + COMPLEXITY columns;
                         adds SCORE column when decay enabled)
internal/ignore/         Gitignore-style pattern matching; loads .hcignore files
internal/md/             Markdown renderers: report.go (analysis JSON → markdown, with
                         UpsertFile for marker-bounded injection); ignore.go + summary.go
                         (LLM prompt for .hcignore generation); comment.go (per-file PR
                         comment NDJSON, dynamic stats table). Templates in templates/.
```

- **Threshold strategy**: median (p50) of commits and lines across all files — self-adaptive, no configuration needed.
- **Quadrant priority order**: HotCritical → HotSimple → ColdComplex → ColdSimple, then by weighted commits descending.
- **Deleted files** (in git history but not on disk) are excluded from results.
- **Decay**: commits are weighted by recency by default; half-life adapts to the analyzed window (= age of oldest commit in scope). Use `--no-decay` for raw commit counts. Narrow the window via `--since` to shorten the half-life.
- **Complexity metric**: indent-sum (always). Each non-blank, non-comment line contributes its indent depth; classification thresholds are the median of indent-sum across files. LOC is still computed and shown as a display column but does not drive classification.
- **Output format** (`--output/-o`): `table` (default), `json`, `csv`. `--json` is shorthand for `--output json` and cannot be combined with `--output <non-json>` (returns an error).
- **Exclude patterns** (`--exclude/-e`): repeatable flag, plus `.hcignore` file support.
- **Report writes**: `hc md report --output FILE` overwrites; `--upsert FILE` injects between marker comments and preserves surrounding content. The two flags are mutually exclusive.
- **Rename tracking**: merges churn stats across git renames so renamed files aren't split.
- **File age floor**: files whose first commit is younger than 14 days are excluded from analysis output (the median-split is unfair to files that haven't had time to accumulate churn). Auto-disables when `--since` is 30 days or less, with a one-line stderr note. Disable explicitly with `--no-min-age`. `FirstSeen` is bounded by the `--since` window — see `docs/proposals/file-age-floor.md` for the limitation and the planned Phase 2 fix.
- Only dependency beyond stdlib is `github.com/urfave/cli/v3`.
