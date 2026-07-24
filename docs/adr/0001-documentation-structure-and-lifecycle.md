# 0001. Documentation structure and lifecycle

## Context

The `docs/` tree grew organically to ~15 documents across three subtrees
(`design/`, `info/`, `proposals/`) with no stated rules. `docs/proposals/` was
numbered with gaps left by deleted documents (`002`, `004`–`007`, `009`) but no
rule saying whether gaps are legal or numbers reusable. `docs/design/` mixed
the living reference (`overview.md`, née `002-design.md`) with two documents
literally titled "— Proposal". Status lived in free-prose `> **Status:** …`
blockquotes or per-item ✅/❌ tables, when it was recorded at all. One deleted
proposal (`file-age-floor.md`) left dangling citations in `CLAUDE.md`,
`AGENTS.md`, and `internal/app/app.go`. Shipped proposals (indentation
complexity, pipeline polish, PR annotations) kept aging as plans with no
durable, citable record of the decisions inside them.

## Decision

Adopt a genre→home model, a controlled document lifecycle, one file-naming and
numbering scheme per home, and required YAML front matter on `docs/proposals/`
documents. Record shipped architectural decisions as **Architecture Decision
Records (ADRs)** under `docs/adr/` that *distill* their source proposals into
the single citable authority.

An ADR is **immutable** and changed only by a superseding ADR (never edited in
substance). ADRs carry **no front matter**: presence in `docs/adr/` means the
decision shipped and is prescriptive. Once an ADR fully distills a source
document's decision, that source is deleted after its inbound citations are
repointed to the ADR; until then it stays in place, marked superseded.

## Rules

### 1. Where documents live

| Genre | Home | Role |
| --- | --- | --- |
| Canonical reference | `docs/design/` | living "how it works now"; updated in the same change as behavior |
| **Decision record (ADR)** | **`docs/adr/`** | one shipped decision + why; the citable authority |
| Design / proposal / RFC | `docs/proposals/` | a change proposed or in flight |
| Implementation plan | `docs/proposals/` | phased execution of an accepted decision |
| Analysis / backlog | `docs/proposals/` | point-in-time or living working records |
| Research / background | `docs/info/` | reference material and prior art; never prescriptive |

A "proposal" and an "RFC" are not separate genres — they are a `design` or
`implementation-plan` document with `status: proposed`. The lifecycle field,
not the type field, carries that distinction (§4).

### 2. File naming and numbering

- **kebab-case** for every file (`lower-case-with-hyphens.md`).
- **`docs/adr/`** — `NNNN-slug.md`, four-digit, strictly sequential
  (scan the highest existing number and add one). H1: `# NNNN. Title`.
- **`docs/proposals/`** — `NNN-slug.md`, three-digit. H1: `# NNN — Title`, and
  the number in the H1 **must** equal the filename number.
- **`docs/info/`** — `NNN-slug.md`, three-digit, same append-only rule as
  proposals. H1 descriptive; no number required.
- **`docs/design/`** — topic slug, no number (`overview.md`). H1 descriptive.
- The four-digit ADR ids stay visually distinct from three-digit proposal ids
  so `docs/adr/0002` and `docs/proposals/010` are never confused in a citation.

### 3. Number stability (`docs/proposals/`, `docs/info/`)

Numbers are permanent identifiers: **append-only, never reused, gaps
expected.** A new document takes the next unused number. A deleted document's
number is retired, not recycled — the gap is the record that something was
removed (the existing gaps `002`, `004`–`007`, `009` are all real deletions).
Two documents must never share a number. Numbers freed by moving a document
*out* of a tree (e.g. the old `design/001`–`004`) are retired the same way.

### 4. Document lifecycle (`docs/proposals/`)

```text
proposed ──► accepted ──► in-progress ──► shipped
                                             │
                                             ├─► superseded   (replaced; see superseded-by / distilled-to)
                                             ├─► deprecated   (no longer the approach, no direct replacement)
                                             └─► historical   (kept as reference for a retired surface)
```

- The line above is the progression for `design` and `implementation-plan`.
  Non-progressing genres — `analysis`, `backlog` — do not traverse it; they are
  `current` while the record is live and valid, `superseded` once another
  document replaces them, `historical` once their subject is retired.
- A decision becomes **ADR-eligible only at `shipped`** (§5). `proposed` /
  `accepted` designs are not distilled into ADRs.
- Deleting a document that still has inbound citations is forbidden — that is
  how the `file-age-floor.md` dangling links happened. Repoint or restore
  first; the lifecycle statuses exist so retirement never requires deletion.

### 5. Front matter — `docs/proposals/` only

