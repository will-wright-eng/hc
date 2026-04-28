package analysis

import (
	"testing"

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
		{Path: "a.go", Commits: 100, WeightedCommits: 100, Authors: 5},
		{Path: "b.go", Commits: 1, WeightedCommits: 1, Authors: 1},
		{Path: "c.go", Commits: 50, WeightedCommits: 50, Authors: 2},
	}
	complexities := []complexity.FileComplexity{
		{Path: "a.go", Lines: 1000, Complexity: 1000},
		{Path: "b.go", Lines: 5, Complexity: 5},
		{Path: "c.go", Lines: 500, Complexity: 500},
	}

	scores := Analyze(churns, complexities)
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
		{Path: "src/a.go", Commits: 10, WeightedCommits: 10, Lines: 100, Complexity: 100, Quadrant: HotCritical},
		{Path: "src/b.go", Commits: 5, WeightedCommits: 5, Lines: 50, Complexity: 50, Quadrant: HotSimple},
		{Path: "lib/c.go", Commits: 1, WeightedCommits: 1, Lines: 200, Complexity: 200, Quadrant: ColdComplex},
	}

	dirs := AnalyzeByDir(fileScores, -1)
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

func TestAnalyzeByDir_Level0_SingleBucket(t *testing.T) {
	fileScores := []FileScore{
		{Path: "cmd/hc/main.go", Commits: 10, WeightedCommits: 10, Lines: 100, Complexity: 100},
		{Path: "internal/git/git.go", Commits: 5, WeightedCommits: 5, Lines: 50, Complexity: 50},
		{Path: "internal/analysis/analysis.go", Commits: 3, WeightedCommits: 3, Lines: 80, Complexity: 80},
		{Path: "README.md", Commits: 2, WeightedCommits: 2, Lines: 30, Complexity: 30},
	}

	dirs := AnalyzeByDir(fileScores, 0)
	if len(dirs) != 1 {
		t.Fatalf("level 0 should produce a single bucket, got %d dirs", len(dirs))
	}
	if dirs[0].Path != "." {
		t.Errorf("expected path '.', got %q", dirs[0].Path)
	}
	if dirs[0].Files != 4 || dirs[0].TotalCommits != 20 {
		t.Errorf("single bucket: got files=%d commits=%d, want 4/20", dirs[0].Files, dirs[0].TotalCommits)
	}
}

func TestAnalyzeByDir_Level1_TopLevelOnly(t *testing.T) {
	fileScores := []FileScore{
		{Path: "cmd/hc/main.go", Commits: 10, WeightedCommits: 10, Lines: 100, Complexity: 100},
		{Path: "cmd/hc/util.go", Commits: 4, WeightedCommits: 4, Lines: 40, Complexity: 40},
		{Path: "internal/git/git.go", Commits: 5, WeightedCommits: 5, Lines: 50, Complexity: 50},
		{Path: "internal/analysis/analysis.go", Commits: 3, WeightedCommits: 3, Lines: 80, Complexity: 80},
	}

	dirs := AnalyzeByDir(fileScores, 1)
	if len(dirs) != 2 {
		t.Fatalf("level 1 should yield 2 top-level dirs, got %d", len(dirs))
	}

	dirMap := make(map[string]DirScore)
	for _, d := range dirs {
		dirMap[d.Path] = d
	}

	cmd, ok := dirMap["cmd"]
	if !ok {
		t.Fatalf("missing cmd bucket")
	}
	if cmd.Files != 2 || cmd.TotalCommits != 14 {
		t.Errorf("cmd: got files=%d commits=%d, want 2/14", cmd.Files, cmd.TotalCommits)
	}

	internal, ok := dirMap["internal"]
	if !ok {
		t.Fatalf("missing internal bucket")
	}
	if internal.Files != 2 || internal.TotalCommits != 8 {
		t.Errorf("internal: got files=%d commits=%d, want 2/8", internal.Files, internal.TotalCommits)
	}
}

func TestAnalyzeByDir_LevelExceedsDepth(t *testing.T) {
	// Files shallower than the cap retain their natural depth (no padding).
	fileScores := []FileScore{
		{Path: "main.go", Commits: 10, WeightedCommits: 10, Lines: 100, Complexity: 100},
		{Path: "cmd/hc/main.go", Commits: 5, WeightedCommits: 5, Lines: 50, Complexity: 50},
		{Path: "a/b/c/d/deep.go", Commits: 1, WeightedCommits: 1, Lines: 20, Complexity: 20},
	}

	dirs := AnalyzeByDir(fileScores, 99)
	dirMap := make(map[string]DirScore)
	for _, d := range dirs {
		dirMap[d.Path] = d
	}

	if _, ok := dirMap["."]; !ok {
		t.Errorf("root file should bucket under '.'")
	}
	if _, ok := dirMap["cmd/hc"]; !ok {
		t.Errorf("depth-2 file should keep cmd/hc, not pad to level 99")
	}
	if _, ok := dirMap["a/b/c/d"]; !ok {
		t.Errorf("depth-4 file should keep its full parent dir under generous cap")
	}
}

func TestAnalyzeByDir_LevelTruncatesDeep(t *testing.T) {
	// Files deeper than the cap are truncated; siblings within the cap merge.
	fileScores := []FileScore{
		{Path: "internal/git/git.go", Commits: 5, WeightedCommits: 5, Lines: 50, Complexity: 50},
		{Path: "internal/git/decay.go", Commits: 3, WeightedCommits: 3, Lines: 30, Complexity: 30},
		{Path: "internal/analysis/sub/deeper.go", Commits: 2, WeightedCommits: 2, Lines: 20, Complexity: 20},
	}

	dirs := AnalyzeByDir(fileScores, 2)
	dirMap := make(map[string]DirScore)
	for _, d := range dirs {
		dirMap[d.Path] = d
	}

	git, ok := dirMap["internal/git"]
	if !ok {
		t.Fatalf("missing internal/git bucket")
	}
	if git.Files != 2 || git.TotalCommits != 8 {
		t.Errorf("internal/git: got files=%d commits=%d, want 2/8", git.Files, git.TotalCommits)
	}

	analysis, ok := dirMap["internal/analysis"]
	if !ok {
		t.Fatalf("deeper file should truncate to internal/analysis at level 2")
	}
	if analysis.Files != 1 || analysis.TotalCommits != 2 {
		t.Errorf("internal/analysis: got files=%d commits=%d, want 1/2", analysis.Files, analysis.TotalCommits)
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
