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
func FormatFiles(w io.Writer, scores []analysis.FileScore, format string) error {
	switch format {
	case "json":
		return formatFilesJSON(w, scores)
	case "csv":
		return formatFilesCSV(w, scores)
	default:
		return formatFilesTable(w, scores)
	}
}

// FormatDirs writes directory scores in the given format.
func FormatDirs(w io.Writer, dirs []analysis.DirScore, format string) error {
	switch format {
	case "json":
		return formatDirsJSON(w, dirs)
	case "csv":
		return formatDirsCSV(w, dirs)
	default:
		return formatDirsTable(w, dirs)
	}
}

// File table output

func formatFilesTable(w io.Writer, scores []analysis.FileScore) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "QUADRANT\tPATH\tCOMMITS\tLINES\tAUTHORS\n")
	for _, s := range scores {
		fmt.Fprintf(tw, "%s\t%s\t%d\t%d\t%d\n",
			s.Quadrant, s.Path, s.Commits, s.Lines, s.Authors)
	}
	return tw.Flush()
}

type fileJSON struct {
	Path     string `json:"path"`
	Commits  int    `json:"commits"`
	Lines    int    `json:"lines"`
	Authors  int    `json:"authors"`
	Quadrant string `json:"quadrant"`
}

func formatFilesJSON(w io.Writer, scores []analysis.FileScore) error {
	items := make([]fileJSON, len(scores))
	for i, s := range scores {
		items[i] = fileJSON{
			Path:     s.Path,
			Commits:  s.Commits,
			Lines:    s.Lines,
			Authors:  s.Authors,
			Quadrant: s.Quadrant.JSONString(),
		}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(items)
}

func formatFilesCSV(w io.Writer, scores []analysis.FileScore) error {
	cw := csv.NewWriter(w)
	cw.Write([]string{"QUADRANT", "PATH", "COMMITS", "LINES", "AUTHORS"})
	for _, s := range scores {
		cw.Write([]string{
			s.Quadrant.String(),
			s.Path,
			fmt.Sprintf("%d", s.Commits),
			fmt.Sprintf("%d", s.Lines),
			fmt.Sprintf("%d", s.Authors),
		})
	}
	cw.Flush()
	return cw.Error()
}

// Dir table output

func formatDirsTable(w io.Writer, dirs []analysis.DirScore) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "QUADRANT\tPATH\tFILES\tTOTAL COMMITS\tTOTAL LINES\n")
	for _, d := range dirs {
		fmt.Fprintf(tw, "%s\t%s\t%d\t%d\t%d\n",
			d.Quadrant, d.Path, d.Files, d.TotalCommits, d.TotalLines)
	}
	return tw.Flush()
}

type dirJSON struct {
	Path         string `json:"path"`
	Files        int    `json:"files"`
	TotalCommits int    `json:"total_commits"`
	TotalLines   int    `json:"total_lines"`
	Quadrant     string `json:"quadrant"`
}

func formatDirsJSON(w io.Writer, dirs []analysis.DirScore) error {
	items := make([]dirJSON, len(dirs))
	for i, d := range dirs {
		items[i] = dirJSON{
			Path:         d.Path,
			Files:        d.Files,
			TotalCommits: d.TotalCommits,
			TotalLines:   d.TotalLines,
			Quadrant:     d.Quadrant.JSONString(),
		}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(items)
}

func formatDirsCSV(w io.Writer, dirs []analysis.DirScore) error {
	cw := csv.NewWriter(w)
	cw.Write([]string{"QUADRANT", "PATH", "FILES", "TOTAL COMMITS", "TOTAL LINES"})
	for _, d := range dirs {
		cw.Write([]string{
			d.Quadrant.String(),
			d.Path,
			fmt.Sprintf("%d", d.Files),
			fmt.Sprintf("%d", d.TotalCommits),
			fmt.Sprintf("%d", d.TotalLines),
		})
	}
	cw.Flush()
	return cw.Error()
}
