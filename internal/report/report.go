package report

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

// fileEntry is the local struct for decoding analyze JSON output.
type fileEntry struct {
	Path            string  `json:"path"`
	Commits         int     `json:"commits"`
	WeightedCommits float64 `json:"weighted_commits,omitempty"`
	Lines           int     `json:"lines"`
	Complexity      int     `json:"complexity"`
	Authors         int     `json:"authors"`
	Quadrant        string  `json:"quadrant"`
}

// dirEntry is the local struct for decoding directory-level analyze JSON output.
type dirEntry struct {
	Path                 string  `json:"path"`
	Files                int     `json:"files"`
	TotalCommits         int     `json:"total_commits"`
	TotalWeightedCommits float64 `json:"total_weighted_commits,omitempty"`
	TotalLines           int     `json:"total_lines"`
	TotalComplexity      int     `json:"total_complexity"`
	Quadrant             string  `json:"quadrant"`
}

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

// Render reads JSON from r (output of hc analyze --format json), groups entries
// by quadrant, and writes structured markdown with sentinel markers to w.
// When collapsible is true, the per-quadrant sections are wrapped in a
// <details> block so they collapse in HTML-rendering markdown viewers.
func Render(r io.Reader, w io.Writer, collapsible bool) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("reading input: %w", err)
	}

	// Try file entries first, fall back to dir entries.
	var fileEntries []fileEntry
	if err := json.Unmarshal(data, &fileEntries); err != nil {
		return fmt.Errorf("parsing JSON: %w", err)
	}

	if len(fileEntries) == 0 {
		return fmt.Errorf("empty JSON input")
	}

	// Detect if this is dir-level JSON by checking for dir-specific fields.
	if isDirJSON(data) {
		var dirEntries []dirEntry
		if err := json.Unmarshal(data, &dirEntries); err != nil {
			return fmt.Errorf("parsing directory JSON: %w", err)
		}
		return renderDirs(w, dirEntries, collapsible)
	}

	return renderFiles(w, fileEntries, collapsible)
}

func isDirJSON(data []byte) bool {
	// Check if the first object has dir-specific keys.
	var raw []map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil || len(raw) == 0 {
		return false
	}
	_, hasFiles := raw[0]["files"]
	_, hasTotalCommits := raw[0]["total_commits"]
	return hasFiles && hasTotalCommits
}

func renderFiles(w io.Writer, entries []fileEntry, collapsible bool) error {
	hasDecay := false
	for _, e := range entries {
		if e.WeightedCommits > 0 {
			hasDecay = true
			break
		}
	}

	grouped := make(map[string][]fileEntry)
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
			if hasDecay {
				sb.WriteString(fmt.Sprintf("| %s | %d | %.1f | %d | %d | %d |\n",
					e.Path, e.Commits, e.WeightedCommits, e.Lines, e.Complexity, e.Authors))
			} else {
				sb.WriteString(fmt.Sprintf("| %s | %d | %d | %d | %d |\n",
					e.Path, e.Commits, e.Lines, e.Complexity, e.Authors))
			}
		}
	}

	if collapsible {
		sb.WriteString("\n</details>\n")
	}

	sb.WriteString(fmt.Sprintf("\n*Generated by `hc report` on %s.*\n", time.Now().Format("2006-01-02")))
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

func renderDirs(w io.Writer, entries []dirEntry, collapsible bool) error {
	hasDecay := false
	for _, e := range entries {
		if e.TotalWeightedCommits > 0 {
			hasDecay = true
			break
		}
	}

	grouped := make(map[string][]dirEntry)
	for _, e := range entries {
		grouped[e.Quadrant] = append(grouped[e.Quadrant], e)
	}

	var sb strings.Builder
	sb.WriteString(MarkerStart + "\n")
	sb.WriteString("## Codebase Hotspot Analysis (by directory)\n\n")
	sb.WriteString("This analysis classifies directories into a 2x2 matrix of **churn** (commit frequency)\n" +
		"x **complexity** (structural size), identifying where development effort concentrates\n" +
		"and where risk accumulates.\n")

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
			sb.WriteString("| Path | Files | Total Commits | Score | Total Lines | Total Complexity |\n")
			sb.WriteString("|------|-------|---------------|-------|-------------|------------------|\n")
		} else {
			sb.WriteString("| Path | Files | Total Commits | Total Lines | Total Complexity |\n")
			sb.WriteString("|------|-------|---------------|-------------|------------------|\n")
		}

		for _, e := range items {
			if hasDecay {
				sb.WriteString(fmt.Sprintf("| %s | %d | %d | %.1f | %d | %d |\n",
					e.Path, e.Files, e.TotalCommits, e.TotalWeightedCommits, e.TotalLines, e.TotalComplexity))
			} else {
				sb.WriteString(fmt.Sprintf("| %s | %d | %d | %d | %d |\n",
					e.Path, e.Files, e.TotalCommits, e.TotalLines, e.TotalComplexity))
			}
		}
	}

	if collapsible {
		sb.WriteString("\n</details>\n")
	}

	sb.WriteString(fmt.Sprintf("\n*Generated by `hc report` on %s.*\n", time.Now().Format("2006-01-02")))
	sb.WriteString(MarkerEnd + "\n")

	_, err := io.WriteString(w, sb.String())
	return err
}
