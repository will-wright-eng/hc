# Go Development Tooling Landscape

## Overview

A tiered survey of Go development tools, from standard-library essentials to experimental and emerging options. Organized by maturity and adoption level to help guide toolchain decisions.

---

## Tier 1: Built-in (ships with Go)

These require no installation beyond the Go toolchain itself.

| Tool | Purpose |
|---|---|
| `go build ./...` | Compile check; catches type errors and undefined references |
| `go vet ./...` | Static analysis for common bugs (printf mismatches, unreachable code, struct tag errors) |
| `go test ./...` | Unit tests, benchmarks, fuzzing (`-fuzz`), and coverage (`-cover`) |
| `go fmt` / `gofmt` | Canonical source formatting — non-negotiable in Go culture |
| `go mod tidy` | Prune unused and add missing module dependencies |
| `go doc` | Browse package documentation from the terminal |

**Key flags worth knowing:**

- `go test -race ./...` — enables the data-race detector; should be standard in CI
- `go test -count=1 ./...` — disables test caching for deterministic runs
- `go build -gcflags='-m'` — prints escape analysis decisions (useful for performance work)
- `go vet -json ./...` — machine-readable output for integration with editors and CI

---

## Tier 2: Essential Third-Party

Widely adopted tools that most Go teams consider standard.

### golangci-lint

Meta-linter that runs 50+ analyzers in a single pass. The single most impactful tool beyond what ships with Go.

- Runs `go vet`, `staticcheck`, `errcheck`, `gocritic`, `gosimple`, `unused`, and many more
- Configurable via `.golangci.yml` — enable/disable individual linters, set severity, exclude paths
- Incremental mode: only lint changed files (`--new-from-rev=HEAD~1`)
- Install: `brew install golangci-lint` or `go install github.com/golangci-lint/golangci-lint/cmd/golangci-lint@latest`

### gopls

The official Go language server, maintained by the Go team. Powers IDE features across VS Code, Neovim, GoLand, Emacs, and others.

- Code completion, rename, find references, go-to-definition
- Real-time diagnostics (runs `go vet` and analyzers in the background)
- Workspace-aware: handles multi-module repos
- Install: `go install golang.org/x/tools/gopls@latest`

### staticcheck

The most thorough standalone static analyzer for Go. Included in golangci-lint but also usable independently.

- Checks for deprecated API usage, inefficient patterns, and correctness bugs
- SA-series checks (e.g., `SA1029`: inappropriate context key type) catch subtle issues `go vet` misses
- Install: `go install honnef.co/go/tools/cmd/staticcheck@latest`

### Delve (dlv)

Go-native debugger that understands goroutines, channels, and defer stacks.

- Breakpoints, conditional breakpoints, watchpoints
- Goroutine-aware: switch between goroutines, inspect their stacks
- Integrates with VS Code and GoLand
- Install: `go install github.com/go-delve/delve/cmd/dlv@latest`

---

## Tier 3: Recommended Additions

Tools that add meaningful value but are not universally adopted.

| Tool | Purpose | Install |
|---|---|---|
| **govulncheck** | Scans dependencies for known CVEs using the Go vulnerability database | `go install golang.org/x/vuln/cmd/govulncheck@latest` |
| **goimports** | `gofmt` + automatic import management (adds missing, removes unused) | `go install golang.org/x/tools/cmd/goimports@latest` |
| **goreleaser** | Cross-compile and publish binaries to GitHub Releases, Homebrew, Docker, etc. | `brew install goreleaser` |
| **mockgen** | Generates mock implementations from interfaces (for `gomock`) | `go install go.uber.org/mock/mockgen@latest` |
| **wire** | Compile-time dependency injection via code generation | `go install github.com/google/wire/cmd/wire@latest` |
| **air** | Live-reload for development — watches files and rebuilds on change | `go install github.com/air-verse/air@latest` |
| **ko** | Build and deploy Go containers without a Dockerfile | `go install github.com/google/ko@latest` |

---

## Tier 4: Specialized / Niche

Useful in specific contexts but not part of a general-purpose setup.

| Tool | Purpose |
|---|---|
| **goleak** (`go.uber.org/goleak`) | Detects goroutine leaks in tests |
| **errcheck** | Ensures all returned errors are checked (included in golangci-lint) |
| **goose** / **golang-migrate** | Database migration management |
| **buf** | Protocol buffer tooling (linting, breaking change detection, code generation) |
| **sqlc** | Generates type-safe Go code from SQL queries |
| **pprof** (`go tool pprof`) | CPU/memory/goroutine profiling — built-in but deserves separate mention |
| **trace** (`go tool trace`) | Execution tracer for visualizing goroutine scheduling and latency |

---

## Tier 5: Experimental / Emerging

Tools that are under active development, recently introduced, or exploring new approaches to Go development. These may have unstable APIs or limited adoption.

| Tool | Purpose | Status |
|---|---|---|
| **deadcode** (`golang.org/x/tools/cmd/deadcode`) | Finds unreachable functions using whole-program analysis (call-graph based, more precise than simple linters) | Go team, relatively new |
| **go telemetry** (`gotelemetry`) | Opt-in usage telemetry for Go toolchain — feeds data back to the Go team to prioritize improvements | Shipped in Go 1.23+, controversial |
| **gaby** | AI-powered issue triage and duplicate detection for Go repos (used on the Go issue tracker itself) | Experimental, Go team |
| **oscar** | AI agent for Go project maintenance — auto-labels, classifies, and responds to GitHub issues | Experimental, Go team |
| **go/analysis framework** | Write custom static analyzers as plugins that integrate with `go vet` and golangci-lint | Stable API, underused |
| **rangefunc** iterators | Go 1.23 introduced `iter.Seq` / `iter.Seq2` — still evolving in terms of tooling support and linter coverage | Language feature, tooling catching up |
| **nilaway** (`go.uber.org/nilaway`) | Practical nil-safety analysis via type-theory-based inference across function boundaries | Uber, pre-1.0 |
| **depguard** | Enforce import allow/deny lists (ban certain packages, restrict internal imports) | Included in golangci-lint |
| **goweight** | Analyzes binary size contribution by dependency — useful for trimming bloated builds | Community |

---

## Recommended Setup for This Project

Given `hc` is a focused CLI tool with minimal dependencies:

1. **Editor**: VS Code + Go extension (runs `gopls` automatically) or Neovim + `nvim-lspconfig`
2. **Pre-commit**: Already configured — `go-fmt`, `go-vet`, `go-build`, `golangci-lint`
3. **CI additions to consider**: `go test -race ./...`, `govulncheck ./...`
4. **Profiling** (if needed): `go tool pprof` for identifying bottlenecks in large-repo analysis runs
