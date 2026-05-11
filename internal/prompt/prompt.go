package prompt

import (
	_ "embed"
	"io"
	"strings"
)

//go:embed templates/ignore.md
var ignoreTemplate string

// IgnoreOpts configures RenderIgnore output.
type IgnoreOpts struct {
	NoSummary bool
}

// RenderIgnore writes the .hcignore prompt to w,
// optionally including a repo summary generated from root.
func RenderIgnore(root string, w io.Writer, opts IgnoreOpts) error {
	tmpl := ignoreTemplate

	if opts.NoSummary {
		tmpl = strings.Replace(tmpl, "{{REPO_SUMMARY}}", "", 1)
		_, err := io.WriteString(w, tmpl)
		return err
	}

	parts := strings.SplitN(tmpl, "{{REPO_SUMMARY}}", 2)

	if _, err := io.WriteString(w, parts[0]); err != nil {
		return err
	}

	if err := writeSummary(root, w); err != nil {
		return err
	}

	if len(parts) > 1 {
		if _, err := io.WriteString(w, parts[1]); err != nil {
			return err
		}
	}

	return nil
}
