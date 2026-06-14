// Package sarif renders `hc analyze --json` into a SARIF 2.1.0 log so hotspot
// findings can be uploaded to GitHub code scanning (Security tab + PR check).
//
// Like internal/md, it is a consumer of schema.Envelope: it reads the analyze
// JSON and writes SARIF, leaving `analyze` and the JSON contract untouched.
// Findings are file-level — each result anchors at startLine 1 — which is by
// design; the specific hunk a PR touches is not what hc flags. See
// docs/proposals/009-sarif-pr-annotations.md.
package sarif

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"unicode"

	"github.com/will-wright-eng/hc/internal/schema"
)

const (
	schemaURI      = "https://json.schemastore.org/sarif-2.1.0.json"
	sarifVersion   = "2.1.0"
	toolName       = "hc"
	informationURI = "https://github.com/will-wright-eng/hc"
	automationID   = "hc"
)

// rule describes one quadrant's SARIF reportingDescriptor and the severity
// (SARIF level) hc assigns to files in that quadrant. cold-simple is healthy
// and intentionally has no rule, so it never produces a finding.
type rule struct {
	id    string
	name  string
	title string // human label used in result messages
	short string
	full  string
	help  string
	level string // SARIF level: warning informs, note is advisory
}

var rules = map[string]rule{
	"hot-critical": {
		id: "hot-critical", name: "HotCritical", title: "Hot/Critical",
		short: "Hotspot: high churn × high complexity",
		full:  "Frequently changed and structurally complex. Highest maintenance risk and the most valuable refactoring target.",
		help:  "Reduce complexity (extract, simplify) or stabilize the churn driving changes here. Keep PRs small and add tests before modifying.",
		level: "warning",
	},
	"hot-simple": {
		id: "hot-simple", name: "HotSimple", title: "Hot/Simple",
		short: "Churny but simple: high churn × low complexity",
		full:  "Frequently changed but structurally simple — typically configuration or glue code.",
		help:  "Low risk. Watch for churn that signals underlying instability.",
		level: "note",
	},
	"cold-complex": {
		id: "cold-complex", name: "ColdComplex", title: "Cold/Complex",
		short: "Complex but stable: high complexity × low churn",
		full:  "Structurally complex but rarely changed — risky to modify when it eventually needs work.",
		help:  "Add tests and documentation before modifying. Do not refactor proactively.",
		level: "note",
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

// defaultQuadrants mirrors `hc md comment`: the quadrants that represent
// actionable risk. hot-simple is opt-in via Options.Quadrants.
var defaultQuadrants = []string{"hot-critical", "cold-complex"}

// Options configures Render.
type Options struct {
	// Quadrants restricts output to the listed quadrant keys. Empty means the
	// default set (hot-critical, cold-complex). Keys without a rule
	// (cold-simple, or typos) yield no findings.
	Quadrants []string
	// Version populates tool.driver.version. Empty omits it.
	Version string
}

// Render reads the envelope JSON from r (output of `hc analyze --json`),
// filters and sorts files, and writes a SARIF 2.1.0 log to w. A clean repo (no
// matching files) produces a valid log with an empty results array — uploading
// it clears previously reported alerts. Output is deterministic: no timestamps,
// stable ordering.
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
	present := make(map[string]bool)
	for _, f := range env.Files {
		if !want[f.Quadrant] {
			continue
		}
		if _, ok := rules[f.Quadrant]; !ok {
			continue // no rule (cold-simple or unknown) → never a finding
		}
		kept = append(kept, f)
		present[f.Quadrant] = true
	}

	sort.SliceStable(kept, func(i, j int) bool {
		if quadrantRank[kept[i].Quadrant] != quadrantRank[kept[j].Quadrant] {
			return quadrantRank[kept[i].Quadrant] < quadrantRank[kept[j].Quadrant]
		}
		return kept[i].WeightedCommits > kept[j].WeightedCommits
	})

	log := sarifLog{
		Schema:  schemaURI,
		Version: sarifVersion,
		Runs: []run{{
			Tool: toolWrapper{Driver: driver{
				Name:           toolName,
				InformationURI: informationURI,
				Version:        opts.Version,
				Rules:          buildRules(present),
			}},
			AutomationDetails: automationDetails{ID: automationID},
			Results:           buildResults(kept, env.Options.Decay),
		}},
	}

	out, err := json.MarshalIndent(log, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding SARIF: %w", err)
	}
	out = append(out, '\n')
	_, err = w.Write(out)
	return err
}

// buildRules emits one reportingDescriptor per quadrant that produced a
// finding, in canonical rank order.
func buildRules(present map[string]bool) []reportingDescriptor {
	keys := make([]string, 0, len(present))
	for k := range present {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return quadrantRank[keys[i]] < quadrantRank[keys[j]] })

	out := make([]reportingDescriptor, 0, len(keys))
	for _, k := range keys {
		r := rules[k]
		out = append(out, reportingDescriptor{
			ID:                   r.id,
			Name:                 r.name,
			ShortDescription:     textBlock{Text: r.short},
			FullDescription:      textBlock{Text: r.full},
			Help:                 textBlock{Text: r.help},
			DefaultConfiguration: configuration{Level: r.level},
			Properties:           ruleProperties{Tags: []string{"maintainability", "hotspot"}},
		})
	}
	return out
}

// buildResults emits one result per file. Callers guarantee every file's
// quadrant has a rule.
func buildResults(files []schema.File, decay bool) []result {
	out := make([]result, 0, len(files))
	for _, f := range files {
		r := rules[f.Quadrant]
		out = append(out, result{
			RuleID:  r.id,
			Level:   r.level,
			Message: textBlock{Text: message(f, r, decay)},
			Locations: []location{{
				PhysicalLocation: physicalLocation{
					ArtifactLocation: artifactLocation{URI: f.Path},
					Region:           region{StartLine: 1},
				},
			}},
			// Stable per (path, ruleId) so the alert tracks the file across
			// commits even though the line-1 anchor is synthetic. Emitted
			// explicitly because the REST upload API does not auto-populate it.
			PartialFingerprints: map[string]string{
				"primaryLocationLineHash": r.id + ":" + f.Path,
			},
		})
	}
	return out
}

func message(f schema.File, r rule, decay bool) string {
	weighted := ""
	if decay {
		weighted = fmt.Sprintf(", weighted %.1f", f.WeightedCommits)
	}
	return fmt.Sprintf("%s is a %s hotspot (%d commit%s%s, complexity %d, %d author%s).",
		f.Path, r.title, f.Commits, plural(f.Commits), weighted, f.Complexity, f.Authors, plural(f.Authors))
}

func quadrantSet(in []string) map[string]bool {
	set := make(map[string]bool)
	if len(in) == 0 {
		for _, q := range defaultQuadrants {
			set[q] = true
		}
		return set
	}
	for _, q := range in {
		set[q] = true
	}
	return set
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// looksLikeBareArray reports whether the first non-whitespace byte is '[' —
// the legacy bare-array form of `hc analyze --json`, so we can return a
// friendlier error than a deep unmarshal failure. Mirrors internal/md.
func looksLikeBareArray(data []byte) bool {
	for _, b := range data {
		if unicode.IsSpace(rune(b)) {
			continue
		}
		return b == '['
	}
	return false
}
