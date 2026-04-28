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

// FormatDirs writes directory scores in the given format.
func FormatDirs(w io.Writer, dirs []analysis.DirScore, format string, decay bool) error {
	switch format {
	case "json":
		return formatDirsJSON(w, dirs, decay)
	case "csv":
		return formatDirsCSV(w, dirs, decay)
	default:
		return formatDirsTable(w, dirs, decay)
	}
}

// File table output

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

// Dir table output

func formatDirsTable(w io.Writer, dirs []analysis.DirScore, decay bool) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if decay {
		_, _ = fmt.Fprintln(tw, "QUADRANT\tPATH\tFILES\tTOTAL COMMITS\tSCORE\tTOTAL LINES\tTOTAL COMPLEXITY")
		for _, d := range dirs {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%d\t%d\t%.1f\t%d\t%d\n",
				d.Quadrant, d.Path, d.Files, d.TotalCommits, d.TotalWeightedCommits, d.TotalLines, d.TotalComplexity)
		}
	} else {
		_, _ = fmt.Fprintln(tw, "QUADRANT\tPATH\tFILES\tTOTAL COMMITS\tTOTAL LINES\tTOTAL COMPLEXITY")
		for _, d := range dirs {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%d\t%d\t%d\t%d\n",
				d.Quadrant, d.Path, d.Files, d.TotalCommits, d.TotalLines, d.TotalComplexity)
		}
	}
	return tw.Flush()
}

type dirJSON struct {
	Path                 string  `json:"path"`
	Files                int     `json:"files"`
	TotalCommits         int     `json:"total_commits"`
	TotalWeightedCommits float64 `json:"total_weighted_commits,omitempty"`
	TotalLines           int     `json:"total_lines"`
	TotalComplexity      int     `json:"total_complexity"`
	Quadrant             string  `json:"quadrant"`
}

func formatDirsJSON(w io.Writer, dirs []analysis.DirScore, decay bool) error {
	items := make([]dirJSON, len(dirs))
	for i, d := range dirs {
		items[i] = dirJSON{
			Path:            d.Path,
			Files:           d.Files,
			TotalCommits:    d.TotalCommits,
			TotalLines:      d.TotalLines,
			TotalComplexity: d.TotalComplexity,
			Quadrant:        d.Quadrant.JSONString(),
		}
		if decay {
			items[i].TotalWeightedCommits = d.TotalWeightedCommits
		}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(items)
}

func formatDirsCSV(w io.Writer, dirs []analysis.DirScore, decay bool) error {
	cw := csv.NewWriter(w)
	if decay {
		_ = cw.Write([]string{"QUADRANT", "PATH", "FILES", "TOTAL COMMITS", "SCORE", "TOTAL LINES", "TOTAL COMPLEXITY"})
		for _, d := range dirs {
			_ = cw.Write([]string{
				d.Quadrant.String(),
				d.Path,
				fmt.Sprintf("%d", d.Files),
				fmt.Sprintf("%d", d.TotalCommits),
				fmt.Sprintf("%.1f", d.TotalWeightedCommits),
				fmt.Sprintf("%d", d.TotalLines),
				fmt.Sprintf("%d", d.TotalComplexity),
			})
		}
	} else {
		_ = cw.Write([]string{"QUADRANT", "PATH", "FILES", "TOTAL COMMITS", "TOTAL LINES", "TOTAL COMPLEXITY"})
		for _, d := range dirs {
			_ = cw.Write([]string{
				d.Quadrant.String(),
				d.Path,
				fmt.Sprintf("%d", d.Files),
				fmt.Sprintf("%d", d.TotalCommits),
				fmt.Sprintf("%d", d.TotalLines),
				fmt.Sprintf("%d", d.TotalComplexity),
			})
		}
	}
	cw.Flush()
	return cw.Error()
}
