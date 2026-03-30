package analysis

import (
	"testing"

	"github.com/will/hc/internal/complexity"
	"github.com/will/hc/internal/git"
)

func TestAnalyze_QuadrantClassification(t *testing.T) {
	churns := []git.FileChurn{
		{Path: "hot-critical.go", Commits: 100, Authors: 5},
		{Path: "hot-simple.go", Commits: 80, Authors: 3},
		{Path: "cold-complex.go", Commits: 2, Authors: 1},
		{Path: "cold-simple.go", Commits: 1, Authors: 1},
	}
	complexities := []complexity.FileComplexity{
		{Path: "hot-critical.go", Lines: 1000},
		{Path: "hot-simple.go", Lines: 10},
		{Path: "cold-complex.go", Lines: 900},
		{Path: "cold-simple.go", Lines: 5},
	}

	scores := Analyze(churns, complexities)

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
		{Path: "a.go", Commits: 100, Authors: 5},
		{Path: "b.go", Commits: 1, Authors: 1},
		{Path: "c.go", Commits: 50, Authors: 2},
	}
	complexities := []complexity.FileComplexity{
		{Path: "a.go", Lines: 1000},
		{Path: "b.go", Lines: 5},
		{Path: "c.go", Lines: 500},
	}

	scores := Analyze(churns, complexities)
	// HotCritical should come first
	if scores[0].Path != "a.go" {
		t.Errorf("expected a.go first (HotCritical), got %s (%s)", scores[0].Path, scores[0].Quadrant)
	}
}

func TestAnalyze_ExcludesDeletedFiles(t *testing.T) {
	churns := []git.FileChurn{
		{Path: "exists.go", Commits: 10, Authors: 1},
		{Path: "deleted.go", Commits: 5, Authors: 1},
	}
	complexities := []complexity.FileComplexity{
		{Path: "exists.go", Lines: 100},
		// deleted.go not present in complexity results (not on disk)
	}

	scores := Analyze(churns, complexities)
	if len(scores) != 1 {
		t.Fatalf("expected 1 score (deleted file excluded), got %d", len(scores))
	}
	if scores[0].Path != "exists.go" {
		t.Errorf("expected exists.go, got %s", scores[0].Path)
	}
}

func TestAnalyzeByDir(t *testing.T) {
	fileScores := []FileScore{
		{Path: "src/a.go", Commits: 10, Lines: 100, Quadrant: HotCritical},
		{Path: "src/b.go", Commits: 5, Lines: 50, Quadrant: HotSimple},
		{Path: "lib/c.go", Commits: 1, Lines: 200, Quadrant: ColdComplex},
	}

	dirs := AnalyzeByDir(fileScores)
	if len(dirs) != 2 {
		t.Fatalf("expected 2 dirs, got %d", len(dirs))
	}

	dirMap := make(map[string]DirScore)
	for _, d := range dirs {
		dirMap[d.Path] = d
	}

	src := dirMap["src"]
	if src.Files != 2 || src.TotalCommits != 15 || src.TotalLines != 150 {
		t.Errorf("src dir: got files=%d commits=%d lines=%d", src.Files, src.TotalCommits, src.TotalLines)
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
