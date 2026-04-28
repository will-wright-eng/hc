# Directory-Level Aggregation — Proposal

## Context

`hc --by-dir` aggregates file-level scores into directory-level scores using each file's full parent directory as the grouping key. On a flat layout this is fine; on a nested codebase it produces one row per leaf directory, which fragments the very signal aggregation is supposed to produce.

Example on this repo today:

```text
hc --by-dir
internal/git
internal/output
internal/analysis
internal/complexity
internal/ignore
internal/report
internal/prompt
cmd/hc
```

Eight rows, all roughly the same shape, when the question the user is usually asking is "where is the heat at the `internal/` vs `cmd/` level?"

This proposal adds a depth cap so the user can collapse aggregation to a chosen level of the tree.

## Convention

Two strong priors in the Unix toolchain:

- `tree -L N` — limit display to N levels deep. The most common reach for "show me the shape of this tree at level N."
- `du --max-depth=N` (`du -d N` on BSD) — same idea applied to disk usage rollups. Notably, `du -d 0` produces a single total for the argument path; `du -d 1` shows top-level subdirs only.

Both establish the mental model: pick a depth, collapse everything below it.

## Design

Add a single flag: `-L, --level N`.

```bash
hc -L 1               # aggregate at depth 1: one row per top-level dir
hc -L 2               # aggregate at depth 2: cmd/hc, internal/git, internal/output, ...
hc -L 0               # single bucket — whole repo as one row
hc                    # unchanged: file-level output
hc --by-dir           # unchanged: leaf-directory aggregation (current behavior)
hc --by-dir -L 2      # explicit; same as -L 2
```

### Decisions

**`-L` implies directory aggregation.** When `-L` is present without `--by-dir`, behave as if `--by-dir` was passed. Forcing the user to type both for the common case adds friction with no benefit — the level concept is meaningless on file-level output.

**`-L` and `--by-dir` remain separate flags.** They are not collapsed into one because `--by-dir` is intended to evolve into the `--by file|dir|author` enum proposed in `cli-ergonomics.md` §7. Subsuming `-L` into that enum would tangle two orthogonal concepts: *what to group by* (dir/author/etc.) and *how deep to roll up* (only meaningful for dir). Keeping them separate lets the enum land cleanly later, with `-L` composing naturally with `--by dir`.

**`-L 0` means "single bucket."** Mirrors `du -d 0`. Useful for whole-repo summary rows and consistent with the depth model. `tree -L 0` errors instead, but `tree`'s constraint is about display nesting, which doesn't apply here.

**Files shallower than N keep their own depth.** A file at `main.go` (depth 1) under `-L 3` aggregates as `.` (or `main.go`'s parent, which is the repo root). It does not get padded. Concretely: the grouping key is `min(file_depth, N)` segments of the file's parent path.

**Files deeper than N are truncated.** A file at `internal/git/sub/sub2/foo.go` under `-L 2` aggregates into `internal/git`. No ellipsis, no marker — the truncated path *is* the key, exactly like `tree -L 2` shows the directory without indicating what lies below.

**Default remains file-level.** No `-L` means current behavior. `--by-dir` without `-L` means current leaf-aggregation behavior. Nothing existing changes; `-L` is purely additive.

### Short-flag choice

`-L` matches `tree`. `-d` is already taken by `--by-dir`, which would have been the `du` analogue, so the choice is forced anyway. `--level` reads naturally as the long form; `--depth` is a defensible alternative but `tree` is the closer mental model since the user is selecting a layer of a hierarchy, not measuring distance.

### Validation

- `-L N` where N < 0 → error: `--level must be >= 0`.
- `-L 0` is valid (single bucket).
- No upper cap. Passing `-L 99` on a shallow tree behaves like leaf-aggregation — files at lower depth aren't padded, so it degrades gracefully to current `--by-dir` semantics.

## Implementation Sketch

The change is local to `internal/analysis/`:

1. `analysis.AnalyzeByDir` already groups `[]FileScore` by directory path. Add a `level int` parameter (or a small options struct if more dir-mode knobs follow).
2. Before grouping, transform each score's directory key:

   ```go
   func capDepth(dir string, level int) string {
       if level <= 0 {
           return "."
       }
       parts := strings.Split(dir, string(filepath.Separator))
       if len(parts) > level {
           parts = parts[:level]
       }
       return filepath.Join(parts...)
   }
   ```

3. `cmd/hc/main.go`: add the `-L, --level` flag (hidden on root, visible on `analyze`, matching the recently-introduced pattern). When `level` is set and `--by-dir` is not, force `byDir = true`.
4. `internal/output/`: no changes — directory rows already render the path verbatim.

Estimated change size: ~30 lines including the flag plumbing and a couple of test cases (depth 0 / depth N / deeper-than-tree).

## Tests

Add to `internal/analysis/analysis_test.go`:

- `TestAnalyzeByDir_Level0_SingleBucket` — all files collapse to `.`.
- `TestAnalyzeByDir_Level1_TopLevelOnly` — `cmd/hc/main.go` and `cmd/hc/util.go` both aggregate into `cmd`.
- `TestAnalyzeByDir_LevelExceedsDepth` — files shallower than N keep their natural depth.
- `TestAnalyzeByDir_DefaultLevelIsLeaf` — passing 0 segments / no-op level preserves current behavior.

CLI-level: `make e2e` smoke-tests the flag end-to-end via the existing pipeline.

## Out of Scope

- The `--by file|dir|author` enum (cli-ergonomics §7). `-L` should land first; the enum can adopt it later as `--by dir -L 2` without API churn.
- Display tweaks for the truncated path (e.g. trailing slash, `…` indicator). Bare path mirrors `tree -L`.
- Per-row file counts within a directory rollup. Useful, but a separate column-level decision.

## Summary

`-L N` adds a tree-style depth cap to directory aggregation. It implies `--by-dir` so the common case is one flag, but stays a distinct flag so the future `--by file|dir|author` enum can land without rework. `-L 0` follows `du`'s "single bucket" semantics; deeper files truncate without a marker; shallower files keep their natural depth. Implementation is a key-transformation in `analysis.AnalyzeByDir` plus flag plumbing — roughly 30 lines.
