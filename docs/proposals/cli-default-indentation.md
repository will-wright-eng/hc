# Default to Indent-Sum Complexity — Implementation Plan

## Context

The `--indentation` / `-i` flag exists because LOC came first. When indent-sum was added later, it was bolted on as an opt-in metric to avoid changing existing behavior. But indent-sum is the better signal: it correlates with control-flow nesting, is less inflated by long literals or repetitive boilerplate, and is the metric Adam Tornhill's original methodology actually recommends.

Today the flag presents a choice the user shouldn't have to make. This plan removes `--indentation` / `-i` and makes indent-sum the only complexity metric. LOC stays in the output as a display column — it's still useful as a "size" signal — but no longer drives quadrant classification.

This follows the same pattern as the decay refactor (`docs/proposals/cli-decay-flag.md`): remove a flag whose default is the wrong choice, and make the "good" behavior unconditional.

---

## Why indent-sum is the better default

- **Correlates with structural complexity**, not text volume. A 500-line config table has high LOC but low indent-sum. A deeply nested 100-line state machine is the opposite — and it's the one that actually carries risk.
- **Language-agnostic.** Works for any language with consistent indentation. Doesn't depend on cyclomatic estimators or AST parsers.
- **Cheap to compute** — a single linear pass over the file.
- **Tornhill's recommendation.** The methodology this tool is based on (`docs/info/hot-cold-fundamental-principle.md`) explicitly prefers indent-based proxies over raw LOC.

---

## Design

### Flag shape

Drop `--indentation` / `-i` entirely. There is no `--loc` opt-out. If a real need surfaces, adding it later is easier than removing it.

```
hc                    # classify by indent-sum (only mode now)
hc --by-dir           # same metric, aggregated
hc --no-decay         # raw counts, still indent-sum
```

### What changes vs. today

| Aspect | Before | After |
|---|---|---|
| Classification metric | LOC by default; indent-sum if `-i` | Always indent-sum |
| `Lines` field on `FileComplexity` | Always populated (LOC) | Unchanged — still populated |
| `Complexity` field | Equals `Lines` (LOC mode) or indent-sum (`-i` mode) | Always indent-sum |
| Threshold (median split) | Computed over `Complexity` | Same — but now always indent-sum-based |
| Output table columns | `LINES` (only) | `LINES` and `COMPLEXITY` (the latter is indent-sum) |
| JSON `metric` field | `"loc"` or `"indentation"` | Drop the field |
| Report header | `Complexity metric: **loc**` (or indentation) | Drop the line; or hard-code `indent-sum` |

### Why keep LOC?

It's free — the file walk already counts lines, and the display benefits from the size signal. A 1,000-line file with low indent-sum is a different beast than a 100-line file with high indent-sum, and seeing both columns helps the reader understand the score.

### Bundled performance fix

Today, when `-i` is on, every file is read twice (`countLines` then `IndentSum`). Making indent-sum the default would impose that cost on every run. Bundle a single-pass implementation: one scanner per file, returning `(lines, indentSum, err)`. This is a small refactor in `internal/complexity/` and avoids a real regression.

### Side benefit: clears the `-i` cross-subcommand overlap

The simplify reviewer flagged that `analyze -i` (indentation) and `report -i` (input) shared a short alias. Both worked because they're scoped to their subcommand, but the conceptual overlap was real. Removing `--indentation` retires that conflict.

---

## Scope

**In scope.**

- Remove `--indentation` / `-i` from `analyzeFlags()` and the `metric` derivation in `runAnalyze`.
- Drop the `metric` parameter from `complexity.Walk`, `output.FormatFiles`, `output.FormatDirs`.
- Combine `countLines` + `IndentSum` into a single-pass function returning both counts.
- Remove the `metric` field from output JSON structs (`fileEntry`, `dirEntry`).
- Update `report.Render` to stop expecting a `metric` field; remove the header line that prints it.
- Add a `COMPLEXITY` column to table and CSV output (indent-sum), alongside the existing `LINES` column.
- Update tests that pass `"indentation"` / `"loc"` to `complexity.Walk` or include `metric` in fixture JSON.
- Update `CLAUDE.md` and `readme.md`.

**Out of scope.**

- A blended score (e.g., `indentSum / Lines`) — separate methodology question worth its own proposal if it goes further.
- Changing `IndentSum`'s tab-width or indent-detection logic.
- Reordering or hiding the `LINES` column entirely.
- Parser improvements for comments/strings inside `countLines`.

---

