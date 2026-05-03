package output

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/will-wright-eng/hc/internal/analysis"
)

// FormatFiles writes file scores in the given format.
func FormatFiles(w io.Writer, scores []analysis.FileScore, format string, decay bool) error {
	switch format {
	case "json":
		return formatFilesJSON(w, scores, decay)
	case "csv":
		return formatFilesCSV(w, scores, decay)
	default:
		return formatFilesTable(w, scores, decay)
	}
}

func formatFilesTable(w io.Writer, scores []analysis.FileScore, decay bool) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if decay {
		_, _ = fmt.Fprintln(tw, "QUADRANT\tPATH\tCOMMITS\tSCORE\tLINES\tCOMPLEXITY\tAUTHORS")
		for _, s := range scores {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%d\t%.1f\t%d\t%d\t%d\n",
				s.Quadrant, s.Path, s.Commits, s.WeightedCommits, s.Lines, s.Complexity, s.Authors)
		}
	} else {
		_, _ = fmt.Fprintln(tw, "QUADRANT\tPATH\tCOMMITS\tLINES\tCOMPLEXITY\tAUTHORS")
		for _, s := range scores {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%d\t%d\t%d\t%d\n",
				s.Quadrant, s.Path, s.Commits, s.Lines, s.Complexity, s.Authors)
		}
	}
	return tw.Flush()
}

type fileJSON struct {
	Path            string  `json:"path"`
	Commits         int     `json:"commits"`
	WeightedCommits float64 `json:"weighted_commits,omitempty"`
	Lines           int     `json:"lines"`
	Complexity      int     `json:"complexity"`
	Authors         int     `json:"authors"`
	Quadrant        string  `json:"quadrant"`
}

func formatFilesJSON(w io.Writer, scores []analysis.FileScore, decay bool) error {
	items := make([]fileJSON, len(scores))
	for i, s := range scores {
		items[i] = fileJSON{
			Path:       s.Path,
			Commits:    s.Commits,
			Lines:      s.Lines,
			Complexity: s.Complexity,
			Authors:    s.Authors,
			Quadrant:   s.Quadrant.JSONString(),
		}
		if decay {
			items[i].WeightedCommits = s.WeightedCommits
		}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(items)
}

func formatFilesCSV(w io.Writer, scores []analysis.FileScore, decay bool) error {
	cw := csv.NewWriter(w)
	if decay {
		_ = cw.Write([]string{"QUADRANT", "PATH", "COMMITS", "SCORE", "LINES", "COMPLEXITY", "AUTHORS"})
		for _, s := range scores {
			_ = cw.Write([]string{
				s.Quadrant.String(),
				s.Path,
				fmt.Sprintf("%d", s.Commits),
				fmt.Sprintf("%.1f", s.WeightedCommits),
				fmt.Sprintf("%d", s.Lines),
				fmt.Sprintf("%d", s.Complexity),
				fmt.Sprintf("%d", s.Authors),
			})
		}
	} else {
		_ = cw.Write([]string{"QUADRANT", "PATH", "COMMITS", "LINES", "COMPLEXITY", "AUTHORS"})
		for _, s := range scores {
			_ = cw.Write([]string{
				s.Quadrant.String(),
				s.Path,
				fmt.Sprintf("%d", s.Commits),
				fmt.Sprintf("%d", s.Lines),
				fmt.Sprintf("%d", s.Complexity),
				fmt.Sprintf("%d", s.Authors),
			})
		}
	}
	cw.Flush()
	return cw.Error()
}
