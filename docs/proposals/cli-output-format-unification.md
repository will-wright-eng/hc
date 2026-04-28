# CLI Output Format Unification — Implementation Plan

## Context

`hc analyze` exposes two flags for the same knob:

```
-o, --output FORMAT    table|json|csv (default: table)
    --json             Shortcut for --output json
```

`--json` was added under #3 of `docs/design/cli-ergonomics.md` as a `gh`-style ergonomic shortcut. The result feels inconsistent next to `-o csv` — two paths to one effect, no conflict check, asymmetric coverage (no `--csv`, no `--table`).

This plan formalizes the intent: `--output/-o` is the canonical flag; `--json` is documented sugar that delegates to it. A conflict between `--json` and `-o <other>` becomes an error rather than a silent override.

The alternative — drop `--json` entirely — is cleaner but loses ergonomics on the dominant pipeline case (`hc analyze --json | hc report`). `--json` is also already on `cli-ergonomics.md`'s shipped list, so removing it reverses a recent decision without new evidence.

---

## Design

### Two patterns considered

1. **Single canonical flag, no shorthands.** `--output/-o {table,json,csv}` only. Matches `kubectl`, `gh` (output side), `aws`, `docker`. Cleanest, one flag to learn. Cost: `--json` becomes `-o json` — two extra characters on the most common pipeline.
2. **Canonical flag + documented aliases.** Keep `-o`; accept `--json` as sugar for `--output json`. Matches `cargo`, `ripgrep`, `jq`. Cost: more flag surface; conflict cases need an explicit error.

This plan chooses **option 2** for the reasons in Context. The discomfort the current state creates is mostly a documentation/error-handling gap, not a flag-surface problem.

### Rules

- `-o/--output` is the canonical flag. Help text leads with it.
- `--json` is sugar for `--output json`. Help text reads exactly that.
- Passing `--json` together with `-o <anything-other-than-json>` is an error:

  ```
  $ hc --json -o csv
  error: --json conflicts with --output csv (use one)
  ```

- `--json -o json` is allowed (idempotent). No special-case for that; a string-equal check is enough.
- No `--csv` or `--table` shorthands. Only `--json` earns its keep — it's the pipeline target. Adding the others would re-introduce the asymmetry this plan is trying to fix.

### Why not drop `--json`?

`gh`-style `--json` is the canonical Unix shortcut for "I'm piping to a machine reader." Three of the four target audiences for `hc` (CI, agent pipelines, the `hc analyze --json | hc report` pipeline documented in `CLAUDE.md`) hit this path. Removing it now would also revert a recently-shipped Tier 1 ergonomics decision without new evidence. Easier to keep the alias and tighten its semantics.

If usage data later shows `--json` is rarely used, deleting it is a one-line change.

---

## Scope

**In scope.**

- Add a conflict check in `runAnalyze` for `--json` + `-o <non-json>`.
- Update `--json`'s usage string to read "Shortcut for `--output json` (cannot combine with `--output`)".
- Update `--output`'s usage string to lead with the canonical role.
- Add a short note in `readme.md` clarifying the alias relationship.
- Add a test for the conflict path.

**Out of scope.**

- Adding `--csv` / `--table` shorthands — see "Rules" above.
- Dropping `--json` — see "Why not drop `--json`?".
- Changes to `report --output` (a different flag with file-path semantics, not format).

---

## Touch Points

- `cmd/hc/main.go` — `analyzeFlags()` (usage strings); `runAnalyze` (conflict check, format derivation).
- `cmd/hc/main_test.go` (or wherever CLI-level tests live) — conflict-error test.
- `readme.md` — flag-table copy.

---

## Code Changes

### `analyzeFlags()` in `cmd/hc/main.go`

```go
&cli.StringFlag{
    Name:    "output",
    Aliases: []string{"o"},
    Usage:   "Output format: table, json, csv",
    Value:   "table",
    Hidden:  hidden,
},
&cli.BoolFlag{
    Name:   "json",
    Usage:  "Shortcut for --output json (cannot combine with --output)",
    Hidden: hidden,
},
```

Only the `--json` usage string changes; the structural shape is unchanged.

### `runAnalyze` in `cmd/hc/main.go`

Today:

```go
format := cmd.String("output")
if cmd.Bool("json") {
    format = "json"
}
```

After:

```go
format := cmd.String("output")
if cmd.Bool("json") {
    if cmd.IsSet("output") && format != "json" {
        return fmt.Errorf("--json conflicts with --output %s (use one)", format)
    }
    format = "json"
}
```

`cmd.IsSet` is the urfave/cli v3 way to distinguish "user passed `-o table`" from "default value of `table`." Without that distinction, the bare `--json` invocation would error against its own default.

### Test

Add to whichever file owns CLI-level tests (or create `cmd/hc/main_test.go`):

```go
func TestAnalyze_JSONConflictsWithOutput(t *testing.T) {
    // hc --json -o csv → exit non-zero, error mentions both flags
}
```

A table-driven case covering `--json -o json` (allowed), `--json -o csv` (rejected), `--json` alone (allowed), `-o csv` alone (allowed) is enough.

### `readme.md`

In the flag table, keep both rows but tie them together:

| Flag | Description |
|---|---|
| `-o, --output FORMAT` | Output format: `table`, `json`, `csv` (default: `table`) |
| `--json` | Shortcut for `--output json`. Cannot combine with `--output`. |

---

## Verification

```bash
make build && make lint && make test
./hc --json | head -5                # ok
./hc -o csv | head -5                # ok
./hc -o json | head -5               # ok
./hc --json -o json | head -5        # ok (idempotent)
./hc --json -o csv                   # error: --json conflicts with --output csv
./hc --json -o table                 # error: --json conflicts with --output table
make e2e                             # full pipeline still passes (uses --json)
```

---

## Tradeoff: Two flags vs one

Keeping both flags grows the help-text surface by one line and adds one error path. The win is preserving the most common pipeline shape (`hc analyze --json | hc report`) while removing the silent-override footgun. If the project later decides flag minimalism beats ergonomics, deleting `--json` is mechanical: drop the flag, drop the conflict check, drop the test case, fix two readme rows.

---

## Suggested Commit

> `cli: tighten --json/--output relationship; reject conflicting combos`
>
> `--json` is now documented sugar for `--output json`. Passing
> `--json` alongside `--output <non-json>` is rejected with a clear
> error instead of silently overriding the user. No flag added or
> removed.

---

## Out of Scope (Reminder)

- `--csv` / `--table` shorthands.
- Dropping `--json`.
- `report --output` (different flag, file path not format).
