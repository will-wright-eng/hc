package complexity

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCountLines(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "test.go")
	content := `package main

import "fmt"

// main is the entry point.
func main() {
	fmt.Println("hello")
}
`
	os.WriteFile(f, []byte(content), 0644)

	lines, err := countLines(f)
	if err != nil {
		t.Fatal(err)
	}

	// Expecting: package main, import "fmt", func main() {, fmt.Println("hello"), }
	// Blank lines and comment lines are excluded
	if lines != 5 {
		t.Errorf("expected 5 non-blank non-comment lines, got %d", lines)
	}
}

func TestWalk(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\nfunc A() {}\n"), 0644)
	os.WriteFile(filepath.Join(dir, "b.py"), []byte("def b():\n    pass\n"), 0644)
	os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("hello\n"), 0644)
	os.WriteFile(filepath.Join(dir, "photo.png"), []byte("not source"), 0644)

	results, err := Walk(dir, "loc")
	if err != nil {
		t.Fatal(err)
	}

	paths := make(map[string]bool)
	for _, r := range results {
		paths[r.Path] = true
	}

	if !paths["a.go"] {
		t.Error("expected a.go in results")
	}
	if !paths["b.py"] {
		t.Error("expected b.py in results")
	}
	if !paths["readme.txt"] {
		t.Error("expected readme.txt in results")
	}
	if paths["photo.png"] {
		t.Error("photo.png should not be in results")
	}
}

func TestShouldSkipDir(t *testing.T) {
	if !shouldSkipDir(".git") {
		t.Error("should skip .git")
	}
	if !shouldSkipDir("node_modules") {
		t.Error("should skip node_modules")
	}
	if shouldSkipDir("src") {
		t.Error("should not skip src")
	}
}

func TestIsCommentLine(t *testing.T) {
	if !isCommentLine("// this is a comment") {
		t.Error("should detect Go/JS comment")
	}
	if !isCommentLine("# python comment") {
		t.Error("should detect Python comment")
	}
	if isCommentLine("x := 1 // inline comment") {
		t.Error("should not detect inline comment as comment-only line")
	}
}
