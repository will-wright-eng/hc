# Indentation-Based Complexity — Implementation Plan

## Context

The current complexity metric is lines of code (LOC). LOC captures file size but not structural complexity — a 500-line lookup table and a 500-line function with deep nesting score identically. Indentation depth is a language-agnostic proxy for nesting complexity: every `if`, `for`, callback, and `try/catch` increases indentation in well-formatted code. Rather than building language-specific AST parsers, we measure what's already visible in the whitespace.

---

## How It Works

For each non-blank, non-comment line, measure the indent level (leading spaces divided by detected indent unit, or leading tab count). The file-level complexity score is the **sum of indent levels across all code lines**. This naturally combines size and depth — a large flat file and a small deeply-nested file can score equally, but a large deeply-nested file dominates.

Example:

```go
func simple() {          // indent 0
    return 1             // indent 1
}                        // indent 0
// sum = 1

func complex() {         // indent 0
    for _, v := range x {   // indent 1
        if v > 0 {          // indent 2
            process(v)       // indent 3
        }                    // indent 2
    }                        // indent 1
}                            // indent 0
// sum = 9
```

### Indent unit detection

Per-file heuristic: scan the first 100 non-blank lines, find the most common leading-whitespace delta between adjacent lines. Common results are 2, 4, or 8 spaces; tabs are counted directly. If detection fails (no indentation variation), default to 4 spaces.

---

## Points of Integration

### `FileComplexity` struct

Add a `Complexity int` field. `Lines` remains and is always populated regardless of metric.

```go
type FileComplexity struct {
    Path       string
    Lines      int
    Complexity int  // indent sum when metric=indentation, same as Lines when metric=loc
}
```

When metric is `loc`, `Complexity` mirrors `Lines` so downstream code can always use `Complexity` for thresholds without branching.

### `Walk()` function

`Walk(root string, metric string) ([]FileComplexity, error)`

The `metric` parameter (`"loc"` or `"indentation"`) determines how `Complexity` is populated. File traversal, filtering, and LOC counting remain unchanged. When metric is `"indentation"`, an additional pass (or combined pass) computes the indent sum.

New file: `internal/complexity/indentation.go` — contains `IndentSum(path string) (int, error)`.

### `analysis.Analyze()`

Currently uses `FileComplexity.Lines` for median threshold calculation. Change to use `FileComplexity.Complexity`. Since `Complexity == Lines` in LOC mode, this is a no-op for existing behavior.

### `FileScore` struct

Add a `Complexity int` field alongside `Lines` so the output layer has access to both.

### Output formatters

When metric is `indentation`, replace the `LINES` column with `COMPLEXITY` in table/CSV output. JSON output includes both `lines` and `complexity` fields always, with a `metric` field indicating which was used for classification.

### CLI (`cmd/hc/main.go`)

Pass the metric value from the flag through to `complexity.Walk()` and propagate to output formatters for column labeling.

---

## Flag Considerations

### `--complexity-metric loc|indentation`

- Default: `loc` — preserves current behavior, no surprise for existing users.
- `indentation` — enables indent-sum scoring.
- Future values (e.g. `cyclomatic`) can be added without breaking the interface.

### Interaction with Future Rollups

Any future directory or mixed-granularity rollup can sum `Complexity` across files the same way it sums `Lines`. No special handling is needed — indent sums are additive.

### Interaction with `--format`

- `table`: column header changes from `LINES` to `COMPLEXITY` when metric is not `loc`.
- `json`: always includes both `lines` and `complexity` fields, plus a top-level or per-entry `metric` field.
- `csv`: column header changes to match table behavior.

### Flag naming alternatives considered

- `--metric` — too generic, could be confused with churn metric.
- `--complexity` — conflicts with being both a flag name and an output column.
- `--complexity-metric` — explicit, namespaced, leaves room for `--churn-metric` later.

---

## Risks

### Unformatted code

If a codebase doesn't use a formatter, indentation may not reflect structure. A file with inconsistent or absent indentation will score artificially low.

**Mitigation:** This is a known limitation, not something we try to fix. The tool's accuracy depends on formatted code, which is the norm in modern development (gofmt, prettier, black, clang-format). Document this assumption. Consider a future `--warn-flat` flag that flags files with suspiciously low max indent for their size.

### Mixed tabs and spaces within a file

A file mixing tabs and spaces will produce unreliable indent levels.

**Mitigation:** Detect the dominant indent character per file (whichever appears more in leading whitespace). Normalize all leading whitespace to the dominant style before measuring. If the split is close to 50/50, fall back to treating the file as LOC-only.

### Tab width ambiguity

One tab could mean 2, 4, or 8 columns of visual indentation. Two files with identical structure but different tab conventions would score the same (1 indent level per tab), which is correct — we're measuring nesting depth, not visual width.

**No mitigation needed.** Tabs-as-single-indent is the right behavior.

### Language idioms inflating scores

Some patterns add indentation without meaningful complexity:

- Go's `if err != nil { return err }` at every call site.
- Java builder patterns and fluent APIs.
- JavaScript promise chains and callback pyramids (though those *are* complex).

**Mitigation:** These inflate scores consistently across all files in the same language, so relative ranking within a codebase is preserved. Cross-language comparison is already unreliable with LOC; indentation doesn't make it worse. Not worth special-casing.

### Generated code

Generated files (protobuf stubs, ORM models, serialization code) may have deep nesting from mechanical patterns, not human complexity.

**Mitigation:** Already partially handled by directory skipping (vendor, node_modules). The `--ignore` flag (roadmap item #3) is the proper solution. Not a blocker for this feature.

### Score magnitude is unintuitive

An indent sum of 4,372 doesn't mean anything to a user in isolation. Unlike LOC (which people have intuition for), indent sums are only meaningful relative to other files.

**Mitigation:** This is fine for the quadrant classification use case — we only need relative ranking to split at the median. The output already shows the raw number; users will develop intuition over time. Consider adding percentile annotations in a future iteration.
