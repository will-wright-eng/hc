// Package app orchestrates the analyze pipeline: path resolution, git history
// extraction, complexity scanning, classification, and subtree filtering. It
// is the layer between the CLI (cmd/hc) and the analysis primitives, and the
// place to put orchestration concerns that don't belong in any single
// internal package.
package app

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/will-wright-eng/hc/internal/analysis"
	"github.com/will-wright-eng/hc/internal/complexity"
	gitpkg "github.com/will-wright-eng/hc/internal/git"
	"github.com/will-wright-eng/hc/internal/ignore"
)

// File age floor: files younger than DefaultMinAge are excluded from analysis.
// Auto-disables on narrow --since windows; opt out via NoMinAge.
const (
	DefaultMinAge     = 14 * 24 * time.Hour
	autoDisableMinAge = 30 * 24 * time.Hour
)

// AnalyzeOptions captures the inputs needed to run the analyze pipeline. The
// CLI builds this from flags; tests build it directly.
type AnalyzeOptions struct {
	// Path is the user-supplied target. May be the repo root, a subdirectory,
	// or a file path. Must resolve to a location inside a git repository.
	Path string
	// Since is the git --since window (e.g. "6 months"). Empty means full history.
	Since string
	// Excludes is the list of additional ignore patterns from --exclude flags.
	// Combined with patterns from <repoRoot>/.hcignore.
	Excludes []string
	// Decay enables recency weighting in churn computation.
	Decay bool
	// NoMinAge forces the file age floor off regardless of Since.
	NoMinAge bool
	// FilesFrom is the projection filter: when non-empty, results are restricted
	// to these repo-root-relative paths. Classification thresholds are still
	// computed across the full corpus — only the output rows shrink. Paths not
	// found in the analyzed tree are silently dropped.
	FilesFrom []string
	// Now is the reference time for time-sensitive analysis. Zero means time.Now().
	Now time.Time
}

// AnalyzeResult is everything the caller needs to render output and report on
// the run. RepoRoot and Subtree are exposed for diagnostics; Decay is echoed
// back so output formatters know whether to render the SCORE column.
// ChurnThreshold and ComplexityThreshold are the median-split values used to
// classify files — surfaced so the JSON envelope can record them.
type AnalyzeResult struct {
	Files               []analysis.FileScore
	RepoRoot            string
	Subtree             string // relative to RepoRoot, "" when analyzing root
	Decay               bool
	AutoDisabledMinAge  bool
	ChurnThreshold      float64
	ComplexityThreshold int
	MinAge              time.Duration
}

// Analyze runs the full analyze pipeline. Paths in the result are always
// repo-root-relative regardless of where Path points. When Path is a
// subdirectory, results are filtered to that subtree but classification
// thresholds are computed across the whole repo.
//
// ctx cancels the underlying git invocations. Pass context.Background() if
// you don't need cancellation.
func Analyze(ctx context.Context, opts AnalyzeOptions) (AnalyzeResult, error) {
	absPath, err := filepath.Abs(opts.Path)
	if err != nil {
		return AnalyzeResult{}, fmt.Errorf("resolving path: %w", err)
	}

	repoRoot, err := gitpkg.RepoRoot(ctx, absPath)
	if err != nil {
		return AnalyzeResult{}, err
	}

	subtree, err := relSubtree(repoRoot, absPath)
	if err != nil {
		return AnalyzeResult{}, err
	}

	patterns, err := ignore.LoadFile(filepath.Join(repoRoot, ".hcignore"))
	if err != nil {
		return AnalyzeResult{}, fmt.Errorf("reading .hcignore: %w", err)
	}
	patterns = append(patterns, opts.Excludes...)
	ig := ignore.New(patterns)

	churns, err := gitpkg.LogWithOptions(ctx, gitpkg.LogOptions{
		RepoPath: repoRoot,
		Since:    opts.Since,
		Ignore:   ig,
		Decay:    opts.Decay,
		Now:      opts.Now,
	})
	if err != nil {
		return AnalyzeResult{}, fmt.Errorf("reading git history: %w", err)
	}

	complexities, err := complexity.WalkWithOptions(repoRoot, complexity.Options{Ignore: ig})
	if err != nil {
		return AnalyzeResult{}, fmt.Errorf("analyzing file complexity: %w", err)
	}

	minAge, autoDisabled := EffectiveMinAge(opts.NoMinAge, opts.Since)
	res := analysis.AnalyzeWithOptions(churns, complexities, analysis.Options{
		MinAge: minAge,
		Now:    opts.Now,
	})
	scores := res.Files

	if subtree != "" {
		scores = filterToSubtree(scores, subtree)
	}

	if len(opts.FilesFrom) > 0 {
		scores = filterToFiles(scores, normalizeFilesFrom(opts.FilesFrom, repoRoot))
	}

	return AnalyzeResult{
		Files:               scores,
		RepoRoot:            repoRoot,
		Subtree:             subtree,
		Decay:               opts.Decay,
		AutoDisabledMinAge:  autoDisabled,
		ChurnThreshold:      res.ChurnThreshold,
		ComplexityThreshold: res.ComplexityThreshold,
		MinAge:              minAge,
	}, nil
}

