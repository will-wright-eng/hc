# Renamed/Moved File Tracking — Implementation Plan

## Context

When a file is renamed or moved, its churn history is split across the old and new paths. A file that accumulated 80 commits before being renamed and 20 commits after appears as two separate entries — one with 80 commits (filtered out because the old path no longer exists on disk) and one with 20 commits. The result is a file that should rank as a hotspot but doesn't.

This is a direct consequence of how the pipeline merges data: `git.Log()` returns paths exactly as they appeared in each commit, while `complexity.Walk()` returns only current on-disk paths. The merge in `analysis.Analyze()` joins on path string equality, so any churn attributed to a stale path is silently dropped.

---

## Options

### Option A: Post-processing rename map

Build a rename mapping after collecting raw churn data, then rewrite stale paths to their current equivalents before merging with complexity data.

**How it works:**

1. Run `git log --diff-filter=R --name-status --format=format:` to extract all rename events across the history window. Each rename event produces a line like `R100\told/path.go\tnew/path.go`.
2. Build a directed graph of old path -> new path. Chase chains: if `a -> b` and `b -> c`, resolve `a -> c`.
3. After `git.Log()` returns `[]FileChurn`, iterate through the results and replace each path with its resolved current path. Merge churn counts when multiple old paths resolve to the same current path.
4. Pass the rewritten `[]FileChurn` into `analysis.Analyze()` as usual.

**Pros:**

- Clean separation — rename resolution is a standalone transformation step between git extraction and analysis.
- No changes to `git.Log()` internals or `complexity.Walk()`.
- Easy to test in isolation: input a `[]FileChurn` with stale paths and a rename map, assert output has current paths with merged counts.

**Cons:**

- Requires a separate `git log` invocation to collect rename data, adding execution time.
- Chain resolution must handle cycles (unlikely but possible with rapid renames) and multiple files renamed to the same target.

### Option B: Inline rename resolution during git log parsing

Modify `gitLogFiles()` to use `--name-status` instead of `--name-only`, detect rename lines in-place, and emit current paths directly.

**How it works:**

