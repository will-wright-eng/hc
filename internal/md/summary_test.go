package md

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupFixtureTree creates a small repo-like directory for testing.
func setupFixtureTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	dirs := []string{
		"src",
		"src/pkg",
		"vendor",
		".git",
		"testdata",
	}
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	files := map[string]int{
		"main.go":               200,
		"src/app.go":            500,
		"src/app_test.go":       300,
		"src/pkg/util.go":       100,
		"vendor/lib.go":         8000,
		"go.mod":                50,
		"go.sum":                4000,
		"README.md":             150,
		".git/HEAD":             20,
		"testdata/fixture.json": 2000,
	}
	for name, size := range files {
		path := filepath.Join(root, name)
		data := make([]byte, size)
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	return root
}

func TestWriteSummary_SkipsGitDir(t *testing.T) {
	root := setupFixtureTree(t)
	var buf bytes.Buffer
	if err := writeSummary(root, &buf); err != nil {
		t.Fatal(err)
	}
	output := buf.String()
	if strings.Contains(output, ".git/HEAD") || strings.Contains(output, ".git") {
		t.Error("summary should not include .git directory contents")
	}
}

func TestWriteSummary_ContainsFencedBlock(t *testing.T) {
	root := setupFixtureTree(t)
	var buf bytes.Buffer
	if err := writeSummary(root, &buf); err != nil {
		t.Fatal(err)
	}
	output := buf.String()
	if !strings.HasPrefix(output, "```text\n") {
		t.Error("summary should start with ```text fence")
	}
	if !strings.HasSuffix(output, "```\n") {
		t.Error("summary should end with ``` fence")
	}
}

func TestWriteSummary_ExtensionHistogram(t *testing.T) {
	root := setupFixtureTree(t)
	var buf bytes.Buffer
	if err := writeSummary(root, &buf); err != nil {
		t.Fatal(err)
	}
	output := buf.String()
	if !strings.Contains(output, ".go") {
		t.Error("extension histogram should contain .go")
	}
}

func TestWriteSummary_LargestFiles(t *testing.T) {
	root := setupFixtureTree(t)
	var buf bytes.Buffer
	if err := writeSummary(root, &buf); err != nil {
		t.Fatal(err)
	}
	output := buf.String()
	if !strings.Contains(output, "vendor/lib.go") {
		t.Error("largest files should include vendor/lib.go (8000 bytes)")
	}
}

func TestWriteSummary_NotableFiles(t *testing.T) {
	root := setupFixtureTree(t)
	var buf bytes.Buffer
	if err := writeSummary(root, &buf); err != nil {
		t.Fatal(err)
	}
	output := buf.String()
	if !strings.Contains(output, "go.sum") {
		t.Error("notable files should include go.sum")
	}
	if !strings.Contains(output, "go.mod") {
		t.Error("notable files should include go.mod")
	}
}

func TestWriteSummary_DirectoryTree(t *testing.T) {
	root := setupFixtureTree(t)
	var buf bytes.Buffer
	if err := writeSummary(root, &buf); err != nil {
		t.Fatal(err)
	}
	output := buf.String()
	if !strings.Contains(output, "Directory structure") {
		t.Error("summary should contain directory structure section")
	}
}

func TestFormatSize(t *testing.T) {
	tests := []struct {
		bytes int64
		want  string
	}{
		{500, "500 B"},
		{1536, "1.5 KB"},
		{2_097_152, "2.0 MB"},
	}
	for _, tt := range tests {
		got := formatSize(tt.bytes)
		if got != tt.want {
			t.Errorf("formatSize(%d) = %q, want %q", tt.bytes, got, tt.want)
		}
	}
}
