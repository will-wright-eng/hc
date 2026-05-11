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
```

Input contract matches `hc md report` (proposal 006): analyze JSON via `-i` or stdin. (`hc md ignore` deliberately doesn't take JSON — see 006 §"Input contract" — so this is the second JSON-consuming `md` subcommand, not a universal rule.)

`hc md comment` is designed against the default `hc analyze --json` shape, which has decay enabled and `weighted_commits` populated. Consumers running `--no-decay` upstream are out of scope for v1.

The quadrant priority sort (hot-critical first, then cold-complex, then by `weighted_commits` desc) and the idempotency-tag generation (`<!-- hc-pr-comment:{path} -->`) move from `scripts/post-pr-file-comments.sh` into the Go renderer. This matches the canonical quadrant order used everywhere else in `hc` (CLAUDE.md: HotCritical → HotSimple → ColdComplex → ColdSimple); the current shell's cold-complex-first ordering is treated as an accidental drift, not a deliberate inversion.

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
  ignore.md
  comment/
    coldcomplex.md
    hotcritical.md
```

(No `report.md` — report rendering stays inline, per 006 §"File Layout".)

The `<!-- hc-stats -->` substitution that the current awk step does moves into the Go renderer. The placeholder is replaced with a markdown table rendered **dynamically from the analyze JSON entry** — no hardcoded field list. Whatever keys the entry carries become rows, in JSON-field order; absent fields are skipped. This keeps the renderer in lockstep with `hc analyze --json` as that schema evolves.

Templates stay plain markdown so wording is reviewable in diffs (proposal 002 §"File Layout").

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

- **Tag placement.** Decided: `tag` is exposed at the top level of each batch entry *and* embedded at the end of `body` (matching the current shell, which appends the tag after the template). Top-level is for the shell loop; the trailing embed is what existing find-by-tag (`contains(tag)`) looks for, so no posting-side change is required.

---

## Out of Scope

- The posting loop (stays in shell, see above).
- Line-level (`subject_type=line`) targeting — proposal 002 §"Future Work."
- New quadrants or template types — mechanical once `hc md comment` exists.
- A `--limit` flag for comment-volume capping. Default unlimited; if the classifier turns out noisy, add the flag at that point rather than pre-engineering for it.
- A `--min-complexity` floor. Same logic as `--limit`: the median-split classifier already filters out low-complexity files into the `*-simple` quadrants, so a separate floor is redundant until proven otherwise. Add it if dogfooding shows the quadrant filter isn't enough.
- `--no-decay` upstream support. The renderer assumes the default analyze JSON shape (decay on, `weighted_commits` present); users running `hc analyze --no-decay` and piping into `hc md comment` aren't a v1 target.

---

## Rollout

1. Proposal 006 is landed — `hc md` namespace and `internal/md/templates/` exist. Start here.
2. Implement `hc md comment` consuming analyze JSON; emit NDJSON.
3. Move templates from `scripts/templates/` into `internal/md/templates/comment/`. Delete the old dir.
4. Rewrite `scripts/post-pr-file-comments.sh` to consume NDJSON from `hc md comment`. Drop the jq filter and awk renderer.
5. Update the `pr-file-comments` Make target and the GitHub Actions workflow to invoke `hc md comment` directly.
6. Dogfood on this repo. If comment volume is too noisy, revisit `--limit` and `--min-complexity` (both currently out of scope).
