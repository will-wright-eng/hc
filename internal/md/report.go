package md

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/will-wright-eng/hc/internal/schema"
)

type quadrantInfo struct {
	Key         string
	Title       string
	Description string
}

var quadrantOrder = []quadrantInfo{
	{
		Key:   "hot-critical",
		Title: "Critical Hotspots — High Churn, High Complexity",
		Description: "These files are both complex and actively changing. They carry the highest risk of\n" +
			"defects and are the most expensive to modify. Prioritize test coverage, small\n" +
			"focused PRs, and careful review when working in these files.",
	},
	{
		Key:   "hot-simple",
		Title: "Hot but Simple — High Churn, Low Complexity",
		Description: "Frequently changed but structurally simple. These are typically configuration,\n" +
			"glue code, or generated files. Low risk — changes here are unlikely to introduce\n" +
			"subtle bugs.",
	},
	{
		Key:   "cold-complex",
		Title: "Cold & Complex — Low Churn, High Complexity",
		Description: "Complex files that are rarely modified. They are stable liabilities — safe today,\n" +
			"but risky to change because they are poorly understood. Add tests and documentation\n" +
			"before modifying. Do not refactor proactively.",
	},
	{
		Key:         "cold-simple",
		Title:       "Cold & Simple — Low Churn, Low Complexity",
		Description: "Stable and simple. These files require no special attention.",
	},
}

// Render reads the envelope JSON from r (output of hc analyze --json), groups
// entries by quadrant, and writes structured markdown with sentinel markers to
// w. When collapsible is true, the per-quadrant sections are wrapped in a
// <details> block so they collapse in HTML-rendering markdown viewers.
func Render(r io.Reader, w io.Writer, collapsible bool) error {
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
	if len(env.Files) == 0 {
		return fmt.Errorf("empty JSON input")
	}

	return renderFiles(w, env.Files, env.Options.Decay, collapsible)
}

func renderFiles(w io.Writer, entries []schema.File, hasDecay, collapsible bool) error {
	grouped := make(map[string][]schema.File)
	for _, e := range entries {
		grouped[e.Quadrant] = append(grouped[e.Quadrant], e)
	}

	var sb strings.Builder
	sb.WriteString(MarkerStart + "\n")
	sb.WriteString("## Codebase Hotspot Analysis\n\n")
	sb.WriteString("This analysis classifies files into a 2x2 matrix of **churn** (commit frequency)\n" +
		"x **complexity** (structural size), identifying where development effort concentrates\n" +
		"and where risk accumulates.\n")

	hotCritical := len(grouped["hot-critical"])
	coldComplex := len(grouped["cold-complex"])
	total := len(entries)
	remaining := total - hotCritical - coldComplex

	sb.WriteString("\n")
	if hotCritical > 0 {
		sb.WriteString(fmt.Sprintf("**%d file%s %s high-risk** — they combine frequent changes with high complexity. "+
			"Treat these as the caution zone: keep PRs small, review carefully, and add tests before modifying. ",
			hotCritical, plural(hotCritical), pluralVerb(hotCritical)))
	}
	if coldComplex > 0 {
		sb.WriteString(fmt.Sprintf("**%d file%s %s stable %s** — rarely touched but complex enough to be dangerous when changed. ",
			coldComplex, plural(coldComplex), pluralVerb(coldComplex),
			pluralNoun(coldComplex, "liability", "liabilities")))
	}
	if remaining > 0 {
		sb.WriteString(fmt.Sprintf("The remaining %d file%s %s low-risk.", remaining, plural(remaining), pluralVerb(remaining)))
	}
	sb.WriteString("\n")

	if collapsible {
		sb.WriteString("\n<details>\n")
		sb.WriteString("<summary>Hotspot categories</summary>\n\n")
	}

	for _, q := range quadrantOrder {
		items := grouped[q.Key]
		if len(items) == 0 {
			continue
		}

		sb.WriteString(fmt.Sprintf("\n### %s\n\n", q.Title))
		sb.WriteString(q.Description + "\n\n")
		if hasDecay {
			sb.WriteString("| Path | Commits | Score | Lines | Complexity | Authors |\n")
			sb.WriteString("|------|---------|-------|-------|------------|--------|\n")
		} else {
			sb.WriteString("| Path | Commits | Lines | Complexity | Authors |\n")
			sb.WriteString("|------|---------|-------|------------|--------|\n")
		}

		for _, e := range items {
			path := escapeTableCell(e.Path)
			if hasDecay {
				sb.WriteString(fmt.Sprintf("| %s | %d | %.1f | %d | %d | %d |\n",
					path, e.Commits, e.WeightedCommits, e.Lines, e.Complexity, e.Authors))
			} else {
				sb.WriteString(fmt.Sprintf("| %s | %d | %d | %d | %d |\n",
					path, e.Commits, e.Lines, e.Complexity, e.Authors))
			}
		}
	}

	if collapsible {
		sb.WriteString("\n</details>\n")
	}

	fmt.Fprintf(&sb, "\n*Generated by `hc md report` on %s.*\n", time.Now().Format("2006-01-02"))
	sb.WriteString(MarkerEnd + "\n")

	_, err := io.WriteString(w, sb.String())
	return err
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func pluralVerb(n int) string {
	if n == 1 {
		return "is"
	}
	return "are"
}

func pluralNoun(n int, singular, pluralForm string) string {
	if n == 1 {
		return singular
	}
	return pluralForm
}
