package complexity

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// FileComplexity represents static analysis for a single file.
type FileComplexity struct {
	Path  string
	Lines int
}

// Walk traverses the file tree at root and counts non-blank, non-comment lines
// for each file. It skips hidden directories and common non-source directories.
func Walk(root string) ([]FileComplexity, error) {
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

		if !isSourceFile(info.Name()) {
			return nil
		}

		lines, err := countLines(path)
		if err != nil {
			return nil // skip files we can't read
		}

		if lines > 0 {
			results = append(results, FileComplexity{
				Path:  rel,
				Lines: lines,
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

// countLines counts non-blank lines in a file.
// It makes a best-effort attempt to skip single-line comments for common styles.
func countLines(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	count := 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if isCommentLine(line) {
			continue
		}
		count++
	}
	return count, scanner.Err()
}

func isCommentLine(line string) bool {
	return strings.HasPrefix(line, "//") ||
		strings.HasPrefix(line, "#") ||
		strings.HasPrefix(line, "--") ||
		strings.HasPrefix(line, ";") ||
		strings.HasPrefix(line, "/*") && strings.HasSuffix(line, "*/") ||
		strings.HasPrefix(line, "<!--") && strings.HasSuffix(line, "-->")
}
