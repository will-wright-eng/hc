// Package annotate renders `hc analyze --json` into GitHub Actions
// workflow-command annotations (::warning / ::notice) for a pull request's
// changed hotspot files. Like the markdown renderers it consumes
// schema.Envelope, but it lives in its own package because the output is CI
// annotations, not markdown. See docs/proposals/010-pr-hotspot-annotations.md.
package annotate

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"unicode"

	"github.com/will-wright-eng/hc/internal/schema"
)

// rule maps a quadrant to its GitHub Actions annotation level and the human
// wording used in the title and message. Quadrants without a rule (cold-simple,
// and hot-simple by default) never produce an annotation.
type rule struct {
	level string // notice | warning
	title string
	lead  string // message body, follows the file path
}

var rules = map[string]rule{
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

// Options configures Render.
type Options struct {
	// Quadrants restricts output to the listed quadrant keys. Empty (or only
	// empty strings) means the default set — the quadrants that have a rule:
	// hot-critical and cold-complex. Keys without a rule yield no annotations.
	Quadrants []string
	// AnchorLines maps a repo-relative path to the line its annotation should
	// target, so the annotation renders inline on the PR diff. Paths not in the
	// map fall back to line 1. Nil is fine (everything falls back to line 1).
	AnchorLines map[string]int
}

// Render reads analyze JSON from r (output of `hc analyze --json`), filters and
// sorts the hotspot files, and writes one GitHub Actions workflow-command
// annotation per file to w. GitHub's runner converts these to check-run
// annotations on the pull request. Empty or filtered-to-empty input produces no
// output.
func Render(r io.Reader, w io.Writer, opts Options) error {
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

	want := quadrantSet(opts.Quadrants)

	kept := make([]schema.File, 0, len(env.Files))
	for _, f := range env.Files {
		if !want[f.Quadrant] {
			continue
		}
		if _, ok := rules[f.Quadrant]; !ok {
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
		rl := rules[f.Quadrant]
		message := fmt.Sprintf("%s %s %s", f.Path, rl.lead, statsSuffix(f, env.Options.Decay))
		if err := emitAnnotation(w, rl.level, f.Path, opts.AnchorLines[f.Path], rl.title, message); err != nil {
			return err
		}
	}

	// Partner notices come after hotspot annotations — hotspot warnings matter
	// more under GitHub's per-level display caps.
	return renderPartnerNotices(w, env, opts)
}

// emitAnnotation writes one GitHub Actions workflow command, owning the wire
// format, the property/data escaping, and the line-1 anchor fallback.
func emitAnnotation(w io.Writer, level, file string, line int, title, message string) error {
	_, err := fmt.Fprintf(w, "::%s file=%s,line=%d,title=%s::%s\n",
		level, escapeProperty(file), max(line, 1), escapeProperty(title), escapeData(message))
	return err
}

// partnerTitle is the annotation title for a missing co-change partner.
const partnerTitle = "hc: Frequent co-change partner not in this PR"

// renderPartnerNotices emits one ::notice per coupling pair with exactly one
// side in the changed set, anchored on the changed side and naming the absent
// partner with the changed-side→partner confidence. Both sides changed means
// the co-change happened — silence. Always notice level: coupling can be
// stale and splits can be intentional; a nudge, not an alarm. Envelopes
// without a coupling section behave exactly as before this feature existed.
func renderPartnerNotices(w io.Writer, env schema.Envelope, opts Options) error {
	if env.Coupling == nil {
		return nil
	}
	// "Files this PR touches" is anchor-map membership: anchors.txt lists
	// every changed file when provided; otherwise fall back to the envelope's
	// file paths (which the PR pipeline projection-filters to the changed
	// set), anchoring at line 1 via emitAnnotation.
	anchors := opts.AnchorLines
	if len(anchors) == 0 {
		anchors = make(map[string]int, len(env.Files))
		for _, f := range env.Files {
			anchors[f.Path] = 1
		}
	}
	for _, p := range env.Coupling.Pairs {
		_, inA := anchors[p.A]
		_, inB := anchors[p.B]
		if inA == inB {
			continue
		}
		src, partner, conf := p.A, p.B, p.ConfidenceAB
		if inB {
			src, partner, conf = p.B, p.A, p.ConfidenceBA
		}
		message := fmt.Sprintf(
			"%s changes together with %s in %.0f%% of its commits (%d co-changes), but this PR does not touch %s. Check whether it needs a matching change.",
			src, partner, conf*100, p.Support, partner)
		if err := emitAnnotation(w, "notice", src, anchors[src], partnerTitle, message); err != nil {
			return err
		}
	}
	return nil
}

func quadrantSet(in []string) map[string]bool {
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

// looksLikeBareArray reports whether the first non-whitespace byte is '[' — the
// legacy bare-array form of `hc analyze --json` — so we can return a friendlier
// error than a deep unmarshal failure. Mirrors internal/md.
func looksLikeBareArray(data []byte) bool {
	for _, b := range data {
		if unicode.IsSpace(rune(b)) {
			continue
		}
		return b == '['
	}
	return false
}
