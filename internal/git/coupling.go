package git

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/will-wright-eng/hc/internal/ignore"
)

// IgnoreRevsFile is the standard filename git and GitHub honor for
// `git blame --ignore-revs-file`. Commits listed there are treated as noise
// (mass renames, format-everything commits) and skipped during coupling pair
// extraction. Churn counting is unaffected.
const IgnoreRevsFile = ".git-blame-ignore-revs"

// CouplingPair is the co-change aggregate for an unordered file pair.
// A < B lexicographically. Support is the raw co-change commit count;
// WeightedSupport applies the same decay weighting as churn (equal to Support
// when decay is off). See docs/proposals/012-change-coupling.md.
type CouplingPair struct {
	A               string
	B               string
	Support         int
	WeightedSupport float64
}

// LogResult bundles per-file churn with coupling pairs extracted from the
// same git log pass.
type LogResult struct {
	Churn []FileChurn
	Pairs []CouplingPair
}

// LogWithCoupling is LogWithOptions plus change-coupling pair extraction over
// the same history pass — no extra git invocation. Commits listed in the repo
// root's .git-blame-ignore-revs are skipped for pairs only.
func LogWithCoupling(ctx context.Context, opts LogOptions) (LogResult, error) {
	return logWithOptions(ctx, opts, true)
}

// LoadIgnoreRevs reads <repoRoot>/.git-blame-ignore-revs into a set of
// lowercase commit SHAs. A missing file yields an empty set. Blank lines and
// `#` comments are skipped, as are lines that aren't full 40/64-char hex
// object names — hc is best-effort where git blame would error.
func LoadIgnoreRevs(repoRoot string) (map[string]struct{}, error) {
	data, err := os.ReadFile(filepath.Join(repoRoot, IgnoreRevsFile))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", IgnoreRevsFile, err)
	}
	revs := make(map[string]struct{})
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || !isHexObjectName(line) {
			continue
		}
		revs[strings.ToLower(line)] = struct{}{}
	}
	return revs, nil
}

func isHexObjectName(s string) bool {
	if len(s) != 40 && len(s) != 64 {
		return false
	}
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9', r >= 'a' && r <= 'f', r >= 'A' && r <= 'F':
		default:
			return false
		}
	}
	return true
}

// extractPairs aggregates unordered co-change pairs across commits. Paths are
// rename-resolved before pairing so a pair survives renames; the resulting
// per-commit set also collapses self-pairs (two old paths of the same file).
// Ignored paths never enter a pair. Commits in ignoreRevs are skipped.
func extractPairs(commits []commitInfo, renames RenameMap, ig *ignore.Matcher, ignoreRevs map[string]struct{}, now time.Time, halfLifeDays float64) []CouplingPair {
	type pairStats struct {
		support         int
		weightedSupport float64
	}
	stats := make(map[[2]string]*pairStats)

	for _, ci := range commits {
		if _, skip := ignoreRevs[strings.ToLower(ci.SHA)]; skip {
			continue
		}
		set := make(map[string]struct{}, len(ci.Files))
		for _, f := range ci.Files {
			if f == "" {
				continue
			}
			resolved := renames.Resolve(f)
			if ig.Match(resolved) {
				continue
			}
			set[resolved] = struct{}{}
		}
		if len(set) < 2 {
			continue
		}
		paths := make([]string, 0, len(set))
		for p := range set {
			paths = append(paths, p)
		}
		sort.Strings(paths)

		w := DecayWeight(ci.Date, now, halfLifeDays)
		for i := 0; i < len(paths); i++ {
			for j := i + 1; j < len(paths); j++ {
				k := [2]string{paths[i], paths[j]}
				s, ok := stats[k]
				if !ok {
					s = &pairStats{}
					stats[k] = s
				}
				s.support++
				s.weightedSupport += w
			}
		}
	}

	pairs := make([]CouplingPair, 0, len(stats))
	for k, s := range stats {
		pairs = append(pairs, CouplingPair{A: k[0], B: k[1], Support: s.support, WeightedSupport: s.weightedSupport})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].A != pairs[j].A {
			return pairs[i].A < pairs[j].A
		}
		return pairs[i].B < pairs[j].B
	})
	return pairs
}