Every `docs/proposals/` document carries a YAML front-matter block. `docs/adr/`,
`docs/design/`, and `docs/info/` documents do **not** (an ADR's state is
implicit; a design doc is always-current by definition; info docs are inert
reference). Schema:

```yaml
---
type: design | implementation-plan | analysis | backlog
status: proposed | accepted | in-progress | shipped | current | superseded | deprecated | historical
created: 2026-04-27          # ISO date first authored
updated: 2026-04-27          # optional; ISO date of a material revision
pin: c2cd6b4                 # optional on any type, required for `analysis`; the commit findings were verified against
supersedes: [008]            # optional; whole proposal doc(s) this replaces (may name a retired number, §3)
superseded-by: 014           # optional; whole proposal doc that replaced this
distilled-to: [0002, 0003]   # optional; ADR(s) that now carry this doc's decision(s)
---
```

**Allowed `status` by `type`.** The lifecycle line in §4 applies only to genres
that ship code or artifacts; the rest are living-or-retired:

| type | allowed `status` |
| --- | --- |
| `design` | `proposed`, `accepted`, `in-progress`, `shipped`, `superseded`, `deprecated`, `historical` |
| `implementation-plan` | `proposed`, `accepted`, `in-progress`, `shipped`, `superseded`, `historical` |
| `analysis` | `current`, `superseded`, `historical` |
| `backlog` | `current`, `historical` |

`current` means the record is live and accurate (at its `pin`, for an
`analysis`); `shipped` is reserved for a `design` or `implementation-plan`
whose code or artifacts landed.

Required: `type`, `status`, `created`. All proposals were back-filled when this
convention landed, so front matter is required on every `docs/proposals/`
document from here on — there is no grandfathered remainder.

### 6. ADR conventions (`docs/adr/`)

- **One decision, distilled.** An ADR records a single shipped decision and its
  rationale, and links to the source proposal(s) it distills.
- **Template** (sections earn their place; most ADRs stop after Consequences):

  ```md
  # NNNN. Imperative decision title

  ## Context
  2–4 sentences: the forces that forced the decision.

  ## Decision
  1–3 sentences: what we decided.

  ## Rules            ← only when code or docs cite this ADR
  The condensed, enforceable prescriptions future changes must follow.

  ## Consequences
  Non-obvious downstream effects; the notable alternative(s) rejected and why.

  ## References
  - Distilled from: [proposals/010](../proposals/010-….md) §3
  - Related: [0003](./0003-….md)
  ```

- **Immutable + supersede.** Never edit an accepted ADR's substance. A material
  change to its rules is a *new* ADR that lists `Supersedes: [NNNN]` in its
  `## References`; the retired ADR gets a correction-grade banner under its
  title: `> Superseded by [0007-…](./0007-….md)`. Typo/link fixes and adding
  that banner are the only permitted edits.
- **The ADR is the primary reference.** In-code and cross-document citations
  point at `docs/adr/NNNN`. When a shipped proposal is fully distilled, it is
  deleted *after* its inbound citations are repointed to the ADR; until then it
  stays with `status: superseded` and `distilled-to: [NNNN]`.

## Consequences

- `docs/design/` shrinks to the living reference (`overview.md`); the two
  proposals misfiled there were renumbered into `docs/proposals/` (`013`,
  `014`) with inbound links repointed, and the background research doc moved to
  `docs/info/006`. The old `design/`-side numbers are retired (§3).
- `file-age-floor.md` was restored as `proposals/015` and its dangling
  citations (`CLAUDE.md`, `AGENTS.md`, `internal/app/app.go`) repointed —
  deletion-with-live-citations is exactly the failure mode §4 now forbids.
- Immutable ADRs mean a convention or architecture change costs a superseding
  ADR plus a citation repoint. Accepted — these rules change rarely, and the
  audit trail ("what was in force when, and why it changed") is the payoff.
- The distillation backfill — the shipped decisions inside `001` (indent-sum
  complexity), `008` (JSON envelope contract), `010` (workflow-command
  annotations), and `015` (file age floor) — is follow-up work, drafted one ADR
  at a time against this template. Authoring an ADR and repointing citations to
  it remain separate changes.
- `docs/info/` H1s stay unnumbered and untouched; conformance there is
  filename-level only.

## References

- [`AGENTS.md`](../../AGENTS.md) / [`CLAUDE.md`](../../CLAUDE.md) —
  Documentation section (added in the same change as this ADR)
- [`docs/design/overview.md`](../design/overview.md) — the canonical reference
- [`docs/proposals/`](../proposals/) — the documents this convention governs
- [`docs/info/`](../info/) — background research and prior art
- Michael Nygard, [Documenting Architecture Decisions](https://cognitect.com/blog/2011/11/15/documenting-architecture-decisions)
