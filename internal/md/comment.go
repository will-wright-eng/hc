package md

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/will-wright-eng/hc/internal/schema"
)

// annotationRule maps a quadrant to its GitHub Actions annotation level and the
// human wording used in the title and message. Quadrants without a rule
// (cold-simple, and hot-simple by default) never produce an annotation.
type annotationRule struct {
	level string // notice | warning
	title string
	lead  string // message body, follows the file path
}

var annotationRules = map[string]annotationRule{
	"hot-critical": {
		level: "warning",
		title: "Hot/Critical hotspot",
		lead:  "was already a Hot/Critical hotspot on the base branch: high churn and high complexity. Keep the diff focused, lean on tests, and review changes here carefully.",
	},
	"cold-complex": {
		level: "notice",
		title: "Cold/Complex hotspot",
		lead:  "was already a Cold/Complex hotspot on the base branch: stable but costly to touch. Prefer a small change and add or adjust tests before refactoring.",
	},
}

// quadrantRank is the canonical emission order (hot-critical first), matching
// the order used across hc.
var quadrantRank = map[string]int{
	"hot-critical": 0,
	"hot-simple":   1,
	"cold-complex": 2,
	"cold-simple":  3,
}

// AnnotateOpts configures RenderAnnotations.
type AnnotateOpts struct {
	// Quadrants restricts output to the listed quadrant keys. Empty (or only
	// empty strings) means the default set — the quadrants that have an
	// annotation rule: hot-critical and cold-complex. Keys without a rule yield
	// no annotations.
	Quadrants []string
	// AnchorLines maps a repo-relative path to the line its annotation should
	// target, so the annotation renders inline on the PR diff. Paths not in the
	// map fall back to line 1. Nil is fine (everything falls back to line 1).
	AnchorLines map[string]int
}

// RenderAnnotations reads analyze JSON from r (output of `hc analyze --json`),
// filters and sorts the hotspot files, and writes one GitHub Actions
// workflow-command annotation per file to w. GitHub's runner converts these to
// check-run annotations on the pull request. Empty or filtered-to-empty input
// produces no output.
func RenderAnnotations(r io.Reader, w io.Writer, opts AnnotateOpts) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("reading input: %w", err)
	}
	if looksLikeBareArray(data) {
		return fmt.Errorf("input is a bare JSON array, not an hc analyze envelope; regenerate with the current `hc analyze --json`")
	}

	var env schema.Envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return fmt.Errorf("parsing JSON: %w", err)
	}
	if env.SchemaVersion == "" {
		return fmt.Errorf("input is not an hc analyze envelope (missing schema_version); regenerate with the current `hc analyze --json`")
	}

	want := annotateQuadrantSet(opts.Quadrants)

	kept := make([]schema.File, 0, len(env.Files))
	for _, f := range env.Files {
		if !want[f.Quadrant] {
			continue
		}
		if _, ok := annotationRules[f.Quadrant]; !ok {
			continue // cold-simple / hot-simple-by-default / unknown → no rule
		}
		kept = append(kept, f)
	}

	// Quadrant rank, then weighted commits desc (decay on), then raw commits
	// desc (so --no-decay still orders by activity), then path for full
	// determinism.
	sort.SliceStable(kept, func(i, j int) bool {
		a, b := kept[i], kept[j]
		if quadrantRank[a.Quadrant] != quadrantRank[b.Quadrant] {
			return quadrantRank[a.Quadrant] < quadrantRank[b.Quadrant]
		}
		if a.WeightedCommits != b.WeightedCommits {
			return a.WeightedCommits > b.WeightedCommits
		}
		if a.Commits != b.Commits {
			return a.Commits > b.Commits
		}
		return a.Path < b.Path
	})

	for _, f := range kept {
		rule := annotationRules[f.Quadrant]
		line := max(opts.AnchorLines[f.Path], 1)
		message := fmt.Sprintf("%s %s %s", f.Path, rule.lead, statsSuffix(f, env.Options.Decay))
		if _, err := fmt.Fprintf(w, "::%s file=%s,line=%d,title=%s::%s\n",
			rule.level, escapeProperty(f.Path), line, escapeProperty(rule.title), escapeData(message)); err != nil {
			return err
		}
	}
	return nil
}

func annotateQuadrantSet(in []string) map[string]bool {
	set := make(map[string]bool)
	for _, q := range in {
		if q != "" {
			set[q] = true
		}
	}
	if len(set) == 0 {
		set["hot-critical"] = true
		set["cold-complex"] = true
	}
	return set
}

func statsSuffix(f schema.File, decay bool) string {
	if decay {
		return fmt.Sprintf("(commits %d, weighted %.1f, complexity %d, authors %d)",
			f.Commits, f.WeightedCommits, f.Complexity, f.Authors)
	}
	return fmt.Sprintf("(commits %d, complexity %d, authors %d)", f.Commits, f.Complexity, f.Authors)
}

// escapeData escapes the message portion of a workflow command (the text after
// `::`). '%' is escaped first so the escapes it introduces are not re-escaped.
func escapeData(s string) string {
	s = strings.ReplaceAll(s, "%", "%25")
	s = strings.ReplaceAll(s, "\r", "%0D")
	s = strings.ReplaceAll(s, "\n", "%0A")
	return s
}

// escapeProperty escapes a workflow-command property value (file, title), which
// additionally must escape ':' and ',' so they are not read as delimiters.
func escapeProperty(s string) string {
	s = escapeData(s)
	s = strings.ReplaceAll(s, ":", "%3A")
	s = strings.ReplaceAll(s, ",", "%2C")
	return s
}
