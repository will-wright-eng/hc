# CLI Decay Flag — Implementation Plan

## Context

Implements **#4** from `docs/design/cli-ergonomics.md` (last Tier 1 item). Today decay is configured by two flags:

```
--decay / -D                  bool, on/off (default: off)
--decay-half-life "6 months"  string, only effective when --decay is set
```

Two flags for one knob, plus a fixed `"6 months"` default that has no relationship to the analysis window.

This plan removes both flags and replaces them with a single `--no-decay` toggle. Decay is always-on; the half-life adapts to the timescale of the analyzed commit history. Pre-1.0 makes the behavior change acceptable; recency-weighted hotspots are what most users actually want when they ask "what's hot in this repo right now?"

---

## Design

### Flag shape

```
hc                    # decay on, half-life = adaptive (from commit history span)
hc --no-decay         # off (raw commit counts)
```

That's it. One flag. No short alias, no value to type, no override.

The user controls the timescale **indirectly through `--since`**: a narrower window yields a shorter adaptive half-life. To get a 6-month half-life, run `hc --since "6 months"`. To get raw counts, run `hc --no-decay`. One knob (window) controls both data range and decay timescale, which is the right coupling — a half-life longer than the data window is meaningless, and a half-life much shorter than the window over-weights the very recent.

### Adaptive half-life

Default half-life is **the age of the oldest commit in the analyzed window** — `now - oldestCommit`, in days. Computed from the `commitFiles` slice `git.Log` already loads.

The intuition: with this half-life, the *oldest* commit in the window is weighted at ~0.5, the *newest* near 1.0. Volume still matters; ancient activity doesn't dominate.

Examples:

| Invocation | Window | Oldest commit | Half-life used |
|---|---|---|---|
| `hc` (active repo, 5y old) | full history | 5y ago | 5 years |
| `hc --since "6 months"` | last 6mo | ~6mo ago | ~6 months |
| `hc --since "2 weeks"` | last 2w | ~2w ago | ~2 weeks |
| `hc --no-decay` | full history | n/a | (no decay) |

### Edge cases

- **Empty result set** (no commits in window): nothing to weight; behavior identical to today.
- **Single commit** or commits all on the same day: oldest-age rounds to ~0 days. Guard: if computed half-life is `<= 0`, fall back to `1.0` weight (no decay) — same behavior as today's `DecayWeight` when `halfLifeDays <= 0`.
- **Future-dated commits** (clock skew): already handled in `DecayWeight` (`if ageDays < 0 { ageDays = 0 }`).

### Why no override flag?

Removing `--decay VALUE` removes the option to pin a fixed half-life. The cost: users who want a half-life decoupled from the window (e.g., to keep SCOREs comparable across time) lose a knob. The benefit: zero flag surface to type for the common case, and the half-life is no longer an arbitrary constant disconnected from the data.

If real usage shows a need for a fixed half-life, `--decay VALUE` can be re-added later as a strict addition. Easier to add a flag than to remove one.

---

## Scope

**In scope.**

- Remove `--decay` (bool, `-D`) and `--decay-half-life` (string, default `"6 months"`).
- Add `--no-decay` (bool).
- Make decay-on the default behavior; default half-life is computed from the loaded commits.
- Change `git.Log` signature so it can distinguish "off" from "on with adaptive default."
- Add a small helper to compute the adaptive half-life.
- Update `Makefile` `e2e` target.
- Update `CLAUDE.md` and `readme.md`.

**Out of scope.**

- Re-adding an explicit-half-life override flag — defer until needed.
- `ParseHalfLife` cleanup — no longer called from `runAnalyze`, but still tested in isolation. Leave it for a future sweep.
- Compact half-life formats (`6mo`, `90d`) — irrelevant once user input doesn't carry a half-life.

---

## Touch Points

- `cmd/hc/main.go` (`analyzeFlags()`) — flag definitions.
- `cmd/hc/main.go` (`runAnalyze`) — derive `decay` from `--no-decay`; remove `ParseHalfLife` call.
- `internal/git/git.go` (`Log`) — signature change; compute adaptive half-life when decay is on.
- `internal/git/decay.go` — add `defaultHalfLifeDays(commits, now)`.
- `internal/git/decay_test.go` — tests for the new helper.
- `Makefile:27` — drop `-D` from `e2e`.
- `CLAUDE.md`, `readme.md` — flag references.

---

## Code Changes

### `analyzeFlags()` in `cmd/hc/main.go`

Remove:

```go
&cli.BoolFlag{
    Name:    "decay",
    Aliases: []string{"D"},
    Usage:   "Weight commits by recency (exponential decay)",
},
&cli.StringFlag{
    Name:  "decay-half-life",
    Usage: "Half-life for decay weighting (e.g. \"90 days\", \"6 months\")",
    Value: "6 months",
},
```

Replace with:

```go
&cli.BoolFlag{
    Name:  "no-decay",
    Usage: "Disable recency weighting (use raw commit counts)",
},
```