// relSubtree computes the repo-root-relative form of absPath. Returns "" when
// absPath is the repo root itself. Errors when absPath escapes the repo.
func relSubtree(repoRoot, absPath string) (string, error) {
	rel, err := filepath.Rel(repoRoot, absPath)
	if err != nil {
		return "", fmt.Errorf("computing subtree: %w", err)
	}
	rel = filepath.ToSlash(rel)
	if rel == "." {
		return "", nil
	}
	if rel == ".." || strings.HasPrefix(rel, "../") {
		return "", fmt.Errorf("path %s is outside the git repository at %s", absPath, repoRoot)
	}
	return rel, nil
}

// normalizeFilesFrom converts the raw --files-from input into a set of
// repo-root-relative forward-slash paths matching the form internal/git and
// internal/complexity use. Accepts ./foo.go, foo.go, and absolute paths inside
// the repo. Blank entries and paths outside the repo are dropped.
func normalizeFilesFrom(raw []string, repoRoot string) map[string]struct{} {
	out := make(map[string]struct{}, len(raw))
	for _, p := range raw {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if filepath.IsAbs(p) {
			rel, err := filepath.Rel(repoRoot, p)
			if err != nil {
				continue
			}
			p = rel
		}
		p = filepath.ToSlash(filepath.Clean(p))
		if p == "." || p == ".." || strings.HasPrefix(p, "../") {
			continue
		}
		out[p] = struct{}{}
	}
	return out
}

// filterToFiles keeps scores whose path is in the given set. An empty set is a
// no-op (caller is responsible for not calling with an empty filter when no
// filter was requested).
func filterToFiles(scores []analysis.FileScore, want map[string]struct{}) []analysis.FileScore {
	out := scores[:0]
	for _, s := range scores {
		if _, ok := want[s.Path]; ok {
			out = append(out, s)
		}
	}
	return out
}

// filterToSubtree keeps scores whose path equals subtree or is nested under it.
// Subtree must be a forward-slash, repo-root-relative path with no trailing slash.
func filterToSubtree(scores []analysis.FileScore, subtree string) []analysis.FileScore {
	prefix := subtree + "/"
	out := scores[:0]
	for _, s := range scores {
		if s.Path == subtree || strings.HasPrefix(s.Path, prefix) {
			out = append(out, s)
		}
	}
	return out
}

// EffectiveMinAge resolves the file age floor for an analyze run. Returns the
// duration to apply (zero means disabled) and whether the auto-disable rule
// fired so the caller can surface a note.
//
// Rules: NoMinAge forces zero. Otherwise, if since parses to a window at or
// below autoDisableMinAge, the floor disables. Unparseable since values leave
// the floor on — see docs/proposals/file-age-floor.md.
func EffectiveMinAge(noMinAge bool, since string) (time.Duration, bool) {
	if noMinAge {
		return 0, false
	}
	if since == "" {
		return DefaultMinAge, false
	}
	days, err := gitpkg.ParseHalfLife(since)
	if err != nil || days <= 0 {
		return DefaultMinAge, false
	}
	window := time.Duration(days * 24 * float64(time.Hour))
	if window <= autoDisableMinAge {
		return 0, true
	}
	return DefaultMinAge, false
}
