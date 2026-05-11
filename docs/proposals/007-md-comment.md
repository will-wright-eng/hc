# `hc md comment` — Per-File PR Hotspot Comment Rendering

## Context

Proposal 002 introduced per-file PR review comments for changed files that fall into the HotCritical or ColdComplex quadrant on the base branch. The current implementation (`scripts/post-pr-file-comments.sh`) does three things in shell:

1. **Filters** the analyze JSON to hot-critical / cold-complex entries (jq).
2. **Renders** comment bodies from markdown templates in `scripts/templates/` (awk substitution into `<!-- hc-stats -->`).
3. **Posts** the comments via `gh api`, with idempotency keying on hidden tags.

Proposal 006 establishes `hc md` as the single home for markdown rendering. This proposal moves steps 1 and 2 into `hc md comment`. Step 3 (posting) stays in shell, because `gh api` glue is GitHub-specific and not part of `hc`'s purpose.

The shell pipeline shrinks from "jq filter → awk render → gh api loop" to "hc md comment | gh api loop."

---

## Surface

```
hc md comment [flags]
```

Per the ergonomics doc (#5: single-word leaves), the verb is `comment`, not `pr-comment` or `pr-comments`. Context disambiguates — `hc md` already implies markdown output, and a `comment` under it is unambiguously a PR comment body.

### Flags

```
-i, --input FILE          JSON input (default: stdin) — output of `hc analyze --json`
-o, --output FILE         Write the batch to FILE (default: stdout)
    --format FORMAT       ndjson|json|tar (default: ndjson) — see "Batch Output" below
    --quadrant QUADRANT   Repeatable. Restrict to one or more quadrants
                          (default: hot-critical,cold-complex)
    --limit N             Cap output to top N entries by priority
    --min-complexity N    Floor — only emit entries with complexity >= N
                          (addresses proposal 002 "Threshold tuning" open question)
```

Input contract matches all other `md` subcommands (proposal 006): analyze JSON via `-i` or stdin.

The quadrant priority sort (cold-complex first, then hot-critical, then by `weighted_commits` desc) and the idempotency-tag generation (`<!-- hc-pr-comment:{path} -->`) move from `scripts/post-pr-file-comments.sh` into the Go renderer.

---

## Output

Each entry in the batch is one rendered comment, ready to post. The shell glue iterates the batch and calls `gh api` per entry.

The unit structure:

```json
{
  "path": "internal/analysis/analyze.go",
  "quadrant": "cold-complex",
  "tag": "<!-- hc-pr-comment:internal/analysis/analyze.go -->",
  "body": "...rendered markdown including the tag...\n"
}
```

`path` and `tag` are exposed at the top level so the shell loop doesn't have to grep them out of `body`.

### Batch format

`--format` selects one of:

| Format    | Shape | For |
|-----------|-------|-----|
| **`ndjson`** (default) | One JSON object per line. | Shell-friendly: `jq` per line, or `while read` loop. Streams. Matches `gh`, `kubectl --watch`. |
| `json`    | Top-level array of objects. | Easier to slurp from a language with a JSON parser. Doesn't stream. |
| `tar`     | tar archive on stdout; one `{tag}.md` per entry containing the rendered body, plus a sidecar `index.json` for metadata. | Useful if a consumer wants the bodies on disk for inspection or `gh pr review --body-file`. Niche. |

**Recommendation:** ship `ndjson` only in v1; leave `json` and `tar` as opt-in extensions. NDJSON is what the shell loop wants; the others are speculative. Add them when a real consumer asks.

Empty-batch behavior: emit zero lines and exit 0. The shell loop `while read line; do ...; done` handles empty input correctly; a sentinel would complicate every consumer.

---

## Templates

The two templates (`scripts/templates/coldcomplex.md`, `scripts/templates/hotcritical.md`) move into `internal/md/templates/comment/`:

```
internal/md/templates/
  report.md
  ignore.md
  comment/
    coldcomplex.md
    hotcritical.md
```

The `<!-- hc-stats -->` substitution that the current awk step does moves into the Go renderer. Templates stay plain markdown so wording is reviewable in diffs (proposal 002 §"File Layout").

`scripts/templates/` is removed.

---

## Shell Pipeline (After)

```bash
hc analyze --json --files-from changed.txt ../hc-base \
  | hc md comment \
  | while IFS= read -r entry; do
      path=$(jq -r .path <<< "$entry")
      body=$(jq -r .body <<< "$entry")
      tag=$(jq -r .tag <<< "$entry")
      # existing find-or-create-by-tag idempotency logic
      ...
    done
```

Compared to today: jq filtering of hotspot data — gone (in hc). awk template substitution — gone (in hc). The shell script keeps `gh api` calls and the find-by-tag idempotency loop. The remaining `jq` calls are reading hc's structured output, not analysis data — small and bounded.

---

## What Stays in Shell

- `gh api` POST and PATCH calls.
- "List existing PR comments, find by tag → PATCH if found, POST if not." GitHub-API plumbing, not markdown rendering.
- The `make pr-changed-files` target (`git diff --name-only`).

Pulling these into `hc` would mean adopting an HTTP client, an auth surface, and a release cadence tied to GitHub API changes — none of which serves the tool's purpose. If we ever want `hc` usable outside GitHub Actions (e.g. GitLab MR comments), revisit then.

---

## Open Questions

- **Tag placement.** The idempotency tag is currently appended to the body just before posting. With `hc` producing both `tag` and `body`, the tag is already embedded in `body` (so the find-by-tag search against existing comments works). Recommended: `tag` at top level *and* embedded in `body`. Top-level is for the shell loop; the embed is for find-by-tag.
- **`--min-complexity` default.** Proposal 002 left "threshold tuning" open. Default 0 keeps current behavior; surfacing the flag lets the workflow dial up if HotCritical/ColdComplex fires too often. Pick a default after dogfooding.
- **`--limit` default.** Proposal 002 raised "comment volume cap." Reasonable default: unlimited — don't paper over a noisy classifier. Workflow can pass `--limit 5` if needed.

---

## Out of Scope

- The posting loop (stays in shell, see above).
- Line-level (`subject_type=line`) targeting — proposal 002 §"Future Work."
- New quadrants or template types — mechanical once `hc md comment` exists.

---

## Rollout

1. Land proposal 006 first (`hc md` namespace + template directory).
2. Implement `hc md comment` consuming analyze JSON; emit NDJSON.
3. Move templates from `scripts/templates/` into `internal/md/templates/comment/`. Delete the old dir.
4. Rewrite `scripts/post-pr-file-comments.sh` to consume NDJSON from `hc md comment`. Drop the jq filter and awk renderer.
5. Update the `pr-file-comments` Make target and the GitHub Actions workflow to invoke `hc md comment` directly.
6. Dogfood on this repo. Decide `--min-complexity` and `--limit` defaults from observed comment volume.
