# GitHub Releases — Implementation Plan

## Context

`hc` is distributed as a Go binary. Users currently build from source via `make build` or `make install`. Adding automated GitHub releases creates downloadable, pre-built binaries for each tagged version — the standard distribution mechanism for Go CLI tools.

The Go ecosystem has converged on a single tool for this: **GoReleaser**. This document evaluates GoReleaser, describes the release workflow, and specifies the changes needed to wire it up.

---

## GoReleaser Analysis

### What it does

GoReleaser is a release automation tool purpose-built for Go. Given a git tag, it:

1. Cross-compiles the binary for every OS/arch combination specified.
2. Packages binaries into archives (`.tar.gz` for Unix, `.zip` for Windows).
3. Generates SHA256 checksums.
4. Produces a changelog from git commits since the previous tag.
5. Creates a GitHub Release with all artifacts attached.
6. Optionally publishes to Homebrew taps, Snapcraft, Docker registries, etc.

### Why GoReleaser (and not alternatives)

| Approach | Pros | Cons |
|---|---|---|
| **GoReleaser** | De facto Go standard. Declarative config. Cross-compilation, checksums, changelogs, and artifact upload in one tool. GitHub Action maintained by the GoReleaser team. | External dependency (config file + action). Overkill if you only need one platform. |
| **Manual `go build` in CI** | No extra tooling. Full control. | Must script cross-compilation matrix, archive creation, checksum generation, and `gh release create` upload manually. Lots of boilerplate that GoReleaser already handles. |
| **`ko`** | Good for container images. | Designed for containerized Go services, not CLI distribution via GitHub Releases. |
| **`nfpm` / `fpm`** | Produces `.deb`, `.rpm`, `.apk` packages. | Solves a different problem (system packages). GoReleaser can invoke nfpm as a post-build step if we ever need this. |

**Decision:** Use GoReleaser. It is the path of least resistance for Go CLI tools, covers all distribution needs, and avoids reinventing archive/checksum/upload scripting.

### GoReleaser configuration scope

GoReleaser supports dozens of features (Docker images, Homebrew taps, Snapcraft, signing, SBOMs). For `hc`, the initial configuration should be minimal:

- **Builds:** cross-compile for `linux`, `darwin`, `windows` × `amd64`, `arm64`.
- **Archives:** `.tar.gz` default, `.zip` for Windows.
- **Checksums:** SHA256 manifest.
- **Changelog:** auto-generated from commits, filtered to exclude noise (`docs:`, `test:`, `chore:`).
- **ldflags:** inject version and commit hash into the binary.

Everything else (Homebrew, Docker, signing) can be added later if needed.

---

## How It Works

### Release flow

```
PR merged to main
       │
       ▼
Developer pushes a semver tag
  git tag v1.2.0 && git push origin v1.2.0
       │
       ▼
GitHub Actions triggers on tag push (v*)
       │
       ▼
GoReleaser builds, packages, and publishes
       │
       ▼
GitHub Release created with binaries + checksums
```

Tags — not merges — trigger releases. This is the Go convention. It gives explicit control over versioning and avoids releasing on every merge (not every merge is release-worthy).

### Version injection

Add linker flags so the binary knows its own version:

```go
// cmd/hc/main.go
var (
    version = "dev"
    commit  = "none"
)
```

GoReleaser passes these at build time via `-ldflags`:

```
-s -w -X main.version={{.Version}} -X main.commit={{.Commit}}
```

`-s -w` strips debug info and DWARF tables, reducing binary size. The `version` and `commit` vars are populated from the git tag and SHA. Local `make build` produces a binary with `version=dev`, which is correct for development.

The `hc` CLI command should expose this via its `Version` field:

```go
cmd := &cli.Command{
    Name:    "hc",
    Version: version,
    // ...
}
```

This gives users `hc --version` for free via urfave/cli.

---

## Points of Integration

