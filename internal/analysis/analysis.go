package analysis

import (
	"sort"
	"time"

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
	FirstSeen       time.Time
}

// Options controls file scoring and post-classification filtering.
type Options struct {
	// MinAge filters files whose first-seen commit is younger than this duration.
	// Zero disables the filter.
	MinAge time.Duration
	// Now is the reference time for age-based filtering. Zero means time.Now().
	Now time.Time
}

// Result bundles the scored files with the thresholds used to classify them.
// Callers that don't need thresholds can use Analyze, which discards them.
type Result struct {
	Files               []FileScore
	ChurnThreshold      float64
	ComplexityThreshold int
}

// Analyze merges churn and complexity data, classifies files into quadrants,
// and returns results sorted by priority.
//
// minAge filters files whose first-seen commit is younger than the given
// duration (zero disables). The median computation runs over all files first,
// so thresholds reflect the whole repository's distribution; young files are
// dropped only after classification.
func Analyze(churns []git.FileChurn, complexities []complexity.FileComplexity, minAge time.Duration) []FileScore {
	return AnalyzeWithOptions(churns, complexities, Options{MinAge: minAge}).Files
}

// AnalyzeWithOptions merges churn and complexity data, classifies files into
// quadrants, and returns results sorted by priority along with the thresholds
// used.
func AnalyzeWithOptions(churns []git.FileChurn, complexities []complexity.FileComplexity, opts Options) Result {
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
			FirstSeen:       ch.FirstSeen,
		})
	}

	if len(scores) == 0 {
		return Result{}
	}

	churnThreshold := medianWeightedCommits(scores)
	complexityThreshold := medianComplexity(scores)

	for i := range scores {
		scores[i].Quadrant = classify(scores[i].WeightedCommits, scores[i].Complexity, churnThreshold, float64(complexityThreshold))
	}

	if opts.MinAge > 0 {
		now := opts.Now
		if now.IsZero() {
			now = time.Now()
		}
		scores = filterByMinAge(scores, now, opts.MinAge)
	}

	sortScores(scores)
	return Result{
		Files:               scores,
		ChurnThreshold:      churnThreshold,
		ComplexityThreshold: complexityThreshold,
	}
}

// filterByMinAge drops files whose first-seen commit is younger than minAge.
// Zero-valued FirstSeen (no in-window commits) is treated as old enough — the
// file existed before the window opened, so the floor doesn't apply.
func filterByMinAge(scores []FileScore, now time.Time, minAge time.Duration) []FileScore {
	out := scores[:0]
	for _, s := range scores {
		if !s.FirstSeen.IsZero() && now.Sub(s.FirstSeen) < minAge {
			continue
		}
		out = append(out, s)
	}
	return out
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
