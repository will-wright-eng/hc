package prompt

import (
	_ "embed"
	"io"
	"strings"
)

//go:embed templates/ignore_file_spec.md
var ignoreFileSpecTemplate string

// IgnoreOpts configures RenderIgnoreFileSpec output.
type IgnoreOpts struct {
	MaxFiles  int
	NoSummary bool
}

// RenderIgnoreFileSpec writes the ignore-file-spec prompt to w,
// optionally including a repo summary generated from root.
func RenderIgnoreFileSpec(root string, w io.Writer, opts IgnoreOpts) error {
	tmpl := ignoreFileSpecTemplate

	if opts.NoSummary {
		// Remove the placeholder entirely.
		tmpl = strings.Replace(tmpl, "{{REPO_SUMMARY}}", "", 1)
		_, err := io.WriteString(w, tmpl)
		return err
	}

	// Split on placeholder so we can inject the summary in the middle.
	parts := strings.SplitN(tmpl, "{{REPO_SUMMARY}}", 2)

	if _, err := io.WriteString(w, parts[0]); err != nil {
		return err
	}

	maxFiles := opts.MaxFiles
	if maxFiles <= 0 {
		maxFiles = 200
	}
	if err := writeSummary(root, w, summaryOpts{MaxFiles: maxFiles}); err != nil {
		return err
	}

	if len(parts) > 1 {
		if _, err := io.WriteString(w, parts[1]); err != nil {
			return err
		}
	}

	return nil
}