## Touch Points

- `cmd/hc/main.go` — drop the `--indentation` flag entry; drop the `metric := "loc" / metric = "indentation"` derivation; update calls to `complexity.Walk` and `output.FormatFiles` / `FormatDirs`.
- `internal/complexity/complexity.go` — drop the `metric` parameter from `Walk`; replace the dual `countLines` + conditional `IndentSum` block with a single-pass call.
- `internal/complexity/indentation.go` — keep `IndentSum` (or fold its logic into the new combined scanner); add `scanFile(path) (lines, indentSum int, err error)` or similar.
- `internal/complexity/complexity_test.go`, `indentation_test.go` — drop calls passing `"indentation"` / `"loc"`; cover the combined scanner.
- `internal/output/output.go` — drop the `metric` parameter from `FormatFiles` / `FormatDirs` (and all internal callees); remove the `Metric` JSON field; remove `complexityColumnLabel`; add a fixed `COMPLEXITY` column to the table and CSV outputs.
- `internal/output/output_test.go` — update assertions that depend on column layout or metric labels.
- `internal/report/report.go` — drop the `metric` field from `fileEntry` / `dirEntry` decode targets; drop the `Complexity metric: **X**` header line.
- `internal/report/report_test.go` — update fixture JSON to remove `"metric"` keys; update assertions that look for the metric line in rendered output.
- `CLAUDE.md` — update the "Complexity metrics" bullet.
- `readme.md` — remove the `--indentation` row from the analyze flag table.

---

## Code Changes (sketch)

### `cmd/hc/main.go`

Remove from `analyzeFlags()`:

```go
&cli.BoolFlag{
    Name:    "indentation",
    Aliases: []string{"i"},
    Usage:   "Use indentation-based complexity instead of LOC",
},
```

Remove from `runAnalyze`:

```go
metric := "loc"
if cmd.Bool("indentation") {
    metric = "indentation"
}
```

Update calls:

```go
complexities, err := complexity.Walk(absPath, ig)
// ...
return output.FormatFiles(os.Stdout, scores, format, decay)
return output.FormatDirs(os.Stdout, dirs, format, decay)
```

### `internal/complexity/complexity.go`

```go
func Walk(root string, ig *ignore.Matcher) ([]FileComplexity, error) {
    // ...
    lines, indentSum, err := scanFile(path)
    if err != nil { return nil }
    if lines > 0 {
        results = append(results, FileComplexity{
            Path:       rel,
            Lines:      lines,
            Complexity: indentSum,
        })
    }
    // ...
}
```

`scanFile` is the new single-pass function. Replaces the existing `countLines` + `IndentSum` pair (or wraps them around a shared scanner).

### `internal/output/output.go`

Drop the `metric` parameter throughout. Drop `complexityColumnLabel`. The table now always has both `LINES` and `COMPLEXITY` columns (today the table has only `LINES`). Drop the `Metric` field from `fileEntry` / `dirEntry`.

### `internal/report/report.go`

Drop the `metric` field from the entry structs. Drop the `Complexity metric: **X**` line from the rendered output. The summary copy can either stay generic or hard-code "indent-sum scoring" if useful.

---

## Verification

```bash
make build && make lint && make test
./hc                                      # default classification, indent-sum-based
./hc --json | jq '.[0]'                   # confirm no "metric" field in output
./hc --no-decay --limit 5                 # works alongside the no-decay default
./hc -i                                   # error: flag provided but not defined
./hc --indentation                        # error: flag provided but not defined
./hc analyze --json | ./hc report         # report header should not include the metric line
make e2e
```

Sanity-check that quadrant assignments shift in expected ways from the prior LOC default (e.g., a long config-table file may move from `Hot Critical` → `Hot Simple` if its indent-sum is low despite high LOC).

---

## Suggested Commit

> `complexity: remove --indentation flag, default to indent-sum scoring`
>
> Drops the `--indentation`/`-i` flag and makes indent-sum the only
> complexity metric. LOC stays as a display column but no longer drives
> classification. Folds `countLines` + `IndentSum` into a single pass to
> avoid the per-file read regression. Removes the `metric` field from the
> analyze JSON contract and the `Complexity metric: X` line from the
> report header.

---

## Out of Scope (Reminder)

- Blended scoring (indent-sum normalized by lines, weighted combinations) — separate methodology proposal.
- A `--loc` opt-out — defer until a real need surfaces.
- `IndentSum` algorithm tweaks (tab-width detection, etc.) — independent.
