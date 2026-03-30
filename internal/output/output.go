package output

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/will/hc/internal/analysis"
)

// FormatFiles writes file scores in the given format.
func FormatFiles(w io.Writer, scores []analysis.FileScore, format string, metric string) error {
	switch format {
	case "json":
		return formatFilesJSON(w, scores, metric)
	case "csv":
		return formatFilesCSV(w, scores, metric)
	default:
		return formatFilesTable(w, scores, metric)
	}
}

// FormatDirs writes directory scores in the given format.
func FormatDirs(w io.Writer, dirs []analysis.DirScore, format string, metric string) error {
	switch format {
	case "json":
		return formatDirsJSON(w, dirs, metric)
	case "csv":
		return formatDirsCSV(w, dirs, metric)
	default:
		return formatDirsTable(w, dirs, metric)
	}
}

func complexityColumnLabel(metric string) string {
	if metric == "loc" {
		return "LINES"
	}
	return "COMPLEXITY"
}

// File table output

func formatFilesTable(w io.Writer, scores []analysis.FileScore, metric string) error {
	col := complexityColumnLabel(metric)
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "QUADRANT\tPATH\tCOMMITS\t%s\tAUTHORS\n", col)
	for _, s := range scores {
		fmt.Fprintf(tw, "%s\t%s\t%d\t%d\t%d\n",
			s.Quadrant, s.Path, s.Commits, s.Complexity, s.Authors)
	}
	return tw.Flush()
}

type fileJSON struct {
	Path       string `json:"path"`
	Commits    int    `json:"commits"`
	Lines      int    `json:"lines"`
	Complexity int    `json:"complexity"`
	Authors    int    `json:"authors"`
	Quadrant   string `json:"quadrant"`
	Metric     string `json:"metric"`
}

func formatFilesJSON(w io.Writer, scores []analysis.FileScore, metric string) error {
	items := make([]fileJSON, len(scores))
	for i, s := range scores {
		items[i] = fileJSON{
			Path:       s.Path,
			Commits:    s.Commits,
			Lines:      s.Lines,
			Complexity: s.Complexity,
			Authors:    s.Authors,
			Quadrant:   s.Quadrant.JSONString(),
			Metric:     metric,
		}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(items)
}

func formatFilesCSV(w io.Writer, scores []analysis.FileScore, metric string) error {
	col := complexityColumnLabel(metric)
	cw := csv.NewWriter(w)
	cw.Write([]string{"QUADRANT", "PATH", "COMMITS", col, "AUTHORS"})
	for _, s := range scores {
		cw.Write([]string{
			s.Quadrant.String(),
			s.Path,
			fmt.Sprintf("%d", s.Commits),
			fmt.Sprintf("%d", s.Complexity),
			fmt.Sprintf("%d", s.Authors),
		})
	}
	cw.Flush()
	return cw.Error()
}

// Dir table output

func formatDirsTable(w io.Writer, dirs []analysis.DirScore, metric string) error {
	col := "TOTAL " + complexityColumnLabel(metric)
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "QUADRANT\tPATH\tFILES\tTOTAL COMMITS\t%s\n", col)
	for _, d := range dirs {
		fmt.Fprintf(tw, "%s\t%s\t%d\t%d\t%d\n",
			d.Quadrant, d.Path, d.Files, d.TotalCommits, d.TotalComplexity)
	}
	return tw.Flush()
}

type dirJSON struct {
	Path            string `json:"path"`
	Files           int    `json:"files"`
	TotalCommits    int    `json:"total_commits"`
	TotalLines      int    `json:"total_lines"`
	TotalComplexity int    `json:"total_complexity"`
	Quadrant        string `json:"quadrant"`
	Metric          string `json:"metric"`
}

func formatDirsJSON(w io.Writer, dirs []analysis.DirScore, metric string) error {
	items := make([]dirJSON, len(dirs))
	for i, d := range dirs {
		items[i] = dirJSON{
			Path:            d.Path,
			Files:           d.Files,
			TotalCommits:    d.TotalCommits,
			TotalLines:      d.TotalLines,
			TotalComplexity: d.TotalComplexity,
			Quadrant:        d.Quadrant.JSONString(),
			Metric:          metric,
		}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(items)
}

func formatDirsCSV(w io.Writer, dirs []analysis.DirScore, metric string) error {
	col := "TOTAL " + complexityColumnLabel(metric)
	cw := csv.NewWriter(w)
	cw.Write([]string{"QUADRANT", "PATH", "FILES", "TOTAL COMMITS", col})
	for _, d := range dirs {
		cw.Write([]string{
			d.Quadrant.String(),
			d.Path,
			fmt.Sprintf("%d", d.Files),
			fmt.Sprintf("%d", d.TotalCommits),
			fmt.Sprintf("%d", d.TotalComplexity),
		})
	}
	cw.Flush()
	return cw.Error()
}
