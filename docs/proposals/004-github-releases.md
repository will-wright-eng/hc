# GitHub Releases — Implementation Plan

## Context

`hc` is distributed as a Go binary. Users currently build from source via `make build` or `make install`. Adding automated GitHub releases produces downloadable pre-built binaries for each tagged version — the standard distribution mechanism for Go CLI tools.

This plan wires up two complementary tools:

- **release-please** decides *when* to release and *what version*. It reads conventional commits, maintains a long-lived "Release PR" that bumps the version and updates `CHANGELOG.md`, and creates the tag + GitHub Release when that PR is merged.
- **GoReleaser** decides *what to ship*. Triggered by the release, it cross-compiles, archives, checksums, and uploads binaries to the release that release-please already created.

Together they cover the whole pipeline with zero manual tagging or changelog writing.

## Current state (2026-05-10)

- `.github/workflows/ci.yml` exists — runs `make test` on push/PR/`workflow_dispatch`. No release workflow yet.
- Other existing workflows: `hotspots.yml`, `pr-file-comments.yml` (orthogonal to release; ignore for this plan).
- `Makefile` `build` target is plain `go build` with no ldflags.
- `cmd/hc/main.go` has no `version`/`commit` vars and the CLI command has no `Version` field.
- No `.goreleaser.yaml`, no release-please config, no git tags pushed yet.
- Commits on `main` are **not** currently in conventional-commit format consistently — adopting release-please means committing to that style going forward.

## Why these two tools

| Tool | Owns | Why it (and not alternatives) |
|---|---|---|
| **release-please** | Version bumps, `CHANGELOG.md`, tag creation, GitHub Release creation | Google-maintained. PR-driven (you review the bump before it ships). Reads conventional commits, so semver is automatic. Alternative `semantic-release` is Node-centric; release-please has first-class Go support. Alternative manual tagging is fine but forces humans to remember semver and write changelogs. |
| **GoReleaser** | Cross-compilation, archives, checksums, binary upload | De facto Go standard. One declarative config covers what would otherwise be ~100 lines of matrix YAML + `gh release create` scripting. Alternatives: `ko` (wrong shape — containers, not CLIs), `nfpm` (system packages — different problem, can be invoked by GoReleaser later). |

**Initial scope is intentionally minimal:** GoReleaser handles only builds/archives/checksums/changelog-from-commits/ldflags. Defer Homebrew taps, Docker images, signing, SBOMs until there's demand.

## Release flow

```
Developer merges PRs to main using conventional-commit messages
       │
       ▼
release-please workflow runs on every push to main
       │
       ├─ Opens or updates a long-lived "Release PR"
       │  (bumps version in manifest, regenerates CHANGELOG.md)
       │
       ▼
Maintainer reviews + merges the Release PR
       │
       ▼
release-please creates the git tag (vX.Y.Z) and a draft GitHub Release
       │
       ▼
Same workflow's next job sees releases_created=true → runs GoReleaser
       │
       ▼
GoReleaser cross-compiles, archives, checksums, and uploads binaries
to the release release-please just created
```

Conventional commit prefixes drive the bump:

- `fix:` → patch (`v1.2.3` → `v1.2.4`)
- `feat:` → minor (`v1.2.3` → `v1.3.0`)
- `feat!:` / `BREAKING CHANGE:` footer → major
- `docs:`, `test:`, `chore:`, `refactor:` → no release

## Version injection

Embed the version in the binary so `hc --version` is honest in both release builds and local dev builds.

```go
// cmd/hc/main.go
var (
    version = "dev"
    commit  = "none"
)

func main() {
    cmd := &cli.Command{
        Name:    "hc",
        Version: version,
        // ...
    }
}
```

GoReleaser passes the tag and SHA via `-ldflags` at build time. The `Makefile` mirrors this so local builds embed the current commit. `-s -w` strips debug info and DWARF tables, shrinking the binary.

Note: release-please can also update a version constant in source, but with ldflags injection that's redundant — the manifest file is the single source of truth for "what version are we on."

## Changes required

### 1. `.goreleaser.yaml` (new, repo root)

```yaml
version: 2
builds:
  - main: ./cmd/hc
    binary: hc
    env:
      - CGO_ENABLED=0
    goos: [linux, darwin, windows]
    goarch: [amd64, arm64]
    ldflags:
      - -s -w -X main.version={{.Version}} -X main.commit={{.Commit}}

archives:
  - format: tar.gz
    name_template: "{{ .ProjectName }}_{{ .Os }}_{{ .Arch }}"
    format_overrides:
      - goos: windows
        format: zip

checksum:
  name_template: checksums.txt

changelog:
  disable: true  # release-please owns the changelog
```

`CGO_ENABLED=0` produces fully static binaries — required for reliable cross-compilation and for running on minimal Linux environments. `changelog.disable: true` because release-please writes `CHANGELOG.md` and the GitHub Release body; letting GoReleaser also generate one would duplicate the content.

### 2. `release-please-config.json` (new, repo root)

```json
{
  "release-type": "go",
  "packages": {
    ".": {
      "package-name": "hc",
      "include-component-in-tag": false
    }
  },
  "changelog-sections": [
    { "type": "feat", "section": "Features" },
    { "type": "fix",  "section": "Bug Fixes" },
    { "type": "perf", "section": "Performance" },
    { "type": "docs", "section": "Documentation", "hidden": true },
    { "type": "test", "section": "Tests",         "hidden": true },
    { "type": "chore","section": "Chores",        "hidden": true }
  ]
}
```

