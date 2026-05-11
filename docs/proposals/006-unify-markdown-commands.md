# Unify `hc report` + `hc prompt` Under `hc md` — Proposal

## Context

`hc` has two top-level commands that both produce markdown from analysis-derived data, but they sit under different verbs and follow different shapes:

- `hc report` — consumes `hc analyze --json` and renders the hotspot markdown report. Supports `--output FILE`, `--upsert FILE`, `--collapsible`.
- `hc prompt ignore` — walks the repo (resolved via `git.RepoRoot` from cwd), builds a summary, and emits a markdown prompt asking an LLM to generate a `.hcignore`.

Both produce markdown. Both are rendering steps in a wider analyze → render pipeline. But they live under different verbs (`report` vs `prompt`), use different input contracts (JSON vs none), and split their templates across an inline literal in `internal/report/` and a file in `internal/prompt/templates/`.

A third markdown output is coming — per-file PR hotspot comments (see proposal 007). Without unification, we'd add a third top-level verb for "renders markdown from analysis output," which is exactly the noise `docs/design/003-cli-ergonomics.md` is trying to remove.

This proposal collapses the two verbs into a single `hc md` namespace with one rendering verb per output kind. Per the ergonomics doc, this is a hard cutover — no deprecation aliases.

---

## Name

Four candidates were evaluated:

| Name       | For | Against |
|------------|-----|---------|
| **`md`**   | Terse, matches `hc`'s own two-letter aesthetic. Unambiguous in this CLI — nothing else lives near markdown rendering. | Slightly cryptic for first-time readers. |
| `markdown` | Fully spelled out, no ambiguity. | Verbose. `hc markdown report` reads heavy. |
| `render`   | Verb form, suggests pipeline stage. | Over-broad — invites future "render JSON," "render CSV," but those belong to `analyze --output`. |
| `doc`      | Short. | Overloaded with "documentation"; misleads about scope. |

**Recommendation: `md`.** Matches the terse style of the rest of the CLI (`hc`, `-e`, `-n`). The ergonomics doc favors single-word subcommands; `md` is unambiguous because nothing else lives near it.

The internal package becomes `internal/md/`, with a single templates directory at `internal/md/templates/`.

---

## Surface

```
hc md report [flags]              # renders the hotspot markdown report
hc md ignore [flags]              # renders the .hcignore-generation LLM prompt
```

Proposal 007 adds `hc md comment` for per-file PR comments — out of scope here.

### Verb-per-output

Each renderer is a single leaf verb under `md`. Same pattern as `gh pr create`, `kubectl get pods`. No hyphenated leaves (ergonomics #5). Adding a new markdown output = adding a new leaf verb, not a flag.

### Input contract

Input shape is per-subcommand — the two renderers have different jobs and shouldn't be forced to share one.

- **`hc md report`** consumes analyze JSON via `-i FILE` or stdin. Pure renderer: given JSON X, expected markdown Y.
- **`hc md ignore`** takes no input file and no path arg. Its output is consumed by a coding agent to build a `.hcignore`, so it has no reason to align with the analyze JSON shape. It resolves the repo root from cwd via `git.RepoRoot` (the same helper analyze uses) and builds its summary directly — this is already how `prompt ignore` behaves today.

Pipeline:

```bash
hc analyze --json | hc md report
hc analyze --json > a.json && hc md report -i a.json
hc md ignore | claude -p > .hcignore
```

### Flags

Per-subcommand flags stay subcommand-specific — no forced uniformity. Concretely:

```
hc md report:
  -i, --input FILE         JSON input (default: stdin)
  -o, --output FILE        Write to FILE (default: stdout)
      --upsert FILE        Inject into existing markdown file
      --collapsible        Wrap quadrants in <details> blocks

hc md ignore:
  (no flags)
```

The two renderers share no flags. `md ignore` takes no input — it walks the repo itself, identically to today's `prompt ignore`.

---

## File Layout

```
internal/md/
  report.go              # was internal/report/report.go (rendering stays inline, not extracted)
  upsert.go              # was internal/report/upsert.go
  ignore.go              # was internal/prompt/prompt.go
  summary.go             # was internal/prompt/summary.go
  templates/
    ignore.md            # was internal/prompt/templates/ignore.md
```

`internal/prompt/` and `internal/report/` are deleted in the same change. Tests move alongside their files. `cmd/hc/main.go` wires `report` and `ignore` as subcommands of a new `md` command and deletes the old top-level `report` and `prompt` commands.

---

## Hard Cutover

No deprecation, no aliases. Running `hc report` or `hc prompt ignore` exits non-zero with an error pointing to the new command. Called out in the changelog of the release that ships this.

---

## Out of Scope

- The PR file-comment renderer (`hc md comment`) — see proposal 007.
- Any change to the analyze JSON shape. `md ignore` is intentionally not a renderer of analyze JSON; it remains a repo-walking command, just under a new verb.
- Multi-format rendering (HTML, plaintext, etc.). `md` is markdown-only by design.

---

## Rollout

1. Move `internal/report/` → `internal/md/` with no behavior changes; update imports in `cmd/hc/main.go`. Tests should pass unchanged.
2. Move `internal/prompt/` → `internal/md/`; consolidate templates into `internal/md/templates/`. Behavior is unchanged — `prompt ignore` already takes no flags and no path arg and resolves the repo root from cwd via `git.RepoRoot`.
3. Wire the new `hc md report` and `hc md ignore` subcommands. Delete `hc report` and `hc prompt`.
4. Update README, CLAUDE.md, and `make e2e` examples.
