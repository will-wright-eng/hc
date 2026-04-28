# CLI Ergonomics — Proposal

## Context

`hc` is a small CLI but its current surface accumulates several papercuts that, taken together, make the tool feel less natural than it should. The standard this proposal measures against is the body of conventions established by POSIX, GNU long options, and the everyday Unix toolchain — `git`, `grep`, `find`, `du`, `tar`, `rsync`, `kubectl`, `gh`, `jq`, `head`. Where this proposal recommends a change, it cites the convention it's anchored to.

The companion proposal `filter-command.md` argues for an `analyze | filter | report` three-stage pipeline. This proposal does **not** contest that architecture. The pipeline is fine; the issues here are at the flag and verb level, plus the friction of the common case.

## Premises

This proposal assumes:

1. **`hc` is pre-1.0.** Breaking changes are acceptable. No deprecation aliases, no "keep the old flag for one release" hedges — rename, remove, move on.
2. **`hc` is multi-verb with a default.** The model is `hc <verb>` (`analyze`, `filter`, `report`, `prompt`), and `hc [path]` is sugar for `hc analyze [path]`. Subcommands are the canonical surface; the bare form exists to cut friction on the common case.

---

## Issues, Ranked by Impact

### 1. No quick path for the common case

**Current.** The headline action requires `hc analyze`. The casual one-shot — "give me a markdown hotspot report for this repo" — looks like:

```bash
hc analyze -D -i --format json | hc report
```

That's three flags, two subcommands, and a pipe to do the thing the tool is named after.

**Convention.** Single-purpose tools (`du`, `wc`, `find`, `grep`) just run. Multi-verb tools (`git`, `docker`, `kubectl`) require a verb but have one canonical "do the obvious thing" path — `git status`, `docker ps`. `jq` pipes by design but ships sensible defaults so `jq .` works out of the box.

**Recommendation.** Make `analyze` the default action when no subcommand is given. `hc` and `hc .` behave like `hc analyze .`. Subcommands remain the canonical surface for explicit use and for the pipeline.

This preserves the staged architecture while lowering the activation energy on the common case.

---

### 2. Inconsistent short-flag casing

**Current.** `-d` is `--by-dir`, `-D` is `--decay`, `-i` is `--indentation`, `-I` is `--input` (on `report`). The case differences are arbitrary.

**Convention.** Case differences in short flags carry meaning: `tar -x` (extract) vs `-X` (exclude-from), `grep -i` (ignore case) vs `-I` (treat binary as nomatch), `ssh -L` vs `-l`. They signal a related-but-distinct option, not "I ran out of letters."

**Recommendation.**

