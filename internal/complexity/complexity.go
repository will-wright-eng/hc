package complexity

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/will-wright-eng/hc/internal/ignore"
)

// FileComplexity represents static analysis for a single file.
type FileComplexity struct {
	Path       string
	Lines      int // non-blank, non-comment lines
	Complexity int // indent-sum across the same lines
}

// Walk traverses the file tree at root and computes per-file line count and
// indent-sum complexity. It skips hidden and common non-source directories.
func Walk(root string, ig *ignore.Matcher) ([]FileComplexity, error) {
	var results []FileComplexity

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}

		if info.IsDir() {
			base := info.Name()
			if shouldSkipDir(base) {
				return filepath.SkipDir
			}
			return nil
		}

		if ig.Match(rel) {
			return nil
		}

		if !isSourceFile(info.Name()) {
			return nil
		}

		lines, indentSum, err := scanFile(path)
		if err != nil {
			return nil // skip files we can't read
		}

		if lines > 0 {
			results = append(results, FileComplexity{
				Path:       rel,
				Lines:      lines,
				Complexity: indentSum,
			})
		}
		return nil
	})

	return results, err
}

func shouldSkipDir(name string) bool {
	switch name {
	case ".git", ".hg", ".svn", "node_modules", "vendor", "__pycache__", ".idea", ".vscode":
		return true
	}
	return strings.HasPrefix(name, ".")
}

var sourceExtensions = map[string]bool{
	".go": true, ".py": true, ".js": true, ".ts": true, ".tsx": true, ".jsx": true,
	".java": true, ".c": true, ".h": true, ".cpp": true, ".cc": true, ".hpp": true,
	".rs": true, ".rb": true, ".php": true, ".swift": true, ".kt": true, ".kts": true,
	".scala": true, ".cs": true, ".m": true, ".mm": true, ".r": true,
	".sh": true, ".bash": true, ".zsh": true, ".fish": true,
	".sql": true, ".lua": true, ".pl": true, ".pm": true,
	".ex": true, ".exs": true, ".erl": true, ".hs": true, ".ml": true, ".mli": true,
	".dart": true, ".v": true, ".zig": true, ".nim": true, ".cr": true,
	".vue": true, ".svelte": true,
	".yaml": true, ".yml": true, ".toml": true, ".json": true, ".xml": true,
	".html": true, ".css": true, ".scss": true, ".sass": true, ".less": true,
	".md": true, ".txt": true, ".rst": true,
	".proto": true, ".graphql": true, ".gql": true,
	".tf": true, ".hcl": true,
	".Makefile": true, ".cmake": true,
}

func isSourceFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	if sourceExtensions[ext] {
		return true
	}
	// Handle extensionless files like Makefile, Dockerfile, etc.
	base := strings.ToLower(name)
	switch base {
	case "makefile", "dockerfile", "rakefile", "gemfile", "procfile", "vagrantfile":
		return true
	}
	return false
}

func isCommentLine(line string) bool {
	return strings.HasPrefix(line, "//") ||
		strings.HasPrefix(line, "#") ||
		strings.HasPrefix(line, "--") ||
		strings.HasPrefix(line, ";") ||
		strings.HasPrefix(line, "/*") && strings.HasSuffix(line, "*/") ||
		strings.HasPrefix(line, "<!--") && strings.HasSuffix(line, "-->")
}