1. Change the git log command from `--name-only` to `--name-status` so each file line includes a status prefix (`M`, `A`, `D`, `R`).
2. When a rename (`R`) line is encountered, record the mapping and emit only the new (destination) path for that commit.
3. After all commits are parsed, make a second pass to rewrite any paths that were later renamed (a file touched as `old.go` in commit 1 and renamed to `new.go` in commit 5 — commit 1's entry needs rewriting).

**Pros:**

- Single `git log` invocation handles both churn counting and rename detection.
- No separate rename data structure to maintain.

**Cons:**

- Tangles rename logic into the parsing loop, making `gitLogFiles()` harder to follow and test.
- Still needs a second pass to retroactively fix paths from earlier commits, so the "single pass" advantage is partially lost.
- Changes the `git log` output format, which affects `gitLogAuthors()` as well — both functions must be updated in lockstep.

### Option C: Per-file `--follow` tracking

Use `git log --follow` on each file to get its full history including renames.

**How it works:**

1. After `complexity.Walk()` returns the list of current on-disk files, run `git log --follow --oneline -- <path>` for each file to get the true commit count that follows the file across renames.
2. Replace the bulk `git.Log()` call with per-file follow queries, or use the per-file results to supplement the bulk results.

**Pros:**

- Git handles all rename detection natively — no custom chain resolution needed.
- Correct by construction for complex rename histories.

**Cons:**

- O(n) git invocations where n is the number of source files. For a repository with 1,000 source files, this means 1,000 separate `git log` calls. Prohibitively slow for any non-trivial repository.
- `--follow` only works for a single path — it cannot be used with bulk `git log` across the whole repository.
- Author tracking would need the same per-file treatment, doubling the invocation count.

---

## Recommendation: Option A

Option A keeps the rename logic isolated, testable, and additive. It introduces one new git invocation but avoids restructuring the existing parsing code. The rename map is a pure function: `([]FileChurn, RenameMap) -> []FileChurn`, which is straightforward to test and reason about.

Option B saves one git call but spreads rename concerns across the parsing layer. Option C is correct but too slow to be practical.

---

## Points of Integration

### New package: `internal/git/rename.go`

Add a new file within the existing `internal/git` package. Rename resolution is fundamentally a git operation — it parses git output and transforms git-derived data — so it belongs alongside the other git functions rather than in a new package.

```go
// RenameMap maps old file paths to their current (on-disk) equivalents.
type RenameMap map[string]string

// DetectRenames runs git log to find all rename events in the given time
// window and returns a resolved mapping of old path -> current path.
// Chains are collapsed: if a -> b -> c, the map contains a -> c and b -> c.
func DetectRenames(repoPath string, since string) (RenameMap, error)

// Resolve returns the current path for the given path. If the path has not
// been renamed, it is returned unchanged.
func (rm RenameMap) Resolve(path string) string
```

**`DetectRenames` implementation outline:**

1. Execute `git log --diff-filter=R --name-status --format=format:` with optional `--since`.
2. Parse each `R###\told\tnew` line to extract `(oldPath, newPath)` pairs.
3. Build the raw map, then resolve chains: for each key, follow the value chain until it reaches a path that is not itself a key.
4. Return the fully resolved map.

### `git.Log()`

After building the `FileChurn` slice from the existing parsing logic, apply the rename map:

```go
func Log(repoPath string, since string) ([]FileChurn, error) {
    // ... existing parsing ...

    renames, err := DetectRenames(repoPath, since)
    if err != nil {
        return nil, fmt.Errorf("detecting renames: %w", err)
    }

    // Rewrite paths and merge churn for renamed files.
    merged := make(map[string]*stats)
    for path, s := range m {
        resolved := renames.Resolve(path)
        if existing, ok := merged[resolved]; ok {
            existing.commits += s.commits
            // Merge author sets
            for _, a := range s.authors {
                existing.authors = append(existing.authors, a)
            }
        } else {
            merged[resolved] = s
        }
    }

    // ... build []FileChurn from merged map, dedup authors ...
}
```

This is the primary integration point. The rename map is applied inside `Log()` so that callers (including `analysis.Analyze()`) receive churn data with current paths. No downstream changes are needed.

### `analysis.Analyze()`

No changes required. Once `git.Log()` emits current paths, the existing `churnMap[cx.Path]` lookup will match correctly. Files that were renamed but still exist on disk will have their full churn history attributed to the current path.

### `complexity.Walk()`

No changes required. Walk already emits current on-disk paths.

### Output formatters

No changes required. Paths in the output will be current paths, as they are today.

### CLI (`cmd/hc/main.go`)

No new flags are needed for the initial implementation. Rename tracking should be on by default — there is no reason to show split churn when the tool can resolve it. If users need to disable it (e.g., for debugging or performance), a `--no-follow-renames` flag can be added later.

The `--since` flag is already passed to `git.Log()`, which will forward it to `DetectRenames()` to scope the rename search to the same time window.

---

## Flag Considerations

### Why no flag to enable/disable?

Rename tracking corrects a data accuracy problem. Showing split churn is not an alternative view — it's missing data. Making it opt-in means most users would never discover it. It should be the default behavior.

### Interaction with `--since`

The rename search uses the same `--since` window as the churn search. A rename that happened before the window is irrelevant — the old path won't appear in the churn data anyway. A rename within the window will be detected and resolved.

### Interaction with `--by-dir`

Directory aggregation uses `dirOf(fs.Path)`. Since paths are resolved to current locations before analysis, directory aggregation will naturally attribute churn to the correct directory — even if the file was moved between directories.

### Interaction with `--ignore`

If ignore patterns are implemented (see `ignore-patterns.md`), the ignore check should run after rename resolution. A file renamed from an ignored path to a non-ignored path should appear in results. The current path determines whether the file is ignored, not any historical path.

---

## Risks

### Rename detection misses copy-then-delete patterns

Git's rename detection is heuristic — it compares file similarity to determine if a deletion and addition in the same commit constitute a rename. If a file was copied to a new location in one commit and the original deleted in a later commit, git may not detect it as a rename.

**Mitigation:** This is a limitation of `git log --diff-filter=R` and cannot be fixed without custom similarity matching. Document it as a known limitation. Users can verify rename detection with `git log --follow -- <file>` for specific files.

### Chain resolution produces incorrect mappings

If file A is renamed to B, and later a new, unrelated file A is created, the rename map could incorrectly attribute old-A's churn to new-A through the stale mapping.

**Mitigation:** When resolving chains, verify that the final target path exists on disk (it will be in the complexity data). If the resolved path does not exist, drop the mapping — the file was renamed and then deleted, so it should be excluded anyway. Additionally, only record the most recent rename event for each source path, so a new file created at an old path does not inherit the old file's history.

### Performance on large repositories

The additional `git log` invocation for rename detection adds time. For most repositories, rename events are a small fraction of total history, so the output is small and parses quickly.

**Mitigation:** The `--since` flag naturally bounds the search. For repositories with very long histories, the default time window already limits the data. If profiling shows the rename query as a bottleneck, it can be run concurrently with the existing `gitLogFiles()` and `gitLogAuthors()` calls using a goroutine.

### Author deduplication after merge

When two `FileChurn` entries are merged (old path + new path -> current path), their author lists must be deduplicated. An author who committed to both the old and new path should be counted once.

**Mitigation:** The existing `Log()` function already deduplicates authors per file. The merge step must apply the same deduplication to the combined author list. Use the same map-based dedup approach already in `gitLogAuthors()`.

### Rename within the `--since` window vs. churn outside it

If a file was renamed within the `--since` window but most of its churn (under the old name) happened before the window, the rename will be detected but the old churn won't appear in the data. This is correct behavior — the `--since` flag means "only consider this time period" — but users might expect `--follow`-like behavior where old churn is included.

**Mitigation:** Document that `--since` applies to both churn counting and rename detection. If a user wants full history, they should omit `--since`. A future enhancement could add `--follow-before-window` to include pre-rename churn, but this adds complexity for a niche use case.