### 3. `.release-please-manifest.json` (new, repo root)

```json
{ ".": "0.0.0" }
```

Seed value. release-please reads this on each run to know the current version; the Release PR updates it.

### 4. `.github/workflows/release.yml` (new)

Single workflow: release-please runs first, GoReleaser runs only if a release was just created.

```yaml
name: release

on:
  push:
    branches: [main]

permissions:
  contents: write
  pull-requests: write

jobs:
  release-please:
    runs-on: ubuntu-latest
    outputs:
      releases_created: ${{ steps.rp.outputs.releases_created }}
      tag_name:         ${{ steps.rp.outputs.tag_name }}
    steps:
      - uses: googleapis/release-please-action@v4
        id: rp
        with:
          config-file: release-please-config.json
          manifest-file: .release-please-manifest.json

  goreleaser:
    needs: release-please
    if: needs.release-please.outputs.releases_created == 'true'
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0  # GoReleaser needs full history
          ref: ${{ needs.release-please.outputs.tag_name }}

      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod

      - uses: goreleaser/goreleaser-action@v6
        with:
          version: latest
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

`GITHUB_TOKEN` is provided automatically — no secret setup. The `pull-requests: write` permission lets release-please open the Release PR; `contents: write` lets it create the tag and lets GoReleaser upload artifacts.

### 5. `cmd/hc/main.go` (modify)

Add the `version`/`commit` vars at package scope and set `Version: version` on the CLI command. Urfave/cli wires `--version` automatically.

### 6. `Makefile` (modify `build` target)

```makefile
VERSION ?= dev
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
LDFLAGS  = -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT)

build: ## build the hc binary
	go build -ldflags "$(LDFLAGS)" -o hc $(REPO_ROOT)/cmd/hc
```

Local builds get the short SHA; tagged builds via GoReleaser get the tag.

### 7. `.github/workflows/ci.yml` (optional tweak)

Current CI runs `make test` only. Consider adding `make lint` (go vet) so PRs catch issues that would otherwise reach a release. Not strictly required.

## File summary

| File | Action | Purpose |
|---|---|---|
| `.goreleaser.yaml` | Create | Build/archive/checksum/ldflags config |
| `release-please-config.json` | Create | release-please behavior (release type, changelog sections) |
| `.release-please-manifest.json` | Create | Current-version tracker (seed `0.0.0`) |
| `.github/workflows/release.yml` | Create | release-please → GoReleaser pipeline |
| `cmd/hc/main.go` | Modify | Add `version`/`commit` vars; set `Version` on CLI |
| `Makefile` | Modify | Add ldflags to `build` for local version injection |
| `.github/workflows/ci.yml` | Optional | Add `make lint` step |

## Cutting the first release

1. Merge the changes above to `main` (use a conventional-commit message, e.g. `feat: add automated release pipeline`).
2. The `release` workflow runs and opens a Release PR titled something like `chore(main): release 0.1.0`.
3. Verify GoReleaser config locally: `goreleaser release --snapshot --clean` (dry run, no upload).
4. Review and merge the Release PR.
5. The workflow re-runs: release-please tags `v0.1.0` and creates the GitHub Release, then GoReleaser uploads binaries to it.
6. Confirm artifacts appear under the GitHub Releases page.

Subsequent releases need no manual action beyond merging conventional-commit PRs into main and then merging the rolling Release PR when you're ready to ship.

## Rollback

If a bad release ships:

1. Delete the GitHub Release (UI or `gh release delete vX.Y.Z`).
2. Delete the tag locally and remotely: `git tag -d vX.Y.Z && git push --delete origin vX.Y.Z`.
3. Revert the manifest bump on `main` (or land a `fix:` and let release-please bump to the next patch).

## Risks

### Conventional commits adoption

release-please does nothing useful if commits aren't formatted as `type: subject`. A single non-conforming squash-merge title means a missed bump.

**Mitigation:** Enforce PR title format with a small action (e.g. `amannn/action-semantic-pull-request`). Default to squash-merge so PR title becomes the commit message — one chokepoint to police instead of every commit.

### Release PR sits stale

If nobody merges the Release PR, conventional commits keep piling up and the PR keeps re-updating. Not broken, just delayed shipping.

**Mitigation:** Treat the Release PR as a routine maintenance task. Optionally add a cron/Slack reminder if it goes >N days without a merge.

### GoReleaser config drift

GoReleaser v2 introduced breaking changes from v1. Future majors may do the same.

**Mitigation:** Pin `version: 2` in `.goreleaser.yaml`. The action (`goreleaser-action@v6`) is semver — breaking changes require an explicit major bump.

### Missing `fetch-depth: 0`

Default checkout depth (1) starves GoReleaser of the history it needs for version detection. Result: failed run.

**Mitigation:** Explicit `fetch-depth: 0` in the GoReleaser job's checkout step (already in the workflow above).

### Tag without passing CI

A maintainer could merge the Release PR while CI is red.

**Mitigation:** Branch protection on `main` requiring `ci` to pass. The Release PR is just another PR — same gates apply.

### Platform-specific build failures

`CGO_ENABLED=0` avoids most cross-compilation pain, but platform-specific code (`syscall`, cgo-using deps) could still break a target.

**Mitigation:** `hc` currently uses only stdlib + `urfave/cli`. Non-issue today; revisit if dependencies change.