- Drop `-D` for `--decay`. Decay should not need a short flag (see #4).
- Drop `-d` for `--by-dir` if the flag survives at all (see #7).
- On `report`, use `-i`/`-o` for `--input`/`--output`. Reusing `-i` across subcommands is fine — short flags are scoped to their subcommand. The current `-I` exists to dodge a non-conflict.

---

### 3. `--format json` instead of `--json` / `-o`

**Current.** `--format json|csv|table`, with `-f` short.

**Convention.** Two patterns dominate:

- `kubectl -o json|yaml|wide` — `-o` for output format.
- `gh` and friends — a dedicated `--json` flag because JSON is the overwhelmingly common machine-readable case.

`--format` is fine but not idiomatic for Unix CLIs at this size. `-f` collides with `find`'s `-f`, `grep`'s `-f` (file of patterns), `tar`'s `-f` (archive file) — all of which mean "file," not "format." That's a real footgun.

**Recommendation.**

- Rename to `-o, --output FORMAT` (matches `kubectl`, `helm`).
- Add `--json` as a shorthand for `--output json` (matches `gh`).

This frees `-f` for a more natural use later (e.g., `--from FILE` for a saved analysis).

---

### 4. `--decay` and `--decay-half-life` are two flags for one knob

**Current.** `--decay` is a boolean on/off; `--decay-half-life "6 months"` configures it. The half-life flag has no effect unless `--decay` is also passed.

**Convention.** GNU long options support optional values: `--decay[=HALFLIFE]`. `git log --since` takes a value; there is no separate `--enable-since`. `grep --color[=WHEN]` uses optional values for exactly this case.

**Recommendation.** Collapse into a single opt-out flag with adaptive defaults:

```
(no flag)              # decay on, half-life adapts to the analyzed window
--no-decay             # disable; use raw commit counts
--since "6 months"     # narrows the window, which shortens the half-life
```

Decay is always on. The half-life derives from the age of the oldest commit in scope, so narrowing `--since` automatically tightens recency weighting — no separate half-life knob needed. This also removes the `-D` short flag entirely (see #2).

**Note on what shipped.** An earlier draft proposed `--decay[=HALFLIFE]` (GNU optional-value style). The implementation went further: rather than ask the user to pick a half-life, derive it from `--since`. Result is one flag (`--no-decay`) instead of two, with no value to tune.

---

### 5. `prompt ignore-file-spec` — depth and naming

**Current.** Three-level command path with a hyphenated leaf name.

**Convention.** Subcommand names are typically single words: `gh pr create`, `kubectl get pods`, `docker image prune`. Hyphenated subcommand names are rare and almost always a smell of a missing noun.

**Recommendation.**

- `hc prompt ignore` — the "spec" suffix is implementation noise. The output is a prompt; that's already implied by the parent verb.
- If more prompt types arrive, they each get a single-word leaf: `hc prompt ignore`, `hc prompt review`, `hc prompt summary`.

---

### 6. `--ignore` / `-x` doesn't match convention

**Current.** `--ignore PATTERN`, short `-x`.

**Convention.** `tar`, `rsync`, `grep`, and `find` all use `--exclude` for this concept. `-x` in `tar` means "extract" — a strong, conflicting prior. `--ignore` is closer to "ignore-case" semantics in most tools.

**Recommendation.**

- Rename to `--exclude PATTERN` (long), `-e` short. Matches `rsync --exclude`, `tar --exclude`, `grep -e`.
- The `.hcignore` filename can stay — `.gitignore` / `.dockerignore` / `.npmignore` all use `ignore` while their CLI flags use `--exclude`. There's no contradiction.

---

### 7. `--by-dir` should be a value, not a boolean

**Current.** `--by-dir` (`-d`) is a boolean that switches output from per-file to per-directory.

**Convention.** When a flag selects between a small enum of grouping modes, `--group-by KEY` (or `--by KEY`) is more extensible than a boolean. `git log --pretty=KEY`, `du --max-depth=N`, `sort -k FIELD`. Today there are two grouping modes; tomorrow you may want by-author, by-extension, by-quadrant.

**Recommendation.**

```
--by file        # default
--by dir
--by author      # future
```

Removes one boolean and one short-flag collision (#2).

---

### 8. `--top N` vs `--limit N` vs `head -n`

**Current.** `--top N`, short `-n`.

**Convention.** `-n N` for "number of items" matches `head`, `tail`, `tail -n`, `git log -n`. The long form is split: `--limit` (psql, gh), `--max-count` (git), `--head` (less common). `--top` is non-standard but unambiguous in context.

**Recommendation.** Mostly fine. Consider renaming long form to `--limit` to match GitHub CLI / Postgres tooling, but `--top` is defensible. Leave the short `-n` alone.

---

### 9. `--since` is correct

Matches `git log --since`. Use the same parser semantics — relative durations, ISO dates, both. No change.

---

### 10. Missing standard flags

**Current.** No `--color`, no `--quiet`, no `--verbose`, no obvious `--no-pager` story for the table output.

**Convention.**

- `--color=auto|always|never` plus respect for `NO_COLOR` env var (<https://no-color.org>).
- `-v, --verbose` and `-q, --quiet` for log volume.
- TTY detection: when stdout is not a terminal, suppress the "reading JSON from stdin..." hint and any color codes. The `report` command already does the TTY check; analyze should too.

**Recommendation.** Add `--color`, `-v`, `-q`. Honor `NO_COLOR`. Ensure all human-readable hints go to stderr (already the case in `report`, audit `analyze`).

---

### 11. `report --output FILE` does an upsert, not a write

**Current.** `--output FILE` upserts into an existing markdown file rather than overwriting.

**Convention.** `--output FILE` in every standard tool means "write here, replacing what was there." This is genuinely surprising behavior.

**Recommendation.** Either:

- Rename the upsert flag to make the intent explicit: `--upsert FILE` or `--inject FILE`. Then `--output FILE` (or simply `>` redirection) does the obvious thing.
- Or document the upsert behavior prominently and make sure `--output /dev/stdout` and shell redirection still work cleanly.

The first option is cleaner. The upsert behavior is a real feature; it deserves its own flag name.

---

### 12. Path positional argument inconsistency

**Current.** `analyze` and `prompt ignore-file-spec` both take an optional `[path]`; `report` does not (it uses `--input` for the JSON path).

**Recommendation.** Keep as-is — `report` operates on a JSON stream, not a repo path, so the asymmetry reflects a real difference. But document it in `--help` so users don't try `hc report .`.

---

## Proposed Surface

After applying the above:

```
hc [path]                              # default: analyze + render to stdout
hc analyze [path] [flags]              # data collection stage (JSON out)
hc filter [flags]                      # shaping stage (per filter-command.md)
hc report [flags]                      # rendering stage
hc prompt ignore [path] [flags]
hc prompt <other-prompts> ...

Common flags on analyze:
  -s, --since DURATION         Restrict churn window (also shortens decay half-life)
      --no-decay               Disable recency weighting (default: on, adaptive half-life)
  -e, --exclude PATTERN        Exclude pattern (repeatable)
      --by file|dir|author     Grouping mode (default: file)
  -n, --limit N                Top N results
  -o, --output FORMAT          table|json|csv (default: table)
      --json                   Shortcut for --output json

Common flags on report:
  -i, --input FILE             JSON input (default: stdin)
  -o, --output FILE            Write to FILE (default: stdout)
      --upsert FILE            Inject into existing markdown file

Global:
  -v, --verbose
  -q, --quiet
      --color=auto|always|never
      --version
  -h, --help
```

---

## Out of Scope

- Architectural changes to the pipeline (`analyze | filter | report`) — see `filter-command.md`.
- New analysis features (cyclomatic, etc.) — see `cyclomatic-analysis.md`.
- Config file support (`.hcrc`, etc.) — separate question.

---

## Summary

The CLI works. The papercuts are: a missing default action, randomly-cased short flags, two flags doing one job, a non-idiomatic format flag, a hyphenated leaf subcommand, an `--ignore` flag where convention says `--exclude`, a boolean where an enum belongs, an `--output` flag that surprises, and missing standard niceties like `--color` and `NO_COLOR`. None of them is fatal. All of them are visible to anyone who lives in a Unix shell.

---

## Appendix: Grouped Priorities

Two views of the same 12 issues. The **tier** view answers "what should I ship first?" The **category** view answers "what kind of change is each one?" — useful for batching commits.

### By Tier

#### Tier 1 — High impact, ship first

These bite on every invocation or hide a real footgun. They also unblock or simplify later tiers.

| # | Change | Why first | Status |
|---|---|---|---|
| 1 | Default action: `hc` → `hc analyze` | Removes friction from every invocation. | ✅ Implemented |
| 11 | Rename `report --output` → `--upsert`; make `--output` overwrite | Current behavior contradicts every other Unix tool — surprising even to the author. | ✅ Implemented |
| 3 | `--format` → `-o, --output FORMAT`; add `--json` | Frees `-f` from a tar/grep/find collision. Hits every JSON pipeline. | ✅ Implemented |
| 4 | Collapse `--decay` + `--decay-half-life` → `--no-decay` only; decay always-on with adaptive half-life | Halves the decay surface area. Eliminates the `-D` short flag (resolves part of #2). | ✅ Implemented |

#### Tier 2 — Cleanup, ship next

Lower frequency but worth doing in one batch since they're all renames or boolean→enum conversions.

| # | Change | Why second | Status |
|---|---|---|---|
| 6 | `--ignore` / `-x` → `--exclude` / `-e` | Aligns with tar/rsync/grep. `-x` carries a strong "extract" prior. | ✅ Implemented |
| 7 | `--by-dir` boolean → `--by file\|dir\|author` enum | Extensible. Removes another short-flag collision. | ❌ Not implemented |
| 5 | `prompt ignore-file-spec` → `prompt ignore` | Drops the hyphenated leaf. Sets the pattern for future `prompt <noun>` commands. | ✅ Implemented |
| 2 | Audit remaining short-flag casing | Mostly resolved by Tier 1 (#4) and Tier 2 (#7). Whatever's left, tidy. | ⚠️ Partial — only `-d` (by-dir) remains; resolves with #7 |

#### Tier 3 — Polish, ship when convenient

Standard-flag hygiene and documentation. None of these block anything.

| # | Change | Why last | Status |
|---|---|---|---|
| 10 | Add `--color=auto\|always\|never`, `-v`, `-q`; honor `NO_COLOR`; audit stderr/stdout split on `analyze` | Only matters in specific contexts (CI, scripts, no-color terminals). | ❌ Not implemented |
| 8 | Optional: rename `--top` → `--limit` long form | Defensible either way. Keep `-n`. | ✅ Implemented |
| 9 | Document `--since` parser semantics | No code change. | ❌ Not documented |
| 12 | Document path-positional asymmetry between `analyze` and `report` in `--help` | No code change. | ❌ Not documented |

### By Category

Useful for batching commits — each category is roughly one PR's worth of mechanical work.

#### Renames (mechanical, low risk)

- #3 `--format` → `-o, --output FORMAT`
- #6 `--ignore` / `-x` → `--exclude` / `-e`
- #5 `prompt ignore-file-spec` → `prompt ignore`
- #11 `report --output` → `--upsert` (paired with a behavior change)
- #8 (optional) `--top` → `--limit`

#### Collapses (one knob where there were two)

- #4 `--decay` + `--decay-half-life` → `--no-decay` (decay always on; half-life derived from `--since` window)
- #7 `--by-dir` boolean → `--by KEY` enum

#### Behavior changes (semantics shift)

- #1 `hc` (no args) now runs `analyze` instead of erroring
- #11 `report --output FILE` now overwrites instead of upserting (the upsert behavior moves to `--upsert`)

#### Additions (new flags, no removals)

- #1b `--report` shortcut for the full `analyze | filter | report` pipeline (optional)
- #3b `--json` shorthand for `--output json`
- #10 `--color`, `-v`, `-q`, `NO_COLOR` env support

#### No-ops (already correct, just document)

- #9 `--since`
- #12 path-positional asymmetry

### Suggested Commit Order

1. Tier 1 renames + behavior changes (#1, #3, #11) — one commit each.
2. Tier 1 collapse (#4).
3. Tier 2 batch (#5, #6, #7) — could be one commit per item or one combined "flag cleanup" commit.
4. Tier 2 audit (#2) — sweep up whatever short-flag casing inconsistencies remain.
5. Tier 3 additions (#10).
6. Tier 3 docs (#8, #9, #12) — readme + `--help` strings only.
