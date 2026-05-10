package analysis

import (
	"testing"
	"time"

	"github.com/will-wright-eng/hc/internal/complexity"
	"github.com/will-wright-eng/hc/internal/git"
)

func TestAnalyze_QuadrantClassification(t *testing.T) {
	churns := []git.FileChurn{
		{Path: "hot-critical.go", Commits: 100, WeightedCommits: 100, Authors: 5},
		{Path: "hot-simple.go", Commits: 80, WeightedCommits: 80, Authors: 3},
		{Path: "cold-complex.go", Commits: 2, WeightedCommits: 2, Authors: 1},
		{Path: "cold-simple.go", Commits: 1, WeightedCommits: 1, Authors: 1},
	}
	complexities := []complexity.FileComplexity{
		{Path: "hot-critical.go", Lines: 1000, Complexity: 1000},
		{Path: "hot-simple.go", Lines: 10, Complexity: 10},
		{Path: "cold-complex.go", Lines: 900, Complexity: 900},
		{Path: "cold-simple.go", Lines: 5, Complexity: 5},
	}

	scores := Analyze(churns, complexities, 0)

	if len(scores) != 4 {
		t.Fatalf("expected 4 scores, got %d", len(scores))
	}

	want := map[string]Quadrant{
		"hot-critical.go": HotCritical,
		"hot-simple.go":   HotSimple,
		"cold-complex.go": ColdComplex,
		"cold-simple.go":  ColdSimple,
	}

	for _, s := range scores {
		expected, ok := want[s.Path]
		if !ok {
			t.Errorf("unexpected path: %s", s.Path)
			continue
		}
		if s.Quadrant != expected {
			t.Errorf("%s: got quadrant %s, want %s", s.Path, s.Quadrant, expected)
		}
	}
}

func TestAnalyze_SortOrder(t *testing.T) {
	churns := []git.FileChurn{
		{Path: "a.go", Commits: 100, WeightedCommits: 100, Authors: 5},
		{Path: "b.go", Commits: 1, WeightedCommits: 1, Authors: 1},
		{Path: "c.go", Commits: 50, WeightedCommits: 50, Authors: 2},
	}
	complexities := []complexity.FileComplexity{
		{Path: "a.go", Lines: 1000, Complexity: 1000},
		{Path: "b.go", Lines: 5, Complexity: 5},
		{Path: "c.go", Lines: 500, Complexity: 500},
	}

	scores := Analyze(churns, complexities, 0)
	// HotCritical should come first
	if scores[0].Path != "a.go" {
		t.Errorf("expected a.go first (HotCritical), got %s (%s)", scores[0].Path, scores[0].Quadrant)
	}
}

func TestAnalyze_ExcludesDeletedFiles(t *testing.T) {
	churns := []git.FileChurn{
		{Path: "exists.go", Commits: 10, WeightedCommits: 10, Authors: 1},
		{Path: "deleted.go", Commits: 5, WeightedCommits: 5, Authors: 1},
	}
	complexities := []complexity.FileComplexity{
		{Path: "exists.go", Lines: 100, Complexity: 100},
		// deleted.go not present in complexity results (not on disk)
	}

	scores := Analyze(churns, complexities, 0)
	if len(scores) != 1 {
		t.Fatalf("expected 1 score (deleted file excluded), got %d", len(scores))
	}
	if scores[0].Path != "exists.go" {
		t.Errorf("expected exists.go, got %s", scores[0].Path)
	}
}

func TestAnalyze_MinAgeFiltersYoungFiles(t *testing.T) {
	now := time.Now()
	churns := []git.FileChurn{
		{Path: "old.go", Commits: 50, WeightedCommits: 50, Authors: 2, FirstSeen: now.Add(-90 * 24 * time.Hour)},
		{Path: "young.go", Commits: 50, WeightedCommits: 50, Authors: 2, FirstSeen: now.Add(-3 * 24 * time.Hour)},
	}
	complexities := []complexity.FileComplexity{
		{Path: "old.go", Lines: 800, Complexity: 800},
		{Path: "young.go", Lines: 800, Complexity: 800},
	}

	scores := Analyze(churns, complexities, 14*24*time.Hour)

	if len(scores) != 1 {
		t.Fatalf("expected young.go to be filtered out, got %d scores", len(scores))
	}
	if scores[0].Path != "old.go" {
		t.Errorf("expected old.go to remain, got %s", scores[0].Path)
	}
}

