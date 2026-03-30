# Ignore Patterns — Implementation Plan

## Context

The hotspot matrix includes every source file that appears in both git history and on disk. In practice, many of these files are not actionable — vendor code, generated protobuf stubs, test fixtures, compiled assets, and build artifacts add noise without surfacing real hotspots. The existing `shouldSkipDir` hardcodes a small set of directories (`.git`, `node_modules`, `vendor`, etc.), but users have no way to exclude project-specific paths or file patterns.

An `--ignore` flag and `.hcignore` config file let users tailor the analysis to the code they actually own and maintain.

---

## How It Works

Ignore patterns follow `.gitignore` syntax — the same globbing rules developers already know. Patterns are collected from two sources:

1. **`.hcignore` file** in the repository root (if present), one pattern per line.
2. **`--ignore` flags** on the command line, each providing one pattern.

Command-line patterns are additive with `.hcignore`. There is no way to negate or override `.hcignore` entries from the CLI — if a pattern is in `.hcignore`, it applies. Blank lines and lines starting with `#` in `.hcignore` are ignored (comments).

Example `.hcignore`:

```
# Generated code
*.pb.go
*_gen.go
internal/generated/**

# Test fixtures
testdata/**

# Build output
dist/**
```

Example CLI usage:

```bash
hc analyze --ignore "*.pb.go" --ignore "testdata/**" --top 20
```

### Pattern matching

Patterns are matched against the file's relative path from the repository root (the same path shown in the output). Matching uses `filepath.Match` semantics extended with `**` for recursive directory matching:

- `*.pb.go` — matches any `.pb.go` file at any depth.
- `testdata/**` — matches everything under any `testdata/` directory.
- `internal/generated/**` — matches a specific subtree.
- `docs/` — matches the `docs` directory and everything under it.

A file is excluded if it matches **any** pattern from either source.

---

## Points of Integration

### New package: `internal/ignore`

A single new file: `internal/ignore/ignore.go`

```go
// Matcher tests relative paths against a set of ignore patterns.
type Matcher struct {
    patterns []string
}

// New creates a Matcher from the combined set of patterns.
func New(patterns []string) *Matcher

// Match returns true if the given relative path should be ignored.
func (m *Matcher) Match(relPath string) bool
```

Keeping the matcher in its own package lets both `complexity.Walk` and the post-`git.Log` filter use the same logic without circular imports.

### `.hcignore` loading

A helper function in the same package:

```go
// LoadFile reads patterns from a .hcignore file. Returns nil (no patterns)
// if the file does not exist. Returns an error only for read failures.
func LoadFile(path string) ([]string, error)
```

This parses the file line-by-line, stripping blank lines and `#` comments. It does not expand globs — patterns are stored as-is for the matcher.

### `complexity.Walk()`

`Walk` currently accepts `(root string, metric string)`. Add an optional matcher:

```go
func Walk(root string, metric string, ignore *ignore.Matcher) ([]FileComplexity, error)
```

