# CLI Renames — Implementation Plan

## Context

Implements the **Renames** category from `docs/design/cli-ergonomics.md`. Five flag/command renames driven by Unix convention. No backwards compatibility (single-user tool), so each change is a hard cut — old flag removed in the same commit that adds the new one.

The five renames, in the suggested commit order:

| # | Surface | Old | New |
|---|---|---|---|
| 3 | `analyze` flag | `--format` / `-f` | `--output` / `-o` (+ `--json` shorthand) |
| 6 | `analyze` flag | `--ignore` / `-x` | `--exclude` / `-e` |
| 11 | `report` flag | `--output` / `-o` (upsert behavior) | `--upsert` (`--output` becomes a plain overwrite) |
| 5 | subcommand | `prompt ignore-file-spec` | `prompt ignore` |
| 8 | `analyze` flag | `--top` (long form only) | `--limit` |

# 11 is a rename **paired with a behavior change**: the upsert behavior moves to `--upsert`, and `--output FILE` is added with the standard "write to file, overwriting" semantics. Both parts ship in one commit

# 8 is marked optional in the design doc; included here for completeness but can be deferred

---

## Scope

**In scope.** Flag/command names visible to users, their `--help` strings, internal Go identifiers that mirror the CLI surface, tests that reference them, the readme flag table, and the Makefile `e2e` target.

**Out of scope.**