func TestAnalyzeWithOptions_UsesNowForMinAge(t *testing.T) {
	now := time.Date(2020, 1, 10, 12, 0, 0, 0, time.UTC)
	churns := []git.FileChurn{
		{Path: "young-at-fixed-now.go", Commits: 5, WeightedCommits: 5, FirstSeen: now.Add(-3 * 24 * time.Hour)},
	}
	complexities := []complexity.FileComplexity{
		{Path: "young-at-fixed-now.go", Lines: 100, Complexity: 100},
	}

	scores := AnalyzeWithOptions(churns, complexities, Options{
		MinAge: 14 * 24 * time.Hour,
		Now:    now,
	})
	if len(scores) != 0 {
		t.Fatalf("expected fixed Now to filter the young file, got %d scores", len(scores))
	}
}

func TestAnalyze_MinAgeMedianUnaffected(t *testing.T) {
	// Young file should still count toward the median, so the surviving
	// older files are classified against the full distribution.
	now := time.Now()
	churns := []git.FileChurn{
		{Path: "old-a.go", Commits: 100, WeightedCommits: 100, FirstSeen: now.Add(-200 * 24 * time.Hour)},
		{Path: "old-b.go", Commits: 1, WeightedCommits: 1, FirstSeen: now.Add(-200 * 24 * time.Hour)},
		// Two young files inflate the median if they were excluded pre-classification.
		{Path: "young-a.go", Commits: 50, WeightedCommits: 50, FirstSeen: now.Add(-2 * 24 * time.Hour)},
		{Path: "young-b.go", Commits: 50, WeightedCommits: 50, FirstSeen: now.Add(-2 * 24 * time.Hour)},
	}
	complexities := []complexity.FileComplexity{
		{Path: "old-a.go", Lines: 500, Complexity: 500},
		{Path: "old-b.go", Lines: 500, Complexity: 500},
		{Path: "young-a.go", Lines: 10, Complexity: 10},
		{Path: "young-b.go", Lines: 10, Complexity: 10},
	}

	scores := Analyze(churns, complexities, 14*24*time.Hour)

	// Median weighted commits over the full set is 50 (between 1 and 50).
	// old-a (100) is hot; old-b (1) is cold. If young files had been excluded
	// before the median was computed, the threshold would shift to 50.5 and
	// old-a's classification might change.
	got := map[string]Quadrant{}
	for _, s := range scores {
		got[s.Path] = s.Quadrant
	}
	if len(scores) != 2 {
		t.Fatalf("expected 2 surviving scores, got %d", len(scores))
	}
	if got["old-a.go"] != HotCritical {
		t.Errorf("old-a.go: got %s, want HotCritical", got["old-a.go"])
	}
	if got["old-b.go"] != ColdComplex {
		t.Errorf("old-b.go: got %s, want ColdComplex", got["old-b.go"])
	}
}

func TestAnalyze_MinAgeZeroDisables(t *testing.T) {
	now := time.Now()
	churns := []git.FileChurn{
		{Path: "young.go", Commits: 5, WeightedCommits: 5, FirstSeen: now.Add(-1 * 24 * time.Hour)},
	}
	complexities := []complexity.FileComplexity{
		{Path: "young.go", Lines: 100, Complexity: 100},
	}
	scores := Analyze(churns, complexities, 0)
	if len(scores) != 1 {
		t.Fatalf("minAge=0 should not filter; got %d", len(scores))
	}
}

func TestAnalyze_MinAgeIgnoresZeroFirstSeen(t *testing.T) {
	// A file with no in-window commits has zero-valued FirstSeen and should
	// pass the floor — it presumably existed before the window opened.
	churns := []git.FileChurn{
		{Path: "no-churn.go", Commits: 0, WeightedCommits: 0}, // FirstSeen zero
	}
	complexities := []complexity.FileComplexity{
		{Path: "no-churn.go", Lines: 100, Complexity: 100},
	}
	scores := Analyze(churns, complexities, 14*24*time.Hour)
	if len(scores) != 1 {
		t.Fatalf("zero FirstSeen should pass floor; got %d", len(scores))
	}
}

func TestMedian(t *testing.T) {
	tests := []struct {
		vals []int
		want int
	}{
		{[]int{1, 3, 5}, 3},
		{[]int{1, 2, 3, 4}, 2},
		{[]int{10}, 10},
		{[]int{}, 0},
	}

	for _, tt := range tests {
		got := median(tt.vals)
		if got != tt.want {
			t.Errorf("median(%v) = %d, want %d", tt.vals, got, tt.want)
		}
	}
}

func TestQuadrantStrings(t *testing.T) {
	if HotCritical.String() != "Hot Critical" {
		t.Errorf("got %q", HotCritical.String())
	}
	if HotCritical.JSONString() != "hot-critical" {
		t.Errorf("got %q", HotCritical.JSONString())
	}
}
