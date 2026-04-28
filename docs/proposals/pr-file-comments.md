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

## Mechanics (v1 — no `hc` changes)

The action is a thin shell pipeline on top of existing tools:

1. **Determine PR diff:**

   ```bash
   git diff --name-only "origin/${{ github.base_ref }}...HEAD" > changed.txt
   ```

2. **Run analysis:**

   ```bash
   ./hc analyze --json > hotspots.json
   ```

3. **Filter** to the intersection of (changed files) × (ColdComplex ∪ HotCritical)
   using `jq`.
4. **Post inline review comments** via `gh api repos/{owner}/{repo}/pulls/{n}/comments`:
   - `path` = file path
   - `line` = first changed line from the patch (must exist in the diff,
     otherwise the API rejects the comment)
   - `body` = quadrant-specific message (markdown, includes a footer tag
     like `<!-- hc-pr-comment:ColdComplex -->` for idempotency)

No new `hc` functionality is required — `git diff --name-only`, `jq`, and
`gh api` cover it. Keep the door open for a `--paths` filter later (see
*Future Work*) if the workflow gets unwieldy.

---

## File Layout

```
.github/workflows/
  pr-file-comments.yml             # new workflow, parallel to hotspots.yml
scripts/
  post-pr-file-comments.sh         # shell glue: filter JSON, find diff lines, post via gh api
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
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
      - run: make build
      - run: ./hc analyze --json > hotspots.json
      - name: Post per-file review comments
        env:
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          GITHUB_REPOSITORY: ${{ github.repository }}
          PR_NUMBER: ${{ github.event.pull_request.number }}
          BASE_REF: ${{ github.base_ref }}
        run: ./scripts/post-pr-file-comments.sh hotspots.json
```

---

## Idempotency

PRs get re-pushed; we don't want N copies of the same comment. Two options:

1. **Tag-and-skip** (simpler): each comment body ends with a hidden tag
   `<!-- hc-pr-comment:{quadrant}:{path} -->`. Before posting, list existing
   PR review comments via `gh api` and skip if a matching tag is already
   present.
2. **Tag-and-replace**: same tag, but delete the old comment and post fresh.
   Useful if quadrant changes between pushes.

Start with option 1 (skip). Comments don't need to update on re-pushes —
the guidance is the same.

---

## Open Questions

- **Line targeting:** first changed line is the simplest choice but may
  feel arbitrary. Alternatives: the file's "hotspot region" (most-churned
  hunk) or a file-level comment via the issues API instead of the
  pulls/comments API. First-changed-line is the v1 default.
- **Threshold tuning:** ColdComplex is currently defined by median split.
  On a small repo this can flag a lot of files. Consider a minimum
  complexity floor (e.g. only flag if `lines > 200` *and* ColdComplex) to
  keep noise down. Worth measuring on this repo before tuning.
- **Coexistence with `hotspots.yml`:** the two workflows are complementary
  — keep both running. The repo-wide report stays as the "scorecard"; the
  inline comments are the "nudge."
- **HotCritical fatigue:** if reviewers find HotCritical comments
  redundant with the report, drop to ColdComplex-only. Easy revert.

---

## Future Work (defer until v1 ships)

- **`hc analyze --paths file1,file2`** — let the workflow ask `hc` to score
  only the changed files. Cleaner than `jq`-filtering after the fact and
  faster on large repos.
- **Diff-aware mode (`hc analyze --since-ref origin/main`)** — score files
  by *the change this PR introduces* rather than current state. Lets the
  comment say "this PR pushed `foo.go` *into* HotCritical" instead of
  "`foo.go` is hot and you touched it." Strongest signal, but a real
  feature — defer until the simple version proves out.
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
2. Write `scripts/post-pr-file-comments.sh`: parse JSON, intersect with
   `git diff --name-only`, resolve a target line per file, post via
   `gh api` with idempotency tag.
3. Add `.github/workflows/pr-file-comments.yml`.
4. Dogfood on this repo for a few PRs; tune thresholds and decide
   HotCritical in/out based on reviewer feedback.
5. README section documenting the action and how to opt in.
