# 011 — Repo Best-Practices Audit

**Date:** 2026-07-02
**Scope:** repository state at v1.3.0 (commit `c2cd6b4`)

## Summary

Overall, `hc` is already above the bar for a small OSS tool: SHA-pinned actions with zizmor enforcement, Dependabot with cooldowns, CODEOWNERS, `exec.CommandContext` throughout, a single dependency, and 71–91% test coverage in the logic packages. The gaps cluster in two places: **CI doesn't enforce things the repo already has**, and **the release chain has no integrity story beyond an unsigned checksums file**.

## Tier 1 — CI enforcement (cheap, high value)

1. **Lint exists but never runs in CI.** The Makefile has `lint: go vet ./...`, but `ci.yml` only runs `make test`. A PR that fails `go vet` merges green today. Add a lint job — and since it means touching CI anyway, `golangci-lint` (staticcheck, errcheck, govet in one runner) is the community standard and catches a much wider class of issues than vet alone.
2. **The ruleset requires review but not green CI.** The `main` ruleset enforces PR + 1 approval, squash-only, signatures, no force-push — but has no `required_status_checks` rule. CI is advisory. Requiring the `go test` and zizmor checks closes that.
3. **No race detector.** `go test -race` in CI is standard, and it's not hypothetical here — the git-log threading work (#43) means `internal/git` has real concurrency.
4. **Platforms are shipped that CI never builds.** goreleaser cuts windows/darwin × amd64/arm64, but CI is ubuntu-only. A tool that shells out to git and manipulates paths is exactly the kind that breaks on Windows first. A cross-compile check (`GOOS=windows go build ./...`) is nearly free; a small OS test matrix is better.
5. **Release config is only validated at release time.** The repo's own history proves the cost — two `fix(release)` commits (#41, #44) for issues that a `goreleaser check` + `goreleaser release --snapshot --clean` CI step would have caught pre-merge.
6. **`make e2e` exists but CI never runs it.** Building `hc` and piping `analyze --json | md report` over the repo itself is a free end-to-end smoke test.

## Tier 2 — Release and supply chain

1. **`go install` builds report `dev (none)`.** The README's primary install path bypasses the goreleaser ldflags entirely. The fix is the standard `runtime/debug.ReadBuildInfo()` fallback in `main.go` — module-installed builds then report their real version.
2. **No provenance or signing.** `checksums.txt` verifies transfer integrity but not origin — an attacker who can serve the tarball can serve the checksums too. GitHub's `actions/attest-build-provenance` is about ten lines in `release.yml`; goreleaser's cosign support is the alternative.
3. **No published container image.** Adding a `dockers:` (or `kos:`) section to `.goreleaser.yaml` publishing to ghcr gives consumers digest-pinning — this is the piece that would let a consumer like Themis use its existing `COPY --from=<image>@sha256:...` idiom.
4. **Workflow-level permissions are broader than either job needs.** `release.yml` grants `contents: write` + `pull-requests: write` to both jobs; only release-please needs the PR scope. Moving permissions to the job level is least-privilege housekeeping (and the kind of thing zizmor increasingly flags).

## Tier 3 — Hygiene

1. **SECURITY.md** — the repo distributes binaries publicly; a disclosure path is expected.
2. **`readme.md` → `README.md`** — purely conventional, everything tolerates lowercase.
3. **`cmd/hc` sits at 0% coverage** — acceptable since it's thin flag wiring over `internal/app` (78%); just keep logic out of it.

## Suggested sequencing

Items 1, 2, and 5 in one PR (CI enforces what exists), 7 and 8 in a second (release integrity), then 9 when downstream consumption (e.g. Themis) becomes real.

## References

- [golangci-lint](https://golangci-lint.run/) — aggregated linter runner
- [govulncheck](https://go.dev/blog/vuln) — call-graph-aware vulnerability scanning, worth adding alongside lint
- [GitHub artifact attestations](https://docs.github.com/en/actions/security-for-github-actions/using-artifact-attestations/using-artifact-attestations-to-establish-provenance-for-builds)
- [goreleaser Docker support](https://goreleaser.com/customization/docker/) and [`goreleaser check`](https://goreleaser.com/cmd/goreleaser_check/)
- [Go version via ReadBuildInfo](https://pkg.go.dev/runtime/debug#ReadBuildInfo)
