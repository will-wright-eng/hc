package analysis

import (
	"sort"

	"github.com/will/hc/internal/complexity"
	"github.com/will/hc/internal/git"
)

// Quadrant classifies a file or directory by churn × complexity.
type Quadrant int

const (
	ColdSimple  Quadrant = iota // low churn, low complexity
	ColdComplex                 // low churn, high complexity
	HotSimple                   // high churn, low complexity
	HotCritical                 // high churn, high complexity
)

func (q Quadrant) String() string {
	switch q {
	case ColdSimple:
		return "Cold Simple"
	case ColdComplex:
		return "Cold Complex"
	case HotSimple:
		return "Hot Simple"
	case HotCritical:
		return "Hot Critical"
	default:
		return "Unknown"
	}
}

// JSONString returns the kebab-case form for JSON output.
func (q Quadrant) JSONString() string {
	switch q {
	case ColdSimple:
		return "cold-simple"
	case ColdComplex:
		return "cold-complex"
	case HotSimple:
		return "hot-simple"
	case HotCritical:
		return "hot-critical"
	default:
		return "unknown"
	}
}

// FileScore is the combined analysis result for a single file.
type FileScore struct {
	Path     string
	Commits  int
	Lines    int
	Authors  int
	Quadrant Quadrant
}

// DirScore is an aggregated analysis result for a directory.
type DirScore struct {
	Path         string
	Files        int
	TotalLines   int
	TotalCommits int
	Quadrant     Quadrant
}

// Analyze merges churn and complexity data, classifies files into quadrants,
// and returns results sorted by priority.
func Analyze(churns []git.FileChurn, complexities []complexity.FileComplexity) []FileScore {
	churnMap := make(map[string]git.FileChurn, len(churns))
	for _, c := range churns {
		churnMap[c.Path] = c
	}

	complexMap := make(map[string]complexity.FileComplexity, len(complexities))
	for _, c := range complexities {
		complexMap[c.Path] = c
	}

	// Build merged set: only files that exist on disk (in complexities) are included.
	// Files in git history but not on disk are excluded.
	var scores []FileScore
	for _, cx := range complexities {
		ch := churnMap[cx.Path]
		scores = append(scores, FileScore{
			Path:    cx.Path,
			Commits: ch.Commits,
			Lines:   cx.Lines,
			Authors: ch.Authors,
		})
	}

	if len(scores) == 0 {
		return nil
	}

	churnThreshold := medianCommits(scores)
	linesThreshold := medianLines(scores)

	for i := range scores {
		scores[i].Quadrant = classify(scores[i].Commits, scores[i].Lines, churnThreshold, linesThreshold)
	}

	sortScores(scores)
	return scores
}

// AnalyzeByDir aggregates file scores into directory-level results.
func AnalyzeByDir(fileScores []FileScore) []DirScore {
	type dirAgg struct {
		files        int
		totalLines   int
		totalCommits int
	}

	m := make(map[string]*dirAgg)
	for _, fs := range fileScores {
		dir := dirOf(fs.Path)
		agg, ok := m[dir]
		if !ok {
			agg = &dirAgg{}
			m[dir] = agg
		}
		agg.files++
		agg.totalLines += fs.Lines
		agg.totalCommits += fs.Commits
	}

	var dirs []DirScore
	for path, agg := range m {
		dirs = append(dirs, DirScore{
			Path:         path,
			Files:        agg.files,
			TotalLines:   agg.totalLines,
			TotalCommits: agg.totalCommits,
		})
	}

	if len(dirs) == 0 {
		return nil
	}

	commitThreshold := medianDirCommits(dirs)
	linesThreshold := medianDirLines(dirs)

	for i := range dirs {
		dirs[i].Quadrant = classify(dirs[i].TotalCommits, dirs[i].TotalLines, commitThreshold, linesThreshold)
	}

	sortDirScores(dirs)
	return dirs
}

func classify(commits, lines, churnThreshold, linesThreshold int) Quadrant {
	hot := commits > churnThreshold
	complex := lines > linesThreshold
	switch {
	case hot && complex:
		return HotCritical
	case hot && !complex:
		return HotSimple
	case !hot && complex:
		return ColdComplex
	default:
		return ColdSimple
	}
}

func medianCommits(scores []FileScore) int {
	vals := make([]int, len(scores))
	for i, s := range scores {
		vals[i] = s.Commits
	}
	return median(vals)
}

func medianLines(scores []FileScore) int {
	vals := make([]int, len(scores))
	for i, s := range scores {
		vals[i] = s.Lines
	}
	return median(vals)
}

func medianDirCommits(dirs []DirScore) int {
	vals := make([]int, len(dirs))
	for i, d := range dirs {
		vals[i] = d.TotalCommits
	}
	return median(vals)
}

func medianDirLines(dirs []DirScore) int {
	vals := make([]int, len(dirs))
	for i, d := range dirs {
		vals[i] = d.TotalLines
	}
	return median(vals)
}

func median(vals []int) int {
	sort.Ints(vals)
	n := len(vals)
	if n == 0 {
		return 0
	}
	if n%2 == 0 {
		return (vals[n/2-1] + vals[n/2]) / 2
	}
	return vals[n/2]
}

// quadrantPriority defines sort order — HotCritical first.
func quadrantPriority(q Quadrant) int {
	switch q {
	case HotCritical:
		return 0
	case HotSimple:
		return 1
	case ColdComplex:
		return 2
	case ColdSimple:
		return 3
	default:
		return 4
	}
}

func sortScores(scores []FileScore) {
	sort.Slice(scores, func(i, j int) bool {
		pi, pj := quadrantPriority(scores[i].Quadrant), quadrantPriority(scores[j].Quadrant)
		if pi != pj {
			return pi < pj
		}
		return scores[i].Commits > scores[j].Commits
	})
}

func sortDirScores(dirs []DirScore) {
	sort.Slice(dirs, func(i, j int) bool {
		pi, pj := quadrantPriority(dirs[i].Quadrant), quadrantPriority(dirs[j].Quadrant)
		if pi != pj {
			return pi < pj
		}
		return dirs[i].TotalCommits > dirs[j].TotalCommits
	})
}

func dirOf(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[:i]
		}
	}
	return "."
}
