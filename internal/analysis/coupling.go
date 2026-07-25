package analysis

import (
	"math"
	"sort"

	"github.com/will-wright-eng/hc/internal/git"
)

// Fixed noise floors for coupling pairs. A deliberate departure from hc's
// median-split identity: pair frequencies are power-law distributed, so a
// median split does not transfer. The support floor uses raw counts (stable
// and explainable); ranking uses weighted support (recency-aware). See
// docs/proposals/012-change-coupling.md.
const (
	CouplingMinSupport    = 5
	CouplingMinConfidence = 0.5
)

// CouplingScore is a co-change pair that survived the noise floors.
// Confidences are asymmetric on purpose: ConfidenceAB is "when A changes, B
// changes this fraction of the time" (weighted co-changes ÷ A's weighted
// churn), and ConfidenceBA the reverse.
type CouplingScore struct {
	A               string
	B               string
	Support         int
	WeightedSupport float64
	ConfidenceAB    float64
	ConfidenceBA    float64
}

// AnalyzeCoupling filters raw pairs to the surviving file universe (both
// sides must be in files — deleted and age-floored paths never pair), applies
// the fixed floors, computes confidences from each side's weighted churn, and
// sorts by weighted support descending, tie-broken by max confidence, then
// path.
func AnalyzeCoupling(pairs []git.CouplingPair, files []FileScore) []CouplingScore {
	weight := make(map[string]float64, len(files))
	for _, f := range files {
		weight[f.Path] = f.WeightedCommits
	}

	scores := make([]CouplingScore, 0)
	for _, p := range pairs {
		wa, okA := weight[p.A]
		wb, okB := weight[p.B]
		if !okA || !okB {
			continue
		}
		if p.Support < CouplingMinSupport {
			continue
		}
		var confAB, confBA float64
		if wa > 0 {
			confAB = p.WeightedSupport / wa
		}
		if wb > 0 {
			confBA = p.WeightedSupport / wb
		}
		if math.Max(confAB, confBA) < CouplingMinConfidence {
			continue
		}
		scores = append(scores, CouplingScore{
			A:               p.A,
			B:               p.B,
			Support:         p.Support,
			WeightedSupport: p.WeightedSupport,
			ConfidenceAB:    confAB,
			ConfidenceBA:    confBA,
		})
	}

	sort.Slice(scores, func(i, j int) bool {
		a, b := scores[i], scores[j]
		if a.WeightedSupport != b.WeightedSupport {
			return a.WeightedSupport > b.WeightedSupport
		}
		ma := math.Max(a.ConfidenceAB, a.ConfidenceBA)
		mb := math.Max(b.ConfidenceAB, b.ConfidenceBA)
		if ma != mb {
			return ma > mb
		}
		if a.A != b.A {
			return a.A < b.A
		}
		return a.B < b.B
	})
	return scores
}
