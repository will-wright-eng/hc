package analysis

import (
	"sort"
	"strings"

	"github.com/will-wright-eng/hc/internal/complexity"
	"github.com/will-wright-eng/hc/internal/git"
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
	Path            string
	Commits         int
	WeightedCommits float64
	Lines           int
	Complexity      int
	Authors         int
	Quadrant        Quadrant
}

// DirScore is an aggregated analysis result for a directory.
type DirScore struct {
	Path                 string
	Files                int
	TotalLines           int
	TotalComplexity      int
	TotalCommits         int
	TotalWeightedCommits float64
	Quadrant             Quadrant
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
			Path:            cx.Path,
			Commits:         ch.Commits,
			WeightedCommits: ch.WeightedCommits,
			Lines:           cx.Lines,
			Complexity:      cx.Complexity,
			Authors:         ch.Authors,
		})
	}

	if len(scores) == 0 {
		return nil
	}

	churnThreshold := medianWeightedCommits(scores)
	complexityThreshold := float64(medianComplexity(scores))

	for i := range scores {
		scores[i].Quadrant = classify(scores[i].WeightedCommits, scores[i].Complexity, churnThreshold, complexityThreshold)
	}

	sortScores(scores)
	return scores
}

// AnalyzeByDir aggregates file scores into directory-level results.
//
// level controls how deeply paths are rolled up:
//
//	level < 0  → no cap; group by each file's full parent directory (default).
//	level == 0 → single bucket; everything aggregates into "." (whole-repo summary).
//	level > 0  → truncate each directory to N path segments before grouping.
//
// Files shallower than level keep their natural depth (no padding); files
// deeper than level are truncated. Mirrors `tree -L N` semantics.
func AnalyzeByDir(fileScores []FileScore, level int) []DirScore {
	type dirAgg struct {
		files                int
		totalLines           int
		totalComplexity      int
		totalCommits         int
		totalWeightedCommits float64
	}

	m := make(map[string]*dirAgg)
	for _, fs := range fileScores {
		dir := capDepth(dirOf(fs.Path), level)
		agg, ok := m[dir]
		if !ok {
			agg = &dirAgg{}
			m[dir] = agg
		}
		agg.files++
		agg.totalLines += fs.Lines
		agg.totalComplexity += fs.Complexity
		agg.totalCommits += fs.Commits
		agg.totalWeightedCommits += fs.WeightedCommits
	}

	var dirs []DirScore
	for path, agg := range m {
		dirs = append(dirs, DirScore{
			Path:                 path,
			Files:                agg.files,
			TotalLines:           agg.totalLines,
			TotalComplexity:      agg.totalComplexity,
			TotalCommits:         agg.totalCommits,
			TotalWeightedCommits: agg.totalWeightedCommits,
		})
	}

	if len(dirs) == 0 {
		return nil
	}

	commitThreshold := medianDirWeightedCommits(dirs)
	complexityThreshold := float64(medianDirComplexity(dirs))

	for i := range dirs {
		dirs[i].Quadrant = classify(dirs[i].TotalWeightedCommits, dirs[i].TotalComplexity, commitThreshold, complexityThreshold)
	}

	sortDirScores(dirs)
	return dirs
}

func classify(weightedCommits float64, lines int, churnThreshold, linesThreshold float64) Quadrant {
	hot := weightedCommits > churnThreshold
	complex := float64(lines) > linesThreshold
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

func medianWeightedCommits(scores []FileScore) float64 {
	vals := make([]float64, len(scores))
	for i, s := range scores {
		vals[i] = s.WeightedCommits
	}
	return medianFloat(vals)
}

func medianComplexity(scores []FileScore) int {
	vals := make([]int, len(scores))
	for i, s := range scores {
		vals[i] = s.Complexity
	}
	return median(vals)
}

func medianDirWeightedCommits(dirs []DirScore) float64 {
	vals := make([]float64, len(dirs))
	for i, d := range dirs {
		vals[i] = d.TotalWeightedCommits
	}
	return medianFloat(vals)
}

func medianDirComplexity(dirs []DirScore) int {
	vals := make([]int, len(dirs))
	for i, d := range dirs {
		vals[i] = d.TotalComplexity
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

func medianFloat(vals []float64) float64 {
	sort.Float64s(vals)
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
		return scores[i].WeightedCommits > scores[j].WeightedCommits
	})
}

func sortDirScores(dirs []DirScore) {
	sort.Slice(dirs, func(i, j int) bool {
		pi, pj := quadrantPriority(dirs[i].Quadrant), quadrantPriority(dirs[j].Quadrant)
		if pi != pj {
			return pi < pj
		}
		return dirs[i].TotalWeightedCommits > dirs[j].TotalWeightedCommits
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

func capDepth(dir string, level int) string {
	if level < 0 || dir == "." {
		return dir
	}
	if level == 0 {
		return "."
	}
	parts := strings.Split(dir, "/")
	if len(parts) > level {
		parts = parts[:level]
	}
	return strings.Join(parts, "/")
}
