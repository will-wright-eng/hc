# CLI Default Action — Implementation Plan

## Context

Implements **#1** from `docs/design/cli-ergonomics.md`: make `hc` (no subcommand) behave like `hc analyze`. So `hc` analyzes cwd, `hc some/path` analyzes that path, `hc --json` runs analyze with `--json`. Existing `hc analyze ...` invocations are unchanged.

The design doc's premise #2 says explicitly: "`hc [path]` is sugar for `hc analyze [path]`. Subcommands are the canonical surface; the bare form exists to cut friction on the common case."

---

## Approach

`urfave/cli/v3` exposes a `DefaultCommand string` field on `cli.Command`. Per its doc comment: *"the (optional) name of a command to run if no command names are passed as CLI arguments."*

This means we don't duplicate flags or write a separate root action — we just point the root command at `analyze` as the default. Args and flags after `hc` route through to the `analyze` subcommand's flag set as if `analyze` had been typed.

```go
cmd := &cli.Command{
    Name:           "hc",
    Usage:          "Hot/Cold codebase analysis — churn × complexity hotspot matrix",
    DefaultCommand: "analyze",
    Commands:       []*cli.Command{ /* existing list, unchanged */ },
}
```

That's the entire functional change. One field on the root `cli.Command`.

---

## Scope

**In scope.**

- Add `DefaultCommand: "analyze"` to the root `cli.Command` in `cmd/hc/main.go`.
- Update `readme.md` Usage section so the headline example is `hc` (or `hc .`) rather than `hc analyze`.
- Add a brief readme note that `hc [path]` is sugar for `hc analyze [path]`.

**Out of scope.**

- A `--report` shortcut for the full `analyze | filter | report` pipeline (#1b in the design appendix's "Additions" — deferred).
- Changing what `hc --help` shows. urfave/cli's default help layout already lists subcommands; that stays.
- Renaming or removing the `analyze` subcommand. It remains the canonical verb; the default is sugar over it.

---

## Touch Points

- `cmd/hc/main.go:21-26` — root `cli.Command` literal, where `Name`, `Usage`, and `Commands` are set today. Add `DefaultCommand: "analyze"`.
- `readme.md:13-35` — Usage block. Lead with `hc` instead of `hc analyze` in the first one or two examples.

No other files are affected. No internal API changes. No test changes.

---

## Behavior Matrix

After the change:

| Invocation | Routes to | Notes |
|---|---|---|
| `hc` | `analyze` (cwd) | New behavior. Previously printed help. |
| `hc .` | `analyze .` | New. |
| `hc some/path` | `analyze some/path` | New. |
| `hc --json` | `analyze --json` | New. Flags after `hc` apply to the default. |
| `hc analyze ...` | `analyze ...` | Unchanged. |
| `hc filter ...` | `filter ...` | Unchanged. |
| `hc report ...` | `report ...` | Unchanged. |
| `hc prompt ignore ...` | `prompt ignore ...` | Unchanged. |
| `hc --help` / `hc -h` | help | Unchanged — flag handlers preempt `DefaultCommand`. |
| `hc --version` | version | Unchanged. |
| `hc bogus` | error: command not found | Unchanged — `bogus` doesn't match a subcommand and isn't a path arg the dispatcher tries to route. (Verify; see below.) |

The one edge case worth verifying with the binary is the last row: when the first arg doesn't match a known subcommand, does urfave/cli route it as a positional to `DefaultCommand`, or does it error with "command not found"? Either is acceptable — both `hc bogus` (treated as a path) and `hc bogus` (treated as an unknown command) are reasonable — but the readme example for `hc some/path` depends on which it is. If urfave errors on unknown subcommands, paths that happen to look like subcommand-style words still work (`./bogus`, `path/to/dir`) since the leading `./` or `/` disambiguates.

---

## Discoverability Tradeoff

Today, `hc` (no args) prints help, which advertises the subcommand surface. After this change, a brand-new user who types `hc` in their repo gets a hotspot table, not help. Mitigations:

- `hc --help` / `hc -h` still prints the full surface.
- The `analyze` output is itself a useful first impression.
- The readme leads with `hc` so docs-first users see the sugar.

The design doc's premise accepts this tradeoff. Single-purpose Unix tools (`du`, `wc`, `find`) all run on bare invocation; `hc`'s posture under this change matches that.

---

## Verification

```bash
make build && make lint && make test    # baseline
./hc                                     # should print analyze table for cwd
./hc .                                   # same
./hc internal/                           # analyze a subdir
./hc --json | head -5                    # flags route through to analyze
./hc analyze --json | head -5            # explicit form still works
./hc filter --help                       # other subcommands unaffected
./hc report --help
./hc prompt ignore --help
./hc --help                              # help still lists subcommands
./hc bogus 2>&1 | head -3                # confirm whichever behavior urfave gives us
```

If `./hc bogus` routes through to `analyze` (treating `bogus` as a path arg that doesn't exist), `analyze` will error with a path-not-found message. If urfave intercepts and reports "command not found," that's also fine. Document whichever it is in the readme only if it's surprising.

---

## Suggested Commit

A single commit:

> `cli: default to "analyze" when no subcommand is given`
>
> `hc` and `hc [path]` now behave like `hc analyze [path]`. Implements #1
> from docs/design/cli-ergonomics.md. Uses urfave/cli's DefaultCommand
> field — no flag duplication, no behavior change for explicit subcommands.

---

## Out of Scope (Reminder)

Tier 1 has one remaining item after this lands: **#4** — collapsing `--decay` + `--decay-half-life` into `--decay[=HALFLIFE]`. That gets its own plan; it's a real flag-API change and worth treating separately from this one-liner.
