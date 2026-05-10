package complexity

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanFile(t *testing.T) {
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

	lines, _, err := scanFile(f)
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

	results, err := Walk(dir, nil)
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

func TestWalkWithOptions_CustomSourceFilePolicy(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "a.go"), "package a\n")
	writeTestFile(t, filepath.Join(dir, "config.custom"), "enabled = true\n")

	results, err := WalkWithOptions(dir, Options{
		IsSourceFile: func(name string) bool {
			return name == "config.custom"
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(results) != 1 {
		t.Fatalf("expected one result, got %d: %v", len(results), results)
	}
	if results[0].Path != "config.custom" {
		t.Fatalf("expected custom source file, got %q", results[0].Path)
	}
}

func TestWalkWithOptions_CustomSkipDirPolicy(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "node_modules")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(nested, "a.go"), "package a\n")

	results, err := WalkWithOptions(dir, Options{
		SkipDir: func(name string) bool {
			return false
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	paths := make(map[string]bool)
	for _, r := range results {
		paths[r.Path] = true
	}
	if !paths["node_modules/a.go"] {
		t.Fatalf("expected custom SkipDir policy to include node_modules/a.go, got %v", paths)
	}
}

func TestWalkWithOptions_CustomScanner(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "a.go"), "package a\n")

	results, err := WalkWithOptions(dir, Options{
		ScanFile: func(path string) (int, int, error) {
			return 7, 42, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected one result, got %d", len(results))
	}
	if results[0].Lines != 7 || results[0].Complexity != 42 {
		t.Fatalf("custom scanner not used: %+v", results[0])
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
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
