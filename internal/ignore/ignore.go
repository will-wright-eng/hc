package ignore

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// Matcher tests relative paths against a set of ignore patterns.
type Matcher struct {
	patterns []string
}

// New creates a Matcher from the combined set of patterns.
// Returns nil if patterns is empty, which makes nil-check callers skip filtering.
func New(patterns []string) *Matcher {
	if len(patterns) == 0 {
		return nil
	}
	return &Matcher{patterns: patterns}
}

// Match returns true if the given relative path should be ignored.
// A nil Matcher never matches.
func (m *Matcher) Match(relPath string) bool {
	if m == nil {
		return false
	}
	// Normalize to forward slashes for consistent matching.
	relPath = filepath.ToSlash(relPath)
	for _, p := range m.patterns {
		if matchPattern(p, relPath) {
			return true
		}
	}
	return false
}

// matchPattern checks whether relPath matches pattern using gitignore-style globbing.
// Supports ** for recursive directory matching.
func matchPattern(pattern, relPath string) bool {
	pattern = filepath.ToSlash(pattern)

	// Directory pattern (trailing slash): match if relPath is under that directory.
	if dir, ok := strings.CutSuffix(pattern, "/"); ok {
		if relPath == dir || strings.HasPrefix(relPath, dir+"/") {
			return true
		}
		return false
	}

	// Pattern contains **: split and match prefix/suffix.
	if strings.Contains(pattern, "**") {
		return matchDoublestar(pattern, relPath)
	}

	// Pattern without path separator: match against basename at any depth.
	if !strings.Contains(pattern, "/") {
		base := filepath.Base(relPath)
		if matched, _ := filepath.Match(pattern, base); matched {
			return true
		}
		return false
	}

	// Pattern with path separator but no **: match the full relative path.
	matched, _ := filepath.Match(pattern, relPath)
	return matched
}

// matchDoublestar handles patterns containing **.
func matchDoublestar(pattern, relPath string) bool {
	parts := strings.SplitN(pattern, "**", 2)
	prefix := parts[0]
	suffix := strings.TrimPrefix(parts[1], "/")

	// Check prefix matches.
	if prefix != "" {
		prefix = strings.TrimSuffix(prefix, "/")
		if !strings.HasPrefix(relPath, prefix+"/") && relPath != prefix {
			return false
		}
	}

	// If suffix is empty, everything under prefix matches.
	if suffix == "" {
		return true
	}

	// Try matching suffix against every possible sub-path.
	segments := strings.Split(relPath, "/")
	for i := range segments {
		candidate := strings.Join(segments[i:], "/")
		if matched, _ := filepath.Match(suffix, candidate); matched {
			return true
		}
		// Also try matching just the basename for patterns like **/*.pb.go.
		if matched, _ := filepath.Match(suffix, segments[i]); matched {
			return true
		}
	}
	return false
}

// LoadFile reads patterns from a .hcignore file. Returns nil (no patterns)
// if the file does not exist. Returns an error only for read failures.
func LoadFile(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var patterns []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, line)
	}
	return patterns, scanner.Err()
}