### `.goreleaser.yaml` (new file, repo root)

```yaml
version: 2
builds:
  - main: ./cmd/hc
    binary: hc
    env:
      - CGO_ENABLED=0
    goos:
      - linux
      - darwin
      - windows
    goarch:
      - amd64
      - arm64
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
  sort: asc
  filters:
    exclude:
      - "^docs:"
      - "^test:"
      - "^chore:"
```

`CGO_ENABLED=0` produces fully static binaries — required for reliable cross-compilation and for running on minimal Linux environments (Alpine, scratch containers).

### `.github/workflows/release.yml` (new file)

```yaml
name: Release

on:
  push:
    tags:
      - "v*"

permissions:
  contents: write

jobs:
  release:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

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

`fetch-depth: 0` is required — GoReleaser needs full git history to generate the changelog and detect the previous tag. `GITHUB_TOKEN` is automatically provided by GitHub Actions; no manual secret configuration is needed.

### `.github/workflows/ci.yml` (new file)

CI should run on every push and PR to catch issues before they reach a tag:

```yaml
name: CI

on:
  push:
    branches: [main]
  pull_request:

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod

      - run: go vet ./...

      - run: go test ./...
```

This mirrors `make lint` and `make test`. A separate CI workflow keeps the feedback loop tight on PRs without coupling it to the release process.

### `cmd/hc/main.go` changes

Add version variables and wire them into the CLI command:

```go
var (
    version = "dev"
    commit  = "none"
)

func main() {
    cmd := &cli.Command{
        Name:    "hc",
        Usage:   "Hot/Cold codebase analysis — churn × complexity hotspot matrix",
        Version: version,
        // ... existing commands
    }
}
```

### `Makefile` changes

Update the `build` target to match GoReleaser's ldflags for local development parity:

```makefile
VERSION ?= dev
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
LDFLAGS  = -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT)

build:
 go build -ldflags "$(LDFLAGS)" -o hc $(REPO_ROOT)/cmd/hc
```

Local builds now embed the current commit hash. Tagged builds via GoReleaser use the tag as the version.

---

## File Summary

| File | Action | Purpose |
|---|---|---|
| `.goreleaser.yaml` | Create | GoReleaser build/archive/checksum/changelog config |
| `.github/workflows/release.yml` | Create | Tag-triggered release workflow |
| `.github/workflows/ci.yml` | Create | Push/PR test and lint workflow |
| `cmd/hc/main.go` | Modify | Add `version`/`commit` vars, set `Version` on CLI command |
| `Makefile` | Modify | Add ldflags to `build` target for local version injection |

---

## Risks

### Tag without passing CI

A developer could push a tag on a commit where tests fail, producing a broken release.

**Mitigation:** The CI workflow runs on pushes to `main`, so the most recent main commit should already be green. For stronger guarantees, add a test job to the release workflow that runs before GoReleaser — but this adds latency to every release. Start without it; add if it becomes a problem.

### GoReleaser version drift

GoReleaser v2 introduced breaking config changes from v1. Future major versions may do the same.

**Mitigation:** Pin `version: 2` in `.goreleaser.yaml`. The GitHub Action (`goreleaser-action@v6`) follows semver — breaking changes require a major action version bump, which must be explicitly adopted.

### Missing `fetch-depth: 0`

If `fetch-depth` is omitted or set to 1 (the default), GoReleaser cannot compute the changelog or detect the previous tag. It will either fail or produce an empty changelog.

**Mitigation:** Explicitly set `fetch-depth: 0` in the checkout step and add a comment explaining why.

### Platform-specific build failures

`CGO_ENABLED=0` avoids most cross-compilation issues, but platform-specific code paths (e.g., `syscall` usage) could still break on certain targets.

**Mitigation:** `hc` uses only stdlib and `urfave/cli` — neither requires CGO or platform-specific syscalls. This is a non-issue for the current codebase but worth noting if dependencies change.
