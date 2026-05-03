# PR File-Level Hotspot Comments — Proposal

## Context

The existing `hotspots.yml` workflow runs `hc analyze` on every PR and posts
one repo-wide markdown report as a sticky PR comment. That's good for
overall awareness but easy to skim past — a reviewer or author has to
remember to open the report, find the file they touched, and connect the
quadrant guidance back to their diff.

This proposal adds a second, complementary action: **per-file inline review
comments** on PRs, scoped to files that fall into a *high-signal* quadrant
*and* were touched by the PR. The goal is to surface quadrant-specific
guidance at the moment and place it's actionable — next to the change.

---

## Which Quadrants to Flag

The four quadrants do not carry equal value as inline PR comments:

| Quadrant     | Flag? | Reasoning |
|--------------|-------|-----------|
| HotCritical  | Yes   | Already on everyone's radar via the report, but worth a reviewer-scrutiny nudge. Secondary priority. |
| **ColdComplex** | **Yes** | **Primary target.** Surprise factor: devs don't expect a "cold" file to be risky. The report's guidance ("add tests/docs first, do not refactor proactively") is exactly what an inline comment can deliver in context. |
| HotSimple    | No    | Low risk; not actionable. |
| ColdSimple   | No    | No signal. |

Each quadrant gets a tailored message body (different advice, not a generic
"this is a hotspot" template).

---

## Base-Branch Analysis Contract

The inline comment should describe the file's risk **before** the PR changed
it. That keeps the signal historical and review-oriented:

- Run `hc analyze` against the PR's base commit/tree.
- Diff the base commit against the PR head to find touched files.
- Post comments on the PR head commit so GitHub can anchor them in the Files
  Changed tab.

This means the comment can safely say:

> On the base branch, this file is classified as Cold Complex.

It should not say:

> This PR made this file Cold Complex.

That stronger statement belongs to a future diff-aware/quadrant-transition
mode.

---

## Mechanics (v1 — no `hc` changes)

The action is a thin shell pipeline on top of existing tools:

1. **Capture refs:**

   ```bash
   BASE_SHA="${{ github.event.pull_request.base.sha }}"
   HEAD_SHA="${{ github.event.pull_request.head.sha }}"
   ```

2. **Determine PR diff:**

   ```bash
   git diff --name-only --diff-filter=ACM "$BASE_SHA...$HEAD_SHA" > changed.txt
   ```

   Deleted files are excluded because they cannot receive file-level review
   comments and will not appear in base-branch analysis output. Rename handling
   is left as an open question for v1.

3. **Run analysis against the base tree:**

   ```bash
   git worktree add --detach ../hc-base "$BASE_SHA"
   ./hc analyze --json ../hc-base > hotspots.json
   ```

4. **Filter** to the intersection of (changed files) × (`cold-complex` ∪
   `hot-critical`) using the pure-Python helper script.
5. **Post file-level review comments** via
   `gh api repos/{owner}/{repo}/pulls/{n}/comments` with
   `subject_type=file` (anchors the comment to the file in the Files
   Changed tab without requiring a specific line number):
   - `path` = changed file path (same path in base and head for v1, because
     renames are skipped)
   - `commit_id` = `HEAD_SHA`
   - `subject_type` = `"file"`
   - `body` = quadrant-specific message (markdown, includes a footer tag
     like `<!-- hc-pr-comment:{path} -->` for idempotency)

   > **TODO (v2):** target a specific diff-hunk line
   > (`subject_type=line`) for more precise placement. Requires parsing
   > `git diff` hunks, handling pure-deletion changes, renames, and
   > files deleted by the PR. See *Future Work* below.

No new `hc` functionality is required — `git worktree`, `git diff --name-only`,
the Python helper, and `gh api` cover it. Keep the door open for a `--paths`
filter later (see *Future Work*) if the workflow gets unwieldy.

---

## File Layout

```
.github/workflows/
  pr-file-comments.yml             # new workflow, parallel to hotspots.yml
scripts/
  post-pr-file-comments.sh         # shell glue: git diff, templates, gh api calls
  filter-pr-hotspots.py            # pure Python JSON/list comparison
  templates/
    hotcritical.md                 # message body for HotCritical
    coldcomplex.md                 # message body for ColdComplex
```

Templates are plain markdown files so the wording is reviewable in diffs
and editable without touching shell.

---

## Workflow Sketch

```yaml
name: pr-file-comments

on:
  pull_request:

permissions:
  contents: read
  pull-requests: write

jobs:
  comments:
    runs-on: ubuntu-latest
    env:
      BASE_SHA: ${{ github.event.pull_request.base.sha }}
      HEAD_SHA: ${{ github.event.pull_request.head.sha }}
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
      - run: make build
      - name: Prepare base worktree
        run: git worktree add --detach ../hc-base "$BASE_SHA"
      - name: Analyze base branch
        run: ./hc analyze --json ../hc-base > hotspots.json
      - name: Post per-file review comments
        env:
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          GITHUB_REPOSITORY: ${{ github.repository }}
          PR_NUMBER: ${{ github.event.pull_request.number }}
        run: ./scripts/post-pr-file-comments.sh hotspots.json
```