Inside the `filepath.Walk` callback, after computing `rel`, check `ignore.Match(rel)` before counting lines. A `nil` matcher means no filtering (preserves the zero-value behavior for callers that don't need it).

This is the natural place to filter because `Walk` already has the relative path and already skips directories. Adding ignore checks here means excluded files are never opened or counted.

### `git.Log()`

`Log` currently accepts `(repoPath string, since string)`. Add an optional matcher:

```go
func Log(repoPath string, since string, ignore *ignore.Matcher) ([]FileChurn, error)
```

Filter happens after parsing git output: when building the `FileChurn` map, skip any path where `ignore.Match(path)` returns true. This ensures churn data and complexity data are filtered consistently — a file excluded from one won't appear in the other.

### `analysis.Analyze()`

No changes needed. `Analyze` merges on path intersection — if a file is excluded from both churn and complexity inputs, it never appears in the output. The ignore layer sits upstream.

### Output formatters

No changes needed. Formatters render whatever `Analyze` produces.

### CLI (`cmd/hc/main.go`)

Add the `--ignore` flag (multi-value string slice) and wire it together:

```go
&cli.StringSliceFlag{
    Name:  "ignore",
    Usage: "Glob pattern to exclude (repeatable, .gitignore syntax)",
},
```

In `runAnalyze`:

1. Read `--ignore` flag values.
2. Call `ignore.LoadFile(filepath.Join(absPath, ".hcignore"))` to get file-based patterns.
3. Combine both slices into a single `ignore.New(patterns)` matcher.
4. Pass the matcher to `complexity.Walk()` and `gitpkg.Log()`.

---

## Flag Considerations

### `--ignore` (repeatable)

- Each invocation adds one pattern: `--ignore "*.pb.go" --ignore "docs/**"`.
- Patterns are additive with `.hcignore`. The combined set is OR'd — matching any single pattern excludes the file.
- An empty `--ignore` (no flags, no `.hcignore`) produces a nil matcher, preserving current behavior exactly.

### Interaction with `--by-dir`

Directory-level aggregation happens after file-level analysis. Ignored files are excluded before aggregation, so directory scores reflect only the non-ignored files. A directory with all files ignored will not appear in the output.

### Interaction with `--complexity-metric`

No interaction. Ignore patterns filter files before complexity is measured, regardless of which metric is used.

### Why not `--include`?

An include-list (allowlist) inverts the mental model — users would need to enumerate what they want rather than what they don't. Most repositories have a small set of paths to exclude and a large set to include. Exclude patterns are more ergonomic for this. If demand appears, `--include` can be added later without conflicting.

### Why `.hcignore` and not reuse `.gitignore`?

`.gitignore` excludes files from version control — those files won't appear in `git log` output anyway. The files we want to ignore for hotspot analysis are tracked files that happen to be uninteresting (generated code, vendored deps that are committed, test fixtures). These are precisely the files `.gitignore` does *not* list. A separate `.hcignore` avoids overloading `.gitignore` semantics.

---

## Risks

### Pattern syntax confusion

Users may expect full regex support or `.gitignore` edge cases like negation patterns (`!important.pb.go`).

**Mitigation:** Document that patterns use `.gitignore`-style globbing, not regex. Defer negation support — it adds complexity for a rare use case. If a user needs to un-ignore a file, they can remove the pattern from `.hcignore` or narrow it.

### `**` matching complexity

Go's `filepath.Match` does not support `**` natively. Implementing recursive matching requires either a third-party library or a custom matcher.

**Mitigation:** Implement a minimal `**` expansion: split the pattern on `**`, match the prefix against the directory path and the suffix against the remaining path. This covers the practical cases (`testdata/**`, `**/*.pb.go`) without needing a full globbing engine. If edge cases appear, consider adopting `doublestar` as a dependency — but avoid it initially to maintain the stdlib-only constraint.

### Performance on large repositories

Checking every file against every pattern is O(files × patterns). For typical repositories (thousands of files, tens of patterns), this is negligible. For monorepos with hundreds of thousands of files and many patterns, it could add latency.

**Mitigation:** Not a concern for the initial implementation. If profiling shows pattern matching as a bottleneck, patterns can be compiled into a prefix trie or the matcher can short-circuit on directory prefixes.

### Inconsistent filtering between churn and complexity

If the ignore matcher is applied to one pipeline but not the other, files could appear in churn data but not complexity (or vice versa), leading to missing entries in the output.

**Mitigation:** The matcher is created once in `runAnalyze` and passed to both `git.Log()` and `complexity.Walk()`. This is enforced by the wiring in `main.go`, not by convention. The matcher is the same object — there's no way for the two pipelines to diverge.

### `.hcignore` not found in subdirectory analysis

When running `hc analyze path/to/subdir`, the `.hcignore` file should still be loaded from the repository root, not from the subdirectory.

**Mitigation:** Locate `.hcignore` relative to the git root (`git rev-parse --show-toplevel`), not relative to the analysis path. This matches how `.gitignore` works and avoids confusion when analyzing subtrees.
