# PR Hotspot Annotations via GitHub Actions Workflow Commands

## Overview

Annotate a pull request's changed files with hotspot findings the way `zizmor`
and `oxlint` do — by emitting **GitHub Actions workflow commands**
(`::warning file=…,line=…,title=…::message`) to stdout, which the runner turns
into check-run annotations shown on the **Files changed** tab and in the
**Checks** tab.

This refactors the existing PR path — [`pr-file-comments.yml`](../../.github/workflows/pr-file-comments.yml)
and `hc md comment` — which today posts `subject_type: file` **PR review
comments** through the REST API. Moving to workflow-command annotations
**removes** the API posting, the `GITHUB_TOKEN` / `pull-requests: write`
permission, the find-or-create dedup, and `scripts/post-pr-file-comments.sh`
entirely.

## Background — current state

`pr-file-comments.yml` runs on `pull_request`:

1. checkout (full history) → build `hc` → add a base-branch worktree at `../hc-base`.
2. `make pr-changed-files` — `git diff --name-only --diff-filter=ACM BASE...HEAD > changed.txt`.
3. `make pr-hotspots-json` — `hc analyze --json --files-from changed.txt ../hc-base`.
4. `make pr-file-comments` — `hc md comment --input hotspots.json | post-pr-file-comments.sh`.

`hc md comment` emits NDJSON `{path, quadrant, tag, body}`; `body` is a markdown
template (`<details>` + a stats table); `tag` is a sentinel.
`post-pr-file-comments.sh` does a find-or-create against `pulls/{n}/comments`
with `subject_type=file`, requiring `GH_TOKEN` and `pull-requests: write`.

## The mechanism — and the one hard constraint

Workflow commands are printed to a step's stdout and parsed by the runner; **no
API call, no token, no permission** (they work even with a read-only token on
fork PRs). But:

- **All** annotations appear in the Checks tab / run summary.
- An annotation renders **inline on "Files changed" only when its line is inside
  the PR's diff hunks.** An annotation on an unchanged line (or a line-less /
  `line: 1` annotation when line 1 wasn't changed) is hidden from the diff view
  and shows only in the Checks tab.
- There is **no** line-less workflow-command or Checks-API annotation that
  displays inline regardless of line. The only line-less, file-level inline
  surface is the `subject_type: file` review comment this refactor moves away
  from.
- **Caps:** 10 each of `notice`/`warning`/`error` **per step**, 50 per job;
  excess are dropped from display (still in the raw logs).

`zizmor`/`oxlint` annotations appear inline because their findings sit on the
lines you edited. hc findings are file-level, so to render inline they must be
anchored to *some* changed line of the file (see Line anchoring).

## Design

Replace `hc md comment` with a top-level **`hc annotate`** whose **only** output
is GitHub workflow-command annotations — one per changed hotspot file. The
previous NDJSON `{path, quadrant, tag, body}` form, its markdown templates, and
the find-or-create `tag` sentinel are removed: nothing else consumes them, so
this needs no backwards-compatibility shim and no `--format` flag.

```text
::warning file=internal/git/git.go,line=12,title=hc: Hot/Critical hotspot::internal/git/git.go was already a Hot/Critical hotspot on the base branch (high churn × high complexity). Keep the diff focused and add tests before changing it.
```

- **Level:** `hot-critical` → `warning`, `cold-complex` → `notice` (advisory;
  these never fail the job). Default quadrant set and ordering carry over from
  the old `hc md comment`.
- **title:** short, e.g. `hc: Hot/Critical hotspot`.
- **message:** concise **plain text** — annotations do not render markdown or
  `<details>`, so the current template bodies collapse to a one-paragraph
  message (multi-line allowed via the `%0A` escape). Sourced from the same
  per-quadrant wording, trimmed; the stats can be appended inline.
- **Escaping:** the emitter must escape `%`→`%25`, `\r`→`%0D`, `\n`→`%0A` in the
  message, and additionally `:`→`%3A`, `,`→`%2C` inside the `file`/`title`
  property values, or the annotation breaks/truncates. A path or message
  containing `,` or `:` (common) makes this mandatory.

> **Naming (resolved):** shipped as a top-level **`hc annotate`** — not under
> `md`, since the output is not markdown — with the renderer in a new
> `internal/annotate` package. The old `hc md comment` name and the `md` grouping
> are gone.

### Line anchoring (the key decision)

To render inline, the annotation needs a line in the diff. hc does not — and by
design should not — parse PR diffs, so the **anchor line is supplied by CI**,
where the diff is already computed:

- `hc annotate --anchor-lines FILE` reads `path<TAB>line`; a path absent from
  the map (or no `--anchor-lines`) falls back to `line=1`.
- The workflow computes anchors from the same `git diff` it already runs:
  `git diff --unified=0 --diff-filter=ACM BASE...HEAD` → first added/changed
  line per file.

"Don't care which line" is satisfied: the first changed line is an arbitrary,
valid anchor and the message remains about the whole file.

**Simpler fallback:** skip `--anchor-lines` and emit `line=1`. Annotations then
appear reliably in the Checks tab/run summary, and inline only when line 1 was
changed. Recommended only if the Checks-tab surface is acceptable; otherwise use
anchors to get the zizmor/oxlint inline experience.

## Refactor specifics

### hc

- `internal/annotate` (new package): the annotation renderer (`Render` /
  `Options`). Reuses envelope parsing, a bare-array guard, the quadrant filter,
  and the rank ordering. The old `internal/md/comment.go` is **deleted** along
  with the `CommentEntry` shape, the `tag` sentinel, the `<!-- hc-stats -->`
  markdown stats table, and the `internal/md/templates/comment/*.md` templates.
- `cmd/hc/main.go`: top-level `annotate` command with `--input` / `--quadrant` /
  `--anchor-lines FILE`. No `--output` (annotations must reach the runner on
  stdout) and no `--format` flag.

### Workflow (`pr-file-comments.yml`)

- Replace the token-bearing "Post per-file hotspot comments" step with a step
  that runs `hc annotate` — the runner ingests the stdout annotations
  directly.
- Drop `pull-requests: write` → `contents: read` only; drop the `GH_TOKEN` /
  `PR_NUMBER` / `GITHUB_REPOSITORY` env.

### Makefile

- `pr-changed-files`: also emit `anchors.txt` (`path<TAB>first-changed-line`).
- Replace `pr-file-comments` with `pr-annotations`:
  `hc annotate --input hotspots.json --anchor-lines anchors.txt`.
- `post-pr-file-comments.sh` is deleted along with the review-comment path.

## Trade-offs vs. the removed review-comment path

| | Workflow-command annotations (new) | `subject_type: file` review comments (removed) |
| --- | --- | --- |
| Token / permission | none | `GITHUB_TOKEN` + `pull-requests: write` |
| Posting | runner reads stdout | REST find-or-create (PATCH/POST) |
| Persistence | ephemeral, regenerated each run (no stale cleanup, no dedup) | persistent, threaded, updated in place |
| Body | plain text, one message | markdown (`<details>`, tables) |
| Inline display | needs a diff-line anchor | true file-level, no line |
| Limit | 10 / level / step | none meaningful |

The zizmor/oxlint annotation UX is the command's sole purpose, so the
review-comment approach is removed outright rather than kept behind a flag. The
right column records what is dropped and why annotations win for this use case.
The main thing lost is persistence/threading; for a churn-vs-complexity nudge on
a PR, regenerating fresh annotations each run (no stale-comment cleanup) is the
better trade.

## Non-goals

- A truly line-less inline annotation — does not exist for check annotations.
- Semantic line targeting — the anchor is arbitrary; hc is file-level by intent.
- Annotating unchanged hotspot files — only the PR's changed files, which is
  also the only place inline annotations can appear.
- Persistent/threaded comments — the `subject_type: file` review-comment path is
  removed; if it is ever wanted again it would be a separate command, not a flag
  on this one.

## Risks

- **10-per-level-per-step cap:** a PR changing >10 hot-critical files shows only
  10 warnings inline (rest in logs). Low likelihood for changed-file scope; if
  it matters, split levels across steps or aggregate. Note it in docs.
- **Line-start parsing:** the `::…::` marker must begin the output line; a tool
  or `make` prefix silently suppresses the annotation. The step must run `hc`
  so its stdout is unprefixed.
- **Escaping bugs** would break or truncate annotations — covered by tests.

## Test coverage to add

- Formatter golden: envelope → exact `::warning::` / `::notice::` lines, in rank
  order, with correct level mapping.
- Escaping: paths/messages containing `:`, `,`, `%`, and newlines; unicode.
- Anchor lines: present (uses the line), path missing from map (falls back to
  `line=1`), no `--anchor-lines` flag.
- Empty / filtered-to-empty input → no output.
- `--quadrant` override and default set, matching the prior `hc md comment`.
