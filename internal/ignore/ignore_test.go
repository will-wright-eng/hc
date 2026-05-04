package ignore

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNilMatcherNeverMatches(t *testing.T) {
	var m *Matcher
	if m.Match("anything.go") {
		t.Error("nil matcher should never match")
	}
}

func TestNewReturnsNilForEmptyPatterns(t *testing.T) {
	if New(nil) != nil {
		t.Error("New(nil) should return nil")
	}
	if New([]string{}) != nil {
		t.Error("New([]) should return nil")
	}
}

func TestBasenameGlob(t *testing.T) {
	m := New([]string{"*.pb.go"})
	tests := []struct {
		path  string
		match bool
	}{
		{"foo.pb.go", true},
		{"internal/gen/foo.pb.go", true},
		{"foo.go", false},
		{"pb.go", false},
	}
	for _, tt := range tests {
		if got := m.Match(tt.path); got != tt.match {
			t.Errorf("Match(%q) = %v, want %v", tt.path, got, tt.match)
		}
	}
}

func TestDoublestarPrefix(t *testing.T) {
	m := New([]string{"testdata/**"})
	tests := []struct {
		path  string
		match bool
	}{
		{"testdata/a.go", true},
		{"testdata/sub/b.go", true},
		// Unanchored patterns match at any depth — gitignore semantics.
		// Use "/testdata/**" to anchor at the repo root.
		{"src/testdata/a.go", true},
		{"testdata", true},
		{"src/main.go", false},
	}
	for _, tt := range tests {
		if got := m.Match(tt.path); got != tt.match {
			t.Errorf("Match(%q) = %v, want %v", tt.path, got, tt.match)
		}
	}
}

func TestAnchoredPattern(t *testing.T) {
	m := New([]string{"/testdata/**"})
	tests := []struct {
		path  string
		match bool
	}{
		{"testdata/a.go", true},
		{"testdata/sub/b.go", true},
		{"src/testdata/a.go", false},
		{"src/main.go", false},
	}
	for _, tt := range tests {
		if got := m.Match(tt.path); got != tt.match {
			t.Errorf("Match(%q) = %v, want %v", tt.path, got, tt.match)
		}
	}
}

func TestDoublestarSuffix(t *testing.T) {
	m := New([]string{"**/*.pb.go"})
	tests := []struct {
		path  string
		match bool
	}{
		{"foo.pb.go", true},
		{"a/b/c.pb.go", true},
		{"a/b/c.go", false},
	}
	for _, tt := range tests {
		if got := m.Match(tt.path); got != tt.match {
			t.Errorf("Match(%q) = %v, want %v", tt.path, got, tt.match)
		}
	}
}

func TestSpecificSubtree(t *testing.T) {
	m := New([]string{"internal/generated/**"})
	tests := []struct {
		path  string
		match bool
	}{
		{"internal/generated/foo.go", true},
		{"internal/generated/sub/bar.go", true},
		{"internal/other/foo.go", false},
	}
	for _, tt := range tests {
		if got := m.Match(tt.path); got != tt.match {
			t.Errorf("Match(%q) = %v, want %v", tt.path, got, tt.match)
		}
	}
}

func TestDirectoryPattern(t *testing.T) {
	m := New([]string{"docs/"})
	tests := []struct {
		path  string
		match bool
	}{
		{"docs/readme.md", true},
		{"docs/sub/file.txt", true},
		// Unanchored "docs/" matches a docs directory at any depth — use
		// "/docs/" to scope to the repo root only.
		{"src/docs/file.go", true},
		{"documentary.go", false},
	}
	for _, tt := range tests {
		if got := m.Match(tt.path); got != tt.match {
			t.Errorf("Match(%q) = %v, want %v", tt.path, got, tt.match)
		}
	}
}

