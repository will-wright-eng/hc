# scc Analysis: Architecture and Performance Optimization Strategies

## Overview

[scc](https://github.com/boyter/scc) (Sloc Cloc and Code) is a Go-based tool for counting lines of code, comments, blanks, and estimating complexity across ~300 languages. It is a purely static file analyzer — it operates on the current working tree and has no interaction with git history, commits, or blame. Its relevance to `hc` is as a reference for high-performance file traversal and analysis in Go, and as a potential complexity backend via `scc --format json`.

---

## Architecture

scc uses a **channel-based pipeline** with bounded concurrency at each stage. The pipeline is defined in `processor/processor.go`:

```
Directory Walker → File Filter → File Processors (N workers) → Summarizer
     (8 goroutines)    (1 goroutine)    (NumCPU * 4 goroutines)     (1 goroutine)
```

### Stage 1: Directory Walking

Uses `github.com/boyter/gocodewalker`, a custom parallel file-walking library also by boyter. Key characteristics:

- Walks multiple directory branches concurrently using a counting semaphore (default 8 concurrent walkers)
- Uses `errgroup` for coordinated goroutine management
- Natively respects `.gitignore`, `.ignore`, `.gitmodules`, and custom `.sccignore` files
- Contains a vendored pure-Go gitignore parser — no git binary or library required
- Outputs `*File` structs to a channel (`potentialFilesQueue`)

### Stage 2: File Filtering

A single goroutine reads from the walker output and applies:

- Regex-based path exclusions
- `os.Lstat` for metadata (skips symlinks, checks size)
- Language detection by file extension and filename
- Allow/exclude list filtering
- Early rejection of files exceeding a byte-size threshold

Accepted files become `FileJob` structs sent to `fileListQueue`.

### Stage 3: File Processing

`FileProcessJobWorkers` (default `runtime.NumCPU() * 4`) goroutines consume from `fileListQueue`. Each worker:

1. Reads the file into a reusable buffer
2. Detects language (including shebang-based detection for extensionless files)
3. Runs `CountStats()` — a byte-level state machine that classifies each line as code, comment, or blank while simultaneously computing complexity

Results are sent to `fileSummaryJobQueue`.

### Stage 4: Summarization

A single goroutine aggregates results by language and formats output (table, JSON, CSV, etc.).

---

## Performance Optimization Strategies

### Garbage Collection Manipulation

```go
debug.SetGCPercent(-1) // disable GC at startup
```

GC is disabled entirely during the file-read phase. After processing `GcFileCount` files (default 10,000), GC is re-enabled. This eliminates GC pauses during the I/O-heavy hot path where many short-lived allocations occur. The tradeoff is higher peak memory usage, which is acceptable for a CLI tool that runs and exits.

### Worker Pool Oversubscription

File processing workers are set to `NumCPU * 4`, deliberately oversubscribing the CPU. The rationale is that workers frequently block on file I/O reads. Oversubscription ensures that while some goroutines wait on disk, others are actively processing. This is effective on both spinning disks (high latency) and SSDs (lower latency but still non-zero).

### Reusable Read Buffers

Each worker maintains a persistent `FileReader` with a `bytes.Buffer` that is reused across files via `Reset()` (which retains the underlying allocation). This avoids per-file heap allocation for the common case. If a buffer grows past `LargeByteCount`, it is replaced with a fresh buffer to release the oversized allocation back to the system.

### Trie-Based Token Matching

Language features — comment delimiters, string literals, complexity keywords — are compiled into trie data structures for multi-pattern matching. This allows the state machine to check for all possible tokens at a given position in a single trie traversal rather than testing each pattern individually.

A `ProcessMask` byte acts as a pre-filter: `shouldProcess(curByte, processBytesMask)` performs a single bitwise AND to determine if the current byte could possibly start any token for the current language. If not, the trie lookup is skipped entirely. This eliminates the vast majority of trie lookups since most bytes in source code are not the start of a comment, string, or keyword.

### Bloom Filter for Extensions

`bloom.go` implements a bloom filter using a seeded PRNG that maps bytes to well-distributed 64-bit values with exactly 3 bits set. This provides O(1) extension lookups with minimal memory, avoiding hash map overhead for the common case of checking whether a file extension is recognized.

### Duplicate Detection via BLAKE2b

When `--duplicates` is enabled, file content is hashed using BLAKE2b-256 during the same byte-scan pass that counts lines. The hash computation piggybacks on the existing read — no second pass over the file is needed. Hashes are stored in a `map[int64][][]byte` keyed by file size, so only files of the same size are compared, reducing hash collision checks.

### Lazy Language Feature Loading

When invoked from the CLI, `isLazy = true` causes language feature structures (tries, masks) to be built on-demand as each language is first encountered. Since a typical repository contains 5-15 languages out of ~300 supported, this avoids constructing ~285 unused trie structures at startup.

### Binary File Detection Shortcut

Only the first 10,000 bytes of a file are checked for NUL characters to determine if a file is binary. If a NUL is found, the file is skipped immediately without reading the remainder.

### Extension Cache

A `sync.Map`-based cache stores extracted file extensions to avoid redundant string processing when many files share the same extension (common in any repository).

---

## Dependency Profile

| Dependency | Purpose |
|---|---|
| `github.com/boyter/gocodewalker` | Parallel directory walking with gitignore support |
| `github.com/boyter/simplecache` | Simple caching |
| `github.com/json-iterator/go` | Fast JSON serialization |
| `github.com/spf13/cobra` / `pflag` | CLI framework |
| `golang.org/x/crypto` | BLAKE2b hashing for duplicate detection |
| `github.com/mark3labs/mcp-go` | MCP protocol support |

No git library dependencies. The only git-related functionality is `.gitignore` pattern matching via a vendored pure-Go parser in `gocodewalker`.

---

## Relevance to hc

### What to adopt

- **Channel-based pipeline architecture.** The walker → filter → process → summarize pattern maps cleanly onto `hc`'s needs: walk files → count LOC → merge with churn → classify and output.
- **Worker pool with oversubscription.** `hc` will be I/O bound on both file reads (complexity) and git log parsing (churn). `NumCPU * 4` workers is a proven default.
- **Reusable buffers.** If `hc` reads files directly for LOC counting, per-worker reusable buffers avoid allocation pressure.
- **GC tuning.** For repositories with tens of thousands of files, disabling GC during the scan phase is a cheap win.

### What to skip

- **Trie-based token matching.** `hc`'s default complexity metric is LOC, which requires only line counting — no language-aware parsing. If AST-based complexity is added later, language-specific tooling (external or built-in) would be more appropriate than a custom trie.
- **Bloom filter for extensions.** Unnecessary unless `hc` needs to classify files by language. A simple map suffices for any ignore-pattern matching.
- **Lazy language loading.** Not applicable — `hc` has no per-language feature structures.

### Integration option

Rather than reimplementing LOC counting, `hc` can optionally shell out to `scc`:

```
scc --format json --no-cocomo --no-complexity [path]
```

This provides per-file LOC, comment lines, blank lines, and complexity for all supported languages. The JSON output is straightforward to parse. This would be exposed via a `--complexity-cmd` flag, with built-in LOC counting as the zero-dependency default.
