# `hc analyze --files-from` — Projection Filter for Integrations

## Context

The PR file-comment workflow (`proposal 002`) follows this pipeline:

1. `hc analyze --json` on the base tree → full hotspot list
2. `git diff --name-only` → list of files touched by the PR
3. `scripts/filter-pr-hotspots.py` → intersect (1) ∩ (2), keep only `hot-critical` / `cold-complex`
4. `scripts/post-pr-file-comments.sh` → render templates and post

Step 3 exists only because `hc analyze` has no way to say "score the whole repo,
but only return rows for this subset of files." Any integration that wants
PR-scoped output — GitHub, GitLab, a local pre-push hook, a code-review bot —
has to ship its own intersect helper.

This proposal adds a `--files-from` flag to `hc analyze` so step 3 collapses
into step 1, and every integration's glue gets shorter. The name matches
`tar --files-from` and `rsync --files-from`, which do exactly this operation —
read a newline-delimited list of paths and operate only on those entries
(see `docs/design/003-cli-ergonomics.md` for the convention-over-invention
rationale that also drove `--exclude` and `-o, --output`).

---

## Strategy: projection, not scan

The non-obvious part of path filtering is **what gets filtered, and when**.

`hc` classifies files by median-split: the median of indent-sum and the median
of weighted commits across *all* files in the analyzed tree are the thresholds.
If the filter short-circuits the file walk, the medians are computed from a
handful of changed files instead of the full corpus, and classification
becomes meaningless (every changed file is "above the median of itself").

So `--files-from` must be a **projection filter**, not a scan filter:

```
walk full tree → compute churn + complexity for all files
              → compute medians across all files
              → classify every file
              → emit rows only for paths in --files-from
```

The scan, the medians, and the classification are unchanged. `--files-from`
only affects what appears in the output.

This is the same shape as filtering after the fact (which is what the Python
helper does today), just moved inside the binary so callers don't need their
own JSON post-processor.

---

## CLI Surface

```bash
hc analyze --files-from changed.txt [path]
hc analyze --files-from -           [path]   # read from stdin
```

- `--files-from` takes a newline-delimited file of repo-relative paths.
  Mirrors `git diff --name-only` output; the file-comments workflow can pipe
  its `changed.txt` directly.
- `-` reads from stdin, enabling `git diff --name-only ... | hc analyze --files-from -`.
- Paths not present in the analyzed tree are silently dropped (e.g. deleted
  files from `git diff`, files excluded by `.hcignore`). A `--strict-files`
  flag could be added later if a caller wants errors instead of silent drops.
- No flag ⇒ existing behavior (emit all rows).
- No short flag. `tar` and `rsync` don't give `--files-from` one, and nothing
  in `hc`'s use cases needs it.

A comma-separated `--files` flag is intentionally **not** part of v1. Every
real caller already has the path list in a file (or piped from `git diff`); a
second surface for the same input is overengineering until something needs it.

Output format is unchanged: same columns, same JSON schema, same quadrant
priority sort. Only the row set shrinks.

---

## What this deletes

- `scripts/filter-pr-hotspots.py` (~125 lines, plus the `python3` dependency
  in CI).
- The "Match changed hotspots" step in `.github/workflows/pr-file-comments.yml`.
- The corresponding `make pr-hotspot-matches` target.

The PR workflow's analysis step becomes:

```bash
git diff --name-only --diff-filter=ACM "$BASE_SHA...$HEAD_SHA" > changed.txt
./hc analyze --json --files-from changed.txt ../hc-base > hotspot-matches.json
```

`post-pr-file-comments.sh` consumes JSON directly (a small shell change — it
already shells out to `jq`-equivalents) instead of TSV produced by Python.

---

## Quadrant Filtering

The Python helper also filters to `hot-critical` ∪ `cold-complex`. That logic
does **not** move into `hc analyze` — quadrant selection is the *integration's*
opinion, not the analyzer's. Two options for the workflow:

1. **Filter in shell:** `jq '.[] | select(.quadrant=="hot-critical" or .quadrant=="cold-complex")'`.
   Simple, no `hc` changes.
2. **Add `--quadrants hot-critical,cold-complex` to `hc analyze`:** symmetric
   with `--files-from`. Marginally cleaner, but ties `hc`'s CLI to a
   PR-comment concept. Defer until a second integration wants the same filter.

Recommend option 1 for v1.

---

## Implementation Notes

- Flag wiring lives in `cmd/hc/main.go` alongside the other `analyze` flags.
- Path normalization: accept both `./foo.go` and `foo.go`; normalize to the
  same form `internal/git` and `internal/complexity` use (slash-separated,
  no leading `./`).
- Filtering point: in `cmd/hc/main.go` after `analysis.Classify` returns
  `[]FileScore`, before handing off to `internal/output`. This keeps
  `internal/analysis` free of caller-specific concerns and means medians /
  thresholds / sort order are all computed on the full set.
- Empty filter set after path normalization (all paths were dropped) ⇒ emit
  empty result with exit code 0. A non-zero exit would force every caller to
  special-case "PR touched no analyzable files."
- Blank lines in the input file are ignored. Lines are trimmed of trailing
  whitespace; no other parsing (no comments, no globs) — keep the contract
  small so `git diff --name-only` output works as-is.

---

## Test Plan

The "projection vs scan" distinction is the only real correctness risk. Tests
that pin it down:

1. **Medians invariant:** for a fixture with N files where the median commit
   count is M, `hc analyze --files-from <single-path file>` reports the same
   quadrant for that file as `hc analyze | grep <path>`. Run both on the same
   fixture and diff.
2. **Output row count:** a `--files-from` file listing three of ten tracked
   files emits exactly three rows (or fewer if some are ignored/missing).
3. **Missing paths:** `--files-from` listing only `nonexistent.go` on a
   non-empty tree exits 0 with empty output, not an error.
4. **Stdin:** `--files-from -` matches `--files-from <file>` for the same
   content.
5. **Blank lines:** a file containing blank lines between paths is parsed the
   same as one without them.
6. **JSON shape:** `--files-from` does not change the JSON schema — same
   keys, same types — only the array length.

---

## Out of Scope

- No `--files` comma-list flag. Every realistic caller already has the path
  list in a file or on stdin; adding a second input surface now would be
  overengineering. Revisit if a use case appears that genuinely can't use a
  file or pipe.
- No `--base-ref` flag. Analyzing a non-checked-out ref means rewriting the
  complexity scanner to read git blobs instead of the filesystem (or shelling
  out to `git archive` inside `hc`, duplicating what `git worktree` already
  does cleanly). The external `git worktree add --detach` is two portable
  shell lines and works on every forge. The cost/benefit doesn't justify
  pulling it into `hc` for v1.
- No `--since-ref` / diff-aware mode. That's a separate, larger feature
  (see proposal 002 *Future Work*).
- No `--quadrants` flag (see above — defer until a second integration asks).

---

## Rollout Order

1. Add `--files-from` flag + path normalization in `cmd/hc/main.go`.
2. Add the projection filter in the analyze action, after classification.
3. Tests per the plan above.
4. Update `scripts/post-pr-file-comments.sh` to consume JSON directly
   (replace the TSV reader with a `jq` loop, or keep a tiny shell adapter).
5. Update `.github/workflows/pr-file-comments.yml`: drop the Python step,
   drop the `python3` install if present, pipe `changed.txt` directly into
   `hc analyze --files-from`.
6. Delete `scripts/filter-pr-hotspots.py` and any `make pr-hotspot-matches`
   target.
7. README note in the `analyze` section documenting `--files-from` as the
   integration hook for PR / MR workflows.
