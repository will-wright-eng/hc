package analysis

import (
	"math"
	"testing"

	"github.com/will-wright-eng/hc/internal/git"
)

func couplingFiles() []FileScore {
	return []FileScore{
		{Path: "a.go", WeightedCommits: 10},
		{Path: "b.go", WeightedCommits: 5},
		{Path: "c.go", WeightedCommits: 20},
	}
}

func TestAnalyzeCoupling_ConfidenceAsymmetry(t *testing.T) {
	pairs := []git.CouplingPair{
		{A: "a.go", B: "b.go", Support: 5, WeightedSupport: 4},
	}
	got := AnalyzeCoupling(pairs, couplingFiles())
	if len(got) != 1 {
		t.Fatalf("expected 1 pair, got %d", len(got))
	}
	// A weighted 10, B weighted 5: 4/10 vs 4/5.
	if math.Abs(got[0].ConfidenceAB-0.4) > 0.001 {
		t.Errorf("confidence a->b = %f, want 0.4", got[0].ConfidenceAB)
	}
	if math.Abs(got[0].ConfidenceBA-0.8) > 0.001 {
		t.Errorf("confidence b->a = %f, want 0.8", got[0].ConfidenceBA)
	}
}

func TestAnalyzeCoupling_Floors(t *testing.T) {
	pairs := []git.CouplingPair{
		// Raw support below 5: dropped even with perfect confidence.
		{A: "a.go", B: "b.go", Support: 4, WeightedSupport: 5},
		// Max confidence below 0.5 (5/20 = 0.25, 5/10 = 0.5 — kept at boundary).
		{A: "a.go", B: "c.go", Support: 5, WeightedSupport: 5},
		// Both confidences below 0.5 (4.5/10 = 0.45, 4.5/20 = 0.225): dropped.
		{A: "a.go", B: "c.go", Support: 5, WeightedSupport: 4.5},
	}
	got := AnalyzeCoupling(pairs[:1], couplingFiles())
	if len(got) != 0 {
		t.Errorf("support 4 should be dropped, got %+v", got)
	}
	got = AnalyzeCoupling(pairs[1:2], couplingFiles())
	if len(got) != 1 {
		t.Errorf("max confidence at the 0.5 boundary should be kept, got %+v", got)
	}
	got = AnalyzeCoupling(pairs[2:], couplingFiles())
	if len(got) != 0 {
		t.Errorf("max confidence below 0.5 should be dropped, got %+v", got)
	}
}

func TestAnalyzeCoupling_UniverseFilter(t *testing.T) {
	pairs := []git.CouplingPair{
		{A: "a.go", B: "deleted.go", Support: 9, WeightedSupport: 9},
		{A: "aged.go", B: "b.go", Support: 9, WeightedSupport: 9},
	}
	got := AnalyzeCoupling(pairs, couplingFiles())
	if len(got) != 0 {
		t.Errorf("pairs with a side outside the file universe should be dropped, got %+v", got)
	}
}

func TestAnalyzeCoupling_Sort(t *testing.T) {
	files := []FileScore{
		{Path: "a.go", WeightedCommits: 10},
		{Path: "b.go", WeightedCommits: 10},
		{Path: "c.go", WeightedCommits: 10},
		{Path: "d.go", WeightedCommits: 6},
	}
	pairs := []git.CouplingPair{
		{A: "a.go", B: "b.go", Support: 5, WeightedSupport: 5},
		{A: "c.go", B: "d.go", Support: 6, WeightedSupport: 6},
		// Ties c/d on weighted support but max confidence is lower (6/10 both
		// sides vs 6/6 for c-d's d side).
		{A: "a.go", B: "c.go", Support: 6, WeightedSupport: 6},
	}
	got := AnalyzeCoupling(pairs, files)
	if len(got) != 3 {
		t.Fatalf("expected 3 pairs, got %d", len(got))
	}
	wantOrder := [][2]string{{"c.go", "d.go"}, {"a.go", "c.go"}, {"a.go", "b.go"}}
	for i, w := range wantOrder {
		if got[i].A != w[0] || got[i].B != w[1] {
			t.Errorf("position %d = (%s, %s), want (%s, %s)", i, got[i].A, got[i].B, w[0], w[1])
		}
	}
}