### `runAnalyze` body

Today:

```go
decay := cmd.Bool("decay")
var halfLifeDays float64
if decay {
    halfLifeDays, err = gitpkg.ParseHalfLife(cmd.String("decay-half-life"))
    if err != nil {
        return fmt.Errorf("parsing decay half-life: %w", err)
    }
}
```

After:

```go
decay := !cmd.Bool("no-decay")
```

That's the entire decay-handling block in `runAnalyze`. The half-life is `git.Log`'s problem now.

### `git.Log` signature and body

Today: `Log(repoPath, since string, ig *ignore.Matcher, halfLifeDays float64)`. The "off" case is signaled by `halfLifeDays <= 0`.

After: `Log(repoPath, since string, ig *ignore.Matcher, decay bool)`. Compute the half-life inside `Log` when `decay == true`; otherwise pass 0 to `DecayWeight` (which already returns 1.0 in that case).

```go
func Log(repoPath string, since string, ig *ignore.Matcher, decay bool) ([]FileChurn, error) {
    commitFiles, err := gitLogFiles(repoPath, since)
    if err != nil { return nil, err }
    // ...
    now := time.Now()
    var halfLifeDays float64
    if decay {
        halfLifeDays = defaultHalfLifeDays(commitFiles, now)
    }
    // existing weighting loop unchanged — DecayWeight handles halfLifeDays<=0
    // ...
}
```

Callers updated: `runAnalyze` passes `decay` (a bool derived from `--no-decay`).

### `defaultHalfLifeDays` in `internal/git/decay.go`

```go
// defaultHalfLifeDays returns now - oldestCommit, in days.
// Returns 0 if commits is empty or all commits are at/after now.
func defaultHalfLifeDays(commits []CommitInfo, now time.Time) float64 {
    var oldest time.Time
    for _, c := range commits {
        if oldest.IsZero() || c.Date.Before(oldest) {
            oldest = c.Date
        }
    }
    if oldest.IsZero() {
        return 0
    }
    days := now.Sub(oldest).Hours() / 24
    if days <= 0 {
        return 0
    }
    return days
}
```

Tests: empty slice → 0, single commit today → 0, single commit 30 days ago → 30, future-dated commit → 0, mixed past+future → distance to oldest past commit.

### `Makefile`

```diff
-./hc analyze -D -i --json | ./hc report
+./hc analyze -i --json | ./hc report
```

Decay is now default. The e2e target naturally exercises the adaptive default.

### `CLAUDE.md` and `readme.md`

Replace decay-related references with:

> **Decay**: commits are weighted by recency by default; half-life adapts to the analysis window (= age of oldest commit in scope). Use `--no-decay` for raw commit counts. Narrow the window with `--since` to shorten the half-life.

Add a `--no-decay` row to the readme flag table; remove `--decay` and `--decay-half-life` rows.

---

## Verification

```bash
make build && make lint && make test     # baseline; new defaultHalfLifeDays tests must pass
./hc                                      # decay on, adaptive half-life, SCORE column visible
./hc --since "6 months"                   # narrower window, shorter adaptive half-life
./hc --no-decay --limit 5                 # raw counts
./hc -D                                   # error: flag provided but not defined
./hc --decay "90 days"                    # error: flag provided but not defined
./hc --decay-half-life "6 months"         # error: flag provided but not defined
make e2e                                  # full pipeline runs with new defaults
```

Sanity-check SCORE values against a prior `--decay -D` run on the same repo: ordering should be similar, magnitudes will differ (longer adaptive half-life on a multi-year repo).

---

## Tradeoff: Output stability

With a fixed `"6 months"` default, SCORE values were stable across runs. With an adaptive default, SCORE values shift as new commits arrive (the oldest-commit anchor moves). Quadrant assignment is unaffected (median-split). Anything that diffs SCORE values across time would need a fixed half-life — see "Why no override flag?" above. Defer adding `--decay VALUE` until that need is concrete.

---

## Suggested Commit

> `cli: replace decay flags with --no-decay; default to adaptive half-life`
>
> Removes `--decay`/`-D` and `--decay-half-life`. Decay is now on by default
> with half-life = age of oldest commit in the analyzed window. Use
> `--no-decay` for raw counts; narrow the window via `--since` to shorten
> the half-life. Implements #4 from docs/design/cli-ergonomics.md.

---

## After This Lands

- Tier 1: complete.
- Tier 2 (#2) audit: `-D` is gone. Only `-d` (by-dir) remains, which #7 will resolve.
- Remaining: Tier 2 #7 (`--by-dir` → `--by KEY`) and Tier 3 polish (#10, #9, #12).

---

## Out of Scope (Reminder)

- An explicit `--decay VALUE` override — defer until needed.
- Pruning `ParseHalfLife` (still tested but no longer called from `runAnalyze`).
- Compact half-life formats — moot under this design.
