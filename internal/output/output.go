package output

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/will-wright-eng/hc/internal/analysis"
	"github.com/will-wright-eng/hc/internal/schema"
)

// ValidateFormat returns an error if format is not a recognized output format.
// Empty string is treated as the default (table) and accepted.
func ValidateFormat(format string) error {
	switch format {
	case "", "table", "json", "csv":
		return nil
	default:
		return fmt.Errorf("unknown output format %q (supported: table, json, csv)", format)
	}
}

// FormatFiles writes file scores in the given format. Unknown formats fall
// back to table; callers that want strict rejection should call ValidateFormat
// first.
//
// JSON output uses the schema.Envelope shape; callers must populate envelope
// for that format. For table/csv, envelope may be the zero value.
func FormatFiles(w io.Writer, scores []analysis.FileScore, format string, decay bool, envelope schema.Envelope) error {
	switch format {
	case "json":
		return FormatJSONEnvelope(w, envelope)
	case "csv":
		return formatFilesCSV(w, scores, decay)
	default:
		return formatFilesTable(w, scores, decay)
	}
}

// FormatJSONEnvelope writes the envelope as indented JSON.
func FormatJSONEnvelope(w io.Writer, env schema.Envelope) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(env)
}

// BuildFiles converts analysis scores into the schema.File slice, applying the
// decay flag the same way the bare-array form used to (weighted_commits is
// emitted only when decay is on).
func BuildFiles(scores []analysis.FileScore, decay bool) []schema.File {
	files := make([]schema.File, len(scores))
	for i, s := range scores {
		files[i] = schema.File{
			Path:       s.Path,
			Commits:    s.Commits,
			Lines:      s.Lines,
			Complexity: s.Complexity,
			Authors:    s.Authors,
			Quadrant:   s.Quadrant.JSONString(),
		}
		if decay {
			files[i].WeightedCommits = s.WeightedCommits
		}
	}
	return files
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
