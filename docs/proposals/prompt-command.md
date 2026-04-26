# Prompt Command — Proposal

## Context

`.hcignore` tells `hc` which paths to skip during churn and complexity analysis
(generated code, vendored deps, lockfiles, fixtures, minified assets, large
blobs). Writing one by hand is tedious and error-prone — the best ignore set
depends on what actually lives in the repo.

An LLM is a good fit for the task: give it a short syntax spec and a summary of
the repo, and it can produce a reasonable `.hcignore` in one shot. But `hc`
should not ship an LLM client — no API keys, no provider choice, no network
calls. Instead, `hc` emits a prompt to stdout and the user pipes it into
whatever harness they already have:

```bash
hc prompt ignore-file-spec | claude -p > .hcignore
```

This keeps `hc` composable (matching `analyze | report`) and agnostic about
which LLM you use.

---

## Command Structure

New top-level command group `prompt`, parallel to `analyze` and `report`. First
subcommand is `ignore-file-spec`; the group is designed to host more prompts
later (e.g. `prompt quadrant-review`).

```
hc prompt ignore-file-spec [path]
```

Flags (minimal — add only if justified):

- `[path]` positional arg, default `.` — the repo to summarize.
- `--max-files N` (default ~200) — cap the file listing so the prompt stays
  tractable for small context windows.
- `--no-summary` — emit prompt text only, skip the repo summary. For users who
  want to assemble context themselves.

---

## File Layout

```
cmd/hc/main.go                     # wire up `prompt` command + subcommand
internal/prompt/                   # new package
  prompt.go                        # prompt templates + Render(path, w, opts)
  prompt_test.go
  summary.go                       # repo summary helpers
  summary_test.go
  templates/
    ignore_file_spec.md            # static prompt body (embedded)
```

The prompt text lives in a real `.md` file so it's editable, reviewable in
diffs, and renderable standalone. Embedded into the binary via `//go:embed` so
`hc` stays a single artifact.

---

## Prompt Body (`templates/ignore_file_spec.md`)

Contains:

- Role + goal: generate a `.hcignore` for `hc`'s churn × complexity analysis.
- Syntax rules:
  - Bare name (no `/`) matches basename at any depth.
  - Trailing `/` anchors a directory at repo root.
  - `**` is the recursive wildcard.
  - No negation (`!`) — unsupported.
  - Standard glob metacharacters (`filepath.Match`): `*`, `?`, `[...]`.
- Output format: only the `.hcignore` contents, grouped under short `#`
  section headers (deps, generated, lockfiles, assets, fixtures).
- Placeholder block `{{REPO_SUMMARY}}` that `Render` fills in.

---

## Repo Summary (`summary.go`)

Compact, token-cheap context the LLM actually needs to pick good patterns:

1. **Top-level tree** — 1–2 levels deep, directories only, with file counts per
   directory.
2. **Extension histogram** — top ~20 extensions by file count
   (`.go: 142`, `.json: 38`, …).
3. **Largest files** — top ~15 by byte size. Catches minified bundles,
   fixtures, vendored blobs.
4. **Notable filenames at repo root** — `go.sum`, `package-lock.json`,
   `yarn.lock`, etc., if present.

Walk with `filepath.WalkDir`, skip `.git/`. Don't import `complexity` or `git`
— this command should be cheap and work in non-git directories too.

Output as a fenced ```` ```text ```` block inside the rendered prompt.

---

## Render Signature

```go
// internal/prompt/prompt.go
type IgnoreOpts struct {
    MaxFiles  int
    NoSummary bool
}

func RenderIgnoreFileSpec(root string, w io.Writer, opts IgnoreOpts) error
```

`main.go` action resolves the path, reads flags, and calls
`RenderIgnoreFileSpec(absPath, os.Stdout, opts)`.

---

## Tests

- `prompt_test.go`: golden-file test that `RenderIgnoreFileSpec` against a
  small fixture tree produces expected output (syntax rules section, summary
  section, correct extension counts).
- `summary_test.go`: unit tests for extension histogram, largest-files sort,
  tree rendering depth cap, `.git/` exclusion.
- Table test for `--no-summary` flag: verify `{{REPO_SUMMARY}}` placeholder is
  absent and no summary block appears.

---

## Docs

- README: new section "Generating a `.hcignore`" with the `| claude -p >`
  one-liner.
- `make e2e` unchanged; optionally add `make prompt` target that prints the
  rendered prompt for the current repo.

---

## Out of Scope

- No LLM client in-tree — no API keys, no provider choice, no network calls.
- No interactive TUI.
- No writing `.hcignore` directly from `hc`. The user redirects stdout. Keeps
  the tool composable and avoids owning a destructive write path.

---

## Rollout Order

1. Create `internal/prompt/templates/ignore_file_spec.md` with the static
   prompt text.
2. Build `summary.go` + tests (pure, no CLI wiring yet).
3. Build `prompt.go` with `//go:embed` + `Render` + tests.
4. Wire `prompt` + `ignore-file-spec` into `cmd/hc/main.go`.
5. README update.
