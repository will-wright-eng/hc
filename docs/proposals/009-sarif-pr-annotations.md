# SARIF Output for Code-Scanning PR Annotations

## Overview

Add a `hc sarif` command that renders `hc analyze --json` into a
[SARIF 2.1.0](https://docs.oasis-open.org/sarif/sarif/v2.1.0/sarif-v2.1.0.html)
document, so hotspot findings can be uploaded to GitHub code scanning. This
surfaces hotspots in the repository **Security tab** and as a **pull-request
check**, using the same standard format that `zizmor`, CodeQL, and most linters
already emit.

SARIF is a downstream consumer of the analyze envelope, exactly like
[`hc md report`](../../internal/md/report.go) and
[`hc md comment`](../../internal/md/comment.go). It reads
`schema.Envelope` from stdin (or `--input`) and writes a SARIF log to stdout —
no changes to `analyze`, the analysis core, or the JSON contract.

```bash
hc analyze --json | hc sarif > results.sarif   # then: upload-sarif
```

### Why this, given `hc md comment` already exists

`hc md comment` was built before SARIF was on the table. The two are
**complementary surfaces over the same findings**, not competitors:

| | `hc md comment` | `hc sarif` (proposed) |
| --- | --- | --- |
| Mechanism | PR review comments via REST | Code scanning (SARIF upload) |
| Where it shows | Inline on the PR, per file | Security tab + PR "code scanning" check |
| Persistence | Lives in the PR thread | Tracked alerts, dismissal workflow, history |
| Format | hc-specific NDJSON | Industry-standard SARIF (interoperable) |
| Granularity | File-level | File-level |

Keep both. `hc md comment` stays the choice for inline, conversational PR
feedback; `hc sarif` adds the standards-based dashboard + check surface and
makes hc legible to any tool that already consumes SARIF.

### File-level findings are intentional, not a limitation

GitHub renders inline "Files changed" annotations from code scanning **only
when every line of a finding's region is inside the PR diff**
([docs](https://docs.github.com/en/code-security/code-scanning/managing-code-scanning-alerts/triaging-code-scanning-alerts-in-pull-requests)).
hc findings are about an *entire file* — the specific hunk a PR touches is not
what hc is flagging — so they anchor at `startLine: 1` and will rarely render
as inline diff annotations.

That is fine and by design. hc's findings reliably surface where they belong:

- **Security tab** — every finding, always, severity-tracked and dismissible.
- **"Code scanning results" PR check** — associates the findings with the PR.
- Inline annotations — only incidentally (when line 1 is in the diff); a
  non-goal.

Chasing inline annotations would mean teaching hc to anchor findings to a
changed line — diff-awareness it does not have and does not want. Explicitly
out of scope (see below).

---

## The `hc sarif` command

### Surface

A top-level `hc sarif` subcommand, sibling to `analyze` and the `md` group.
It mirrors the `md` consumers' I/O exactly:

- Reads the envelope from `--input FILE` or stdin (reuse `openJSONInput`).
- Writes to `--output FILE` or stdout.
- Rejects bare-array / pre-envelope input with the same guidance message
  `RenderComments` already produces.
- `--quadrants` filter (repeatable / comma-list), matching
  `hc md comment`'s `CommentOpts.Quadrants`.

It is **not** placed under `md` (SARIF is not markdown) and is **not** a new
`--output sarif` format on `analyze` — see Alternatives.

### Which quadrants become findings

Default to the same set `hc md comment` emits — the quadrants that represent
actionable risk — and let `--quadrants` widen it:

| Quadrant | Default | SARIF `level` | Rule intent |
| --- | --- | --- | --- |
| `hot-critical` | ✅ emit | `warning` | High churn × high complexity — top refactor target |
| `cold-complex` | ✅ emit | `note` | Complex but stable — risky to change; simplify/document |
| `hot-simple` | opt-in | `note` | Churny but simple — usually fine; watch instability |
| `cold-simple` | never | — | Healthy; not a finding |

Severity is driven by SARIF **`level`** (`warning`/`note`), **not**
`security-severity`. `security-severity` is for security rules; it would file
maintainability hotspots under critical/high/medium/low buckets and fail the
PR check aggressively. Keeping everything at `warning`/`note` is informational
by default and consistent with hc's non-blocking posture. Gating (`error`-level
`hot-critical`) is deliberately deferred — see Open questions.

### SARIF hc emits

One `reportingDescriptor` (rule) per emitted quadrant; one `result` per file.
`uri` is the envelope `path` verbatim (already repo-root-relative,
forward-slash). `region: {startLine: 1}` marks the whole-file anchor. Results
are ordered by quadrant rank then weighted commits descending — the same order
used in `internal/md/comment.go` — for stable diffs.

```json
{
  "$schema": "https://json.schemastore.org/sarif-2.1.0.json",
  "version": "2.1.0",
  "runs": [
    {
      "tool": {
        "driver": {
          "name": "hc",
          "informationUri": "https://github.com/will-wright-eng/hc",
          "version": "1.2.0",
          "rules": [
            {
              "id": "hot-critical",
              "name": "HotCritical",
              "shortDescription": { "text": "Hotspot: high churn × high complexity" },
              "fullDescription": { "text": "Frequently changed and structurally complex. Highest maintenance risk and the most valuable refactoring target." },
              "help": { "text": "Reduce complexity (extract, simplify) or stabilize the churn driving changes here." },
              "defaultConfiguration": { "level": "warning" },
              "properties": { "tags": ["maintainability", "hotspot"] }
            },
            {
              "id": "cold-complex",
              "name": "ColdComplex",
              "shortDescription": { "text": "Complex but stable: high complexity × low churn" },
              "fullDescription": { "text": "Structurally complex but rarely changed. Risky to modify when it eventually needs work; simplify or document before it turns hot." },
              "help": { "text": "Add tests/docs or simplify proactively so future changes are safe." },
              "defaultConfiguration": { "level": "note" },
              "properties": { "tags": ["maintainability", "hotspot"] }
            }
          ]
        }
      },
      "automationDetails": { "id": "hc" },
      "results": [
        {
          "ruleId": "hot-critical",
          "level": "warning",
          "message": { "text": "internal/git/git.go is a Hot/Critical hotspot (47 commits, weighted 31.4, complexity 612, 4 authors)." },
          "locations": [
            {
              "physicalLocation": {
                "artifactLocation": { "uri": "internal/git/git.go" },
                "region": { "startLine": 1 }
              }
            }
          ],
          "partialFingerprints": { "primaryLocationLineHash": "hot-critical:internal/git/git.go" }
        }
      ]
    }
  ]
}
```

### Stability and determinism

- **`partialFingerprints.primaryLocationLineHash`** is set explicitly, stable
  per `(path, ruleId)` (e.g. a hash of `ruleId + "\0" + path`). This keeps an
  alert tracked across commits even though the line-1 anchor is synthetic, and
  avoids duplicate alerts when uploading via the REST API (which, unlike the
  `upload-sarif` action, does not auto-populate fingerprints).
- **No timestamps** are copied from the envelope into the SARIF, so
  `hc analyze --json | hc sarif` is byte-reproducible for a fixed envelope and
  can be pinned with a golden-file test.
- Set `automationDetails.id: "hc"` so the analysis is self-categorizing;
  uploads should also pass `category: hc`.

### CI integration (dogfooding in this repo)

A workflow runs the pipeline and uploads via `upload-sarif`. Because
`--format=sarif` is reproducible and informational, this is non-blocking by
nature; the findings populate the Security tab + the code-scanning check.

```yaml
name: hotspots-sarif
on:
  pull_request:
  push:
    branches: [main]

permissions: {}

jobs:
  hotspots:
    runs-on: ubuntu-latest
    permissions:
      security-events: write  # upload SARIF -> code scanning (free on public repos)
      contents: read
    steps:
      - uses: actions/checkout@<sha>  # full history needed for churn
        with:
          fetch-depth: 0
          persist-credentials: false
      - uses: actions/setup-go@<sha>
        with:
          go-version-file: go.mod
      - run: make build
      - run: ./hc analyze --json | ./hc sarif > results.sarif
      - uses: github/codeql-action/upload-sarif@<sha>
        with:
          sarif_file: results.sarif
          category: hc
```

Permissions: `security-events: write` (all repos), plus `actions: read` on
private repos. Code scanning + SARIF upload is **free on public repos**;
private/internal repos require GitHub Advanced Security / Code Security.

### Risk

Low. New, additive command; no changes to `analyze`, `internal/analysis`, or
the JSON envelope. Fully unit-testable against a golden SARIF file from a fixed
envelope.

---

## Alternatives considered

- **`--output sarif` on `analyze`.** Lower friction (one command), but it
  couples SARIF construction into `analyze`'s output dispatch
  (`internal/output`) and makes `analyze` emit something other than the single
  canonical JSON. SARIF needs rule metadata (descriptions, help, level
  mapping) — that is *rendering*, not *serialization*, and belongs with the
  envelope consumers (`md`), not the producer. A subcommand keeps `analyze`'s
  output canonical, stays composable in pipelines, and is easier to golden-test
  in isolation. Rejected for v1; revisit only if a one-shot ergonomic becomes a
  real ask.
- **Reusing `security-severity` for severity.** Rejected — it mislabels
  maintainability findings as security severities and over-gates the PR check.
  `level` is the correct axis.
- **Diff-aware line anchoring for inline annotations.** Rejected — see
  Non-goals.

---

## Implementation plan

1. **`internal/sarif`** — a new package (sibling to `internal/md`) holding the
   SARIF 2.1.0 types and a `Render(r io.Reader, w io.Writer, opts) error` that
   parses `schema.Envelope`, filters/sorts files, and writes the log. Reuse the
   quadrant rank/order and the bare-array guard from `internal/md/comment.go`.
2. **Rule metadata** — a small table mapping each emitted quadrant to its rule
   (`id`, `name`, descriptions, `help`, default `level`). Source the prose from
   the existing comment templates so the language stays consistent.
3. **`cmd/hc/main.go`** — register the `sarif` command with `--input`,
   `--output`, `--quadrants`; wire the `version` var into `tool.driver.version`.
4. **Tests** — golden SARIF file in `internal/sarif/testdata`; a round-trip
   test (`analyze --json | sarif`) asserting valid, deterministic output;
   filter tests for `--quadrants`; the bare-array rejection path.
5. **Docs** — `CLAUDE.md` architecture note, README usage, and the dogfooding
   workflow above.

---

## Open questions

- **Gating.** Should `hot-critical` ever be `error` (failing the PR check)?
  Default no (informational). If wanted later, add a `--fail-level`/quadrant→
  level override rather than hard-coding it.
- **Rule id namespacing.** Plain quadrant keys (`hot-critical`) vs prefixed
  (`hc/hot-critical`). Plain matches hc's vocabulary everywhere else; `category:
  hc` already scopes the analysis. Leaning plain.
- **`hot-simple` default.** Off by default (mirrors `hc md comment`); revisit
  if users want it surfaced.

---

## Non-goals

- **Inline "Files changed" diff annotations.** Requires anchoring findings to a
  changed line; hc is intentionally file-level. Out of scope.
- **Per-line / region analysis.** No line data exists in the pipeline today.
- **`security-severity` buckets.** Not a security tool.
- **Replacing `hc md comment`.** It stays; SARIF is an additional surface.

---

## Test coverage to add

- Golden SARIF for a representative multi-quadrant envelope (pins shape +
  ordering + fingerprints).
- `--quadrants` filtering (default set, widened set, empty result).
- Bare-array / missing-`schema_version` input rejected with guidance.
- Determinism: same envelope in ⇒ byte-identical SARIF out.
- Path edge cases in `uri` (paths with spaces; confirm no leading slash / `./`).

---

## Appendix A — Standalone GitHub Action (deferred)

**Status: deferred.** Depends on `hc sarif` landing and proving out in this
repo's own CI first. Captured here so the intent isn't lost.

### Idea

Package the pipeline above as a reusable composite action so **any** repo can
get hotspots-as-code-scanning in one step:

```yaml
- uses: will-wright-eng/hc@v1
  with:
    path: .
    since: "6 months"
    quadrants: hot-critical,cold-complex
    category: hc
```

### Sketch

A composite `action.yml` that:

1. Installs `hc` for the runner — either download the matching release binary
   produced by GoReleaser, or `go install github.com/will-wright-eng/hc/cmd/hc@<version>`.
2. Runs `hc analyze --json <path> | hc sarif --quadrants <…> > results.sarif`.
3. Uploads with `github/codeql-action/upload-sarif` (pinned by SHA), passing
   `category`.

Inputs: `path`, `since`, `no-decay`, `quadrants`, `version` (of hc),
`category`, `sarif-file`. Consumers grant `security-events: write` (and
`actions: read` on private repos); document the public-vs-Advanced-Security
distinction.

### Distribution

Prefer shipping `action.yml` at this repo's root so `uses: will-wright-eng/hc@v1`
works directly — one release cadence via release-please + GoReleaser, no second
repo to keep in sync. A separate `hc-action` repo is the alternative but adds
maintenance and version-skew surface.

### Why deferred

- Requires `hc sarif` (Option A) to exist and be stable first.
- Needs a binary-distribution decision (download vs `go install`) and
  cross-OS/arch testing in the action.
- Adds an external-support surface (other repos' CI) that's only worth taking
  on once there's demand and the SARIF output has settled.

Revisit once `hc sarif` has shipped and run against this repo for a few
releases.