---

## Idempotency

PRs get re-pushed; we don't want N copies of the same comment. Each
comment body ends with a hidden tag `<!-- hc-pr-comment:{path} -->`.
Before posting, list existing PR review comments via `gh api`. If a matching
tag is already present, update that comment's body instead of creating a new
one.

Keying on path (not quadrant) means a file that moves quadrants between
pushes won't produce a duplicate. Because v1 analysis is anchored to the base
commit, quadrant churn should only happen when the PR base changes.

---

## Open Questions After Base-Branch Alignment

- **Threshold tuning:** ColdComplex is currently defined by median split.
  On a small repo this can flag a lot of files. Consider a minimum
  complexity floor (e.g. only flag if `complexity > 200` *and* ColdComplex)
  to keep noise down. Worth measuring on this repo before tuning.
- **Comment volume cap:** even with quadrant filtering, a large PR could touch
  many HotCritical/ColdComplex files. Consider a hard cap (e.g. first 5 files
  by score) plus a summary note in the sticky report.
- **Rename handling:** the simple `--name-only --diff-filter=ACM` v1 skips
  renames. Supporting renames cleanly means reading `git diff --name-status -M`,
  looking up hotspot data by the base path, and posting the comment to the PR
  head path.
- **Stale comment cleanup:** updating matching comments avoids duplicates, but
  comments for files that stop matching after a rebase or force-push may remain.
  Decide whether v1 should delete obsolete `hc-pr-comment` review comments or
  leave them alone.
- **Coexistence with `hotspots.yml`:** the two workflows are complementary
  — keep both running. The repo-wide report stays as the "scorecard"; the
  inline comments are the "nudge." Open question: should the repo-wide report
  also analyze the base branch for conceptual consistency, or should it keep
  reporting the PR merge result?
- **HotCritical fatigue:** if reviewers find HotCritical comments
  redundant with the report, drop to ColdComplex-only. Easy revert.
- **Tool version in this repo:** the workflow builds `hc` from the checked-out
  workflow tree, then points that binary at the base worktree. For PRs that
  modify `hc`'s own analysis behavior, decide whether comments should use the
  candidate implementation, the base-branch implementation, or a released
  binary.

---

## Known Limitations (accepted for v1)

- **Fork PRs:** `GITHUB_TOKEN` is read-only on PRs from forks, so the
  workflow can run analysis but not post inline comments. Same gap as
  the existing `hotspots.yml`. Accepted as-is — revisit if external
  contributors become a meaningful share of PRs.
- **PR-introduced new files are absent from base analysis:** net-new files in
  a PR won't get hotspot comments because they do not exist in the base
  worktree. That is intentional for v1: the comment is about historical risk,
  not the risk introduced by new code.
- **PR changes do not affect v1 classifications:** because analysis runs
  against `BASE_SHA`, PR-added files, PR churn, and PR complexity changes cannot
  shift median-split thresholds or quadrant assignments.

---

## Future Work (defer until v1 ships)

- **Line-level comment targeting** — anchor each comment to a specific
  diff hunk line (`subject_type=line`) instead of the whole file.
  Better signal-to-noise when only a small region is touched. Requires
  parsing `git diff` hunks (not just `--name-only`), handling
  pure-deletion changes, renames, and files deleted by the PR. Defer
  until the file-level v1 is dogfooded.
- **`hc analyze --paths file1,file2`** — let the workflow ask `hc` to score
  only the changed files that already exist on the base branch. Cleaner than
  filtering after the fact and faster on large repos.
- **Diff-aware mode (`hc analyze --since-ref origin/main`)** — score files
  by *the change this PR introduces* rather than current state. Lets the
  comment say "this PR pushed `foo.go` *into* HotCritical" instead of
  "on the base branch, `foo.go` was already HotCritical." Strongest signal,
  but a real feature — defer until the simple version proves out.
- **Quadrant transitions** — flag files that *moved* quadrants on this PR
  (e.g. ColdSimple → HotCritical). Requires diff-aware mode.

---

## Out of Scope

- No new `hc` subcommand or flags in v1. All filtering lives in the
  workflow.
- No replacement of the existing `hotspots.yml` report comment.
- No per-author or per-team routing of comments.

---

## Rollout Order

1. Author message templates (`scripts/templates/*.md`) — wording is the
   bulk of the value, get it right first.
2. Write `scripts/filter-pr-hotspots.py`: parse hotspot JSON and intersect it
   with the changed-file list.
3. Write `scripts/post-pr-file-comments.sh`: run `git diff --name-only`, render
   templates, and post file-level review comments via `gh api` with idempotency
   tags.
4. Add `.github/workflows/pr-file-comments.yml`.
5. Dogfood on this repo for a few PRs; tune thresholds and decide
   HotCritical in/out based on reviewer feedback.
6. README section documenting the action and how to opt in.