func TestExactPathPattern(t *testing.T) {
	m := New([]string{"internal/gen/types.go"})
	tests := []struct {
		path  string
		match bool
	}{
		{"internal/gen/types.go", true},
		{"internal/gen/other.go", false},
		{"types.go", false},
	}
	for _, tt := range tests {
		if got := m.Match(tt.path); got != tt.match {
			t.Errorf("Match(%q) = %v, want %v", tt.path, got, tt.match)
		}
	}
}

func TestMultiplePatterns(t *testing.T) {
	m := New([]string{"*.pb.go", "testdata/**", "vendor/"})
	matches := []string{
		"foo.pb.go",
		"testdata/x.go",
		"vendor/lib/a.go",
	}
	for _, p := range matches {
		if !m.Match(p) {
			t.Errorf("expected match for %q", p)
		}
	}
	if m.Match("src/main.go") {
		t.Error("src/main.go should not match")
	}
}

func TestLoadFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".hcignore")

	content := "# comment\n\n*.pb.go\ntestdata/**\n  vendor/  \n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	patterns, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	expected := []string{"*.pb.go", "testdata/**", "vendor/"}
	if len(patterns) != len(expected) {
		t.Fatalf("got %d patterns, want %d", len(patterns), len(expected))
	}
	for i, p := range patterns {
		if p != expected[i] {
			t.Errorf("pattern[%d] = %q, want %q", i, p, expected[i])
		}
	}
}

func TestLoadFileMissing(t *testing.T) {
	patterns, err := LoadFile("/nonexistent/.hcignore")
	if err != nil {
		t.Fatal(err)
	}
	if patterns != nil {
		t.Error("missing file should return nil patterns")
	}
}

// TestNegationPattern: gitignore's last-match-wins rule lets "!keep.go"
// override a broader earlier pattern. Order matters in the pattern slice.
func TestNegationPattern(t *testing.T) {
	m := New([]string{"*.go", "!keep.go"})
	tests := []struct {
		path  string
		match bool
	}{
		{"foo.go", true},
		{"keep.go", false},
		{"sub/keep.go", false},
		{"foo.txt", false},
	}
	for _, tt := range tests {
		if got := m.Match(tt.path); got != tt.match {
			t.Errorf("Match(%q) = %v, want %v", tt.path, got, tt.match)
		}
	}
}

// TestNegationOrder: reversing the order of negation and broad pattern means
// the broad pattern wins (it's last). Confirms order-dependence.
func TestNegationOrder(t *testing.T) {
	m := New([]string{"!keep.go", "*.go"})
	if !m.Match("keep.go") {
		t.Error("expected keep.go to match — broad *.go comes after the negation")
	}
}

// TestEscapedComment: a literal file named "#config" can be ignored by
// escaping the leading hash. The unescaped form is treated as a comment.
func TestEscapedComment(t *testing.T) {
	m := New([]string{`\#config`})
	if !m.Match("#config") {
		t.Error("expected \\#config pattern to match literal #config path")
	}
	if m.Match("config") {
		t.Error("plain config should not match")
	}
}

// TestLeadingSlashAnchor: a leading slash anchors the pattern to the repo
// root. "/main.go" should match top-level main.go but not nested copies.
func TestLeadingSlashAnchor(t *testing.T) {
	m := New([]string{"/main.go"})
	if !m.Match("main.go") {
		t.Error("expected /main.go to match top-level main.go")
	}
	if m.Match("cmd/hc/main.go") {
		t.Error("anchored /main.go should not match nested main.go")
	}
}

// TestLoadFilePreservesEscapes: LoadFile strips comments and blanks but must
// pass escaped-hash patterns through to the matcher.
func TestLoadFilePreservesEscapes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".hcignore")

	content := "# real comment\n\\#literal\n*.go\n!keep.go\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	patterns, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	expected := []string{`\#literal`, "*.go", "!keep.go"}
	if len(patterns) != len(expected) {
		t.Fatalf("got %d patterns, want %d: %v", len(patterns), len(expected), patterns)
	}
	for i, p := range patterns {
		if p != expected[i] {
			t.Errorf("pattern[%d] = %q, want %q", i, p, expected[i])
		}
	}
}
