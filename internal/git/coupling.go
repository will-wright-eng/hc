package git

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/will-wright-eng/hc/internal/ignore"
)

// ignoreRevsFile is the standard filename git and GitHub honor for
// `git blame --ignore-revs-file`. Commits listed there are treated as noise
// (mass renames, format-everything commits) and skipped during coupling pair
// extraction. Churn counting is unaffected.
const ignoreRevsFile = ".git-blame-ignore-revs"

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
// same git log pass. Pair order is unspecified; internal/analysis imposes a
// total order after applying the noise floors.
type LogResult struct {
	Churn []FileChurn
	Pairs []CouplingPair
}

// loadIgnoreRevs reads <repoRoot>/.git-blame-ignore-revs into a set of
// lowercase commit SHAs. A missing file yields an empty set. Blank lines and
// `#` comments are skipped, as are lines that aren't full 40/64-char hex
// object names — hc is best-effort where git blame would error.
func loadIgnoreRevs(repoRoot string) (map[string]struct{}, error) {
	lines, err := ignore.LoadFile(filepath.Join(repoRoot, ignoreRevsFile))
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", ignoreRevsFile, err)
	}
	var revs map[string]struct{}
	for _, line := range lines {
		if !isHexObjectName(line) {
			continue
		}
		if revs == nil {
			revs = make(map[string]struct{})
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

	// resolve memoizes rename resolution + ignore matching per unique raw
	// path ("" = ignored): Match runs every ignore pattern's regexp, too
	// costly to repeat per file occurrence per commit.
	resolved := make(map[string]string)
	resolve := func(raw string) string {
		if r, ok := resolved[raw]; ok {
			return r
		}
		r := renames.Resolve(raw)
		if ig.Match(r) {
			r = ""
		}
		resolved[raw] = r
		return r
	}

	set := make(map[string]struct{})
	var paths []string
	for _, ci := range commits {
		// %H is lowercase hex, matching the loadIgnoreRevs set.
		if _, skip := ignoreRevs[ci.SHA]; skip {
			continue
		}
		clear(set)
		for _, f := range ci.Files {
			if f == "" {
				continue
			}
			if r := resolve(f); r != "" {
				set[r] = struct{}{}
			}
		}
		if len(set) < 2 {
			continue
		}
		paths = paths[:0]
		for p := range set {
			paths = append(paths, p)
		}
		sort.Strings(paths) // canonical A < B pair keys

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
	return pairs
}
