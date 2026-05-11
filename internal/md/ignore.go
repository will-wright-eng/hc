package md

import (
	_ "embed"
	"io"
	"strings"
)

//go:embed templates/ignore.md
var ignoreTemplate string

// RenderIgnore writes the .hcignore prompt to w, including a repo summary
// generated from root.
func RenderIgnore(root string, w io.Writer) error {
	parts := strings.SplitN(ignoreTemplate, "{{REPO_SUMMARY}}", 2)

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