- The `--decay` collapse (#4) — not a rename, lives in its own plan.
- The `--by-dir` → `--by KEY` change (#7) — boolean→enum, not a rename.
- Default action (#1) — separate plan.
- Internal `output.FormatFiles(format string, ...)` parameter name — the value is still a format string from the function's perspective, even though the user-facing flag becomes `--output`. Leave it.

---

## 1. `--format` → `--output` / `-o` (+ `--json`)

### Touch points

- `cmd/hc/main.go:40-45` — `StringFlag{Name: "format", Aliases: ["f"]}` declaration on `analyze`.
- `cmd/hc/main.go:135` — `format := cmd.String("format")` in `runAnalyze`.
- `readme.md:45` — flag table row.
- `Makefile:27` — `e2e` target uses `--format json`.

### Changes

In `cmd/hc/main.go`, replace the `format` flag with two:

```go
&cli.StringFlag{
    Name:    "output",
    Aliases: []string{"o"},
    Usage:   "Output format: table, json, csv",
    Value:   "table",
},
&cli.BoolFlag{
    Name:  "json",
    Usage: "Shortcut for --output json",
},
```

In `runAnalyze`:

```go
format := cmd.String("output")
if cmd.Bool("json") {
    format = "json"
}
```

The `format` variable name inside `runAnalyze` and the `format string` parameter on `output.FormatFiles` / `output.FormatDirs` stay — they describe the value, not the flag. No internal API change.

### Ripple

- Update `Makefile:27`: `--format json` → `--json` (cleanest demo of the new shorthand).
- Update `readme.md:45` row.
- Validation: if both `--output csv` and `--json` are passed, `--json` wins. Document in `--help`. (Or error out — pick one. Recommendation: `--json` wins silently; it's a shorthand.)

---

## 2. `--ignore` → `--exclude` / `-e`

### Touch points

- `cmd/hc/main.go:56-60` — `StringSliceFlag{Name: "ignore", Aliases: ["x"]}`.
- `cmd/hc/main.go:148` — `cmd.StringSlice("ignore")` in `runAnalyze`.
- `readme.md:48` — flag table row.

### Changes

Rename the flag declaration:

```go
&cli.StringSliceFlag{
    Name:    "exclude",
    Aliases: []string{"e"},
    Usage:   "Glob pattern to exclude (repeatable, .gitignore syntax)",
},
```

Update the read site: `cmd.StringSlice("exclude")`.

### Ripple

- The `.hcignore` file name does **not** change. The CLI flag and the file format are separate concerns; `.gitignore` / `tar --exclude` precedent applies.
- `internal/ignore/` package name does not change — the file format is still a gitignore-style ignore file.
- Update `readme.md:48`.

---

## 3. `report --output` → `--upsert` (+ new `--output` with overwrite semantics)

### Touch points

- `cmd/hc/main.go:83-87` — `--output` / `-o` flag on `report`.
- `cmd/hc/main.go:188` — `outputPath := cmd.String("output")`.
- `cmd/hc/main.go:211-217` — branch that calls `report.UpsertFile(outputPath, ...)`.
- `internal/report/upsert.go:52` — `UpsertFile(path, content string)` (no rename needed; name already accurate).
- `readme.md:55` — flag table row for `--output`.

### Changes

Replace the single `--output` flag on `report` with two flags:

```go
&cli.StringFlag{
    Name:    "output",
    Aliases: []string{"o"},
    Usage:   "Write report to FILE (overwrites; default: stdout)",
},
&cli.StringFlag{
    Name:  "upsert",
    Usage: "Inject report into existing markdown file (preserves surrounding content)",
},
```

In `runReport`, branch on which is set:

```go
upsertPath := cmd.String("upsert")
outputPath := cmd.String("output")

// ... render into buf ...

switch {
case upsertPath != "":
    if err := report.UpsertFile(upsertPath, buf.String()); err != nil {
        return fmt.Errorf("writing output: %w", err)
    }
    fmt.Fprintf(os.Stderr, "report upserted into %s\n", upsertPath)
    return nil
case outputPath != "":
    if err := os.WriteFile(outputPath, buf.Bytes(), 0o644); err != nil {
        return fmt.Errorf("writing output: %w", err)
    }
    fmt.Fprintf(os.Stderr, "report written to %s\n", outputPath)
    return nil
default:
    _, err := buf.WriteTo(os.Stdout)
    return err
}
```

If both `--output` and `--upsert` are passed, error: `--output and --upsert are mutually exclusive`.

### Ripple

- `internal/report/upsert.go` `UpsertFile` function: no rename, name is already accurate.
- `report --input` / `-I`: out of scope here. (The design doc proposes lowering `-I` to `-i`; that's a casing fix, not a rename.) But: with `--output` now meaning overwrite, the `-i` / `-o` short pair becomes natural. Worth doing in the same commit since it's one line: change `Aliases: []string{"I"}` → `Aliases: []string{"i"}` on the input flag.
- Update `readme.md:55` — split into two rows.

---

## 4. `prompt ignore-file-spec` → `prompt ignore`

### Touch points

- `cmd/hc/main.go:96` — `Name: "ignore-file-spec"`.
- `cmd/hc/main.go:110` — `Action: runPromptIgnoreFileSpec`.
- `cmd/hc/main.go:223` — `func runPromptIgnoreFileSpec(...)`.
- `cmd/hc/main.go:239` — `prompt.RenderIgnoreFileSpec(...)`.
- `internal/prompt/prompt.go:18-20` — `RenderIgnoreFileSpec` declaration.
- `internal/prompt/prompt.go:12` — `IgnoreOpts` doc comment references function.
- `internal/prompt/prompt_test.go:9,12,32,35,52,55,65,68` — four test functions named `TestRenderIgnoreFileSpec_*` plus their call sites.

### Changes

CLI:

```go
{
    Name:      "ignore",
    Usage:     "Emit a prompt that asks an LLM to generate a .hcignore file",
    ArgsUsage: "[path]",
    // flags unchanged
    Action: runPromptIgnore,
},
```

Rename in `cmd/hc/main.go`: `runPromptIgnoreFileSpec` → `runPromptIgnore`.

Rename in `internal/prompt/prompt.go`: `RenderIgnoreFileSpec` → `RenderIgnore`. Update doc comment.

Rename in `internal/prompt/prompt_test.go`: test functions become `TestRenderIgnore_*`, call sites updated.

### Ripple

- `IgnoreOpts` struct name stays — it's already the right name.
- Template files under `internal/prompt/templates/` — check filename; if there's an `ignore-file-spec.tmpl` or similar, rename to `ignore.tmpl`.
- No readme references to verify (the `prompt` subcommand isn't in the readme flag tables).

---

## 5. (Optional) `--top` → `--limit`

### Touch points

- `cmd/hc/main.go:46-50` — `IntFlag{Name: "top", Aliases: ["n"]}`.
- `cmd/hc/main.go:137` — `top := cmd.Int("top")`.
- `cmd/hc/main.go:174,180,181` — three reads of the local `top` variable.
- `readme.md:46` — flag table row.

### Changes

```go
&cli.IntFlag{
    Name:    "limit",
    Aliases: []string{"n"},
    Usage:   "Limit to top N results",
},
```

Update `runAnalyze`: `top := cmd.Int("limit")` (or rename the local to `limit` for consistency — preferred).

Short alias `-n` stays (matches `head -n`, `git log -n`).

### Ripple

- Update `readme.md:46`.
- Local variable rename inside `runAnalyze` is a clean-up bonus: `top` → `limit` everywhere in the function.

---

## Verification

After each commit:

```bash
make build && make lint && make test
```

E2E sanity (after the `--format` and `--ignore` commits):

```bash
./hc analyze --json | ./hc report                 # exercises new --json shorthand
./hc analyze --exclude 'vendor/**' --limit 5      # exercises new --exclude and --limit
./hc report --output /tmp/report.md               # overwrites
./hc report --upsert HOTSPOTS.md                  # upserts
./hc prompt ignore                                # new subcommand path
```

`make e2e` should pass after the `--format` commit (since the Makefile target itself is updated in that commit).

Help-output spot-check:

```bash
./hc analyze --help
./hc report --help
./hc prompt --help
./hc prompt ignore --help
```

Look for stale flag names or stale `Usage` strings.

---

## Suggested Commit Sequence

1. **`cli: rename --format to --output, add --json`** — flag #3. Updates `Makefile` `e2e` target in same commit.
2. **`cli: rename --ignore to --exclude`** — flag #6.
3. **`cli: split report --output into --output (overwrite) and --upsert`** — flag #11. Includes `-I` → `-i` short-alias fix on `report --input` since both touch the same flag block.
4. **`cli: rename "prompt ignore-file-spec" to "prompt ignore"`** — #5. Includes `RenderIgnoreFileSpec` → `RenderIgnore` Go rename and test renames.
5. **(optional) `cli: rename --top to --limit`** — #8. Defer if it feels like churn.

Each commit is self-contained: build passes, tests pass, readme updated.

---

## Out of Scope (Reminder)

The design doc lists other categories — collapses (#4, #7), behavior changes (#1, and #11's behavior half is here), additions (#10, plus the `--json`/`--report` additions which are folded into the rename commits where they naturally belong), no-ops (#9, #12). Each gets its own implementation plan.
