package md

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHasSection(t *testing.T) {
	if HasSection([]byte("# My Doc\nSome text")) {
		t.Error("should not detect section in plain text")
	}
	if !HasSection([]byte("# My Doc\n" + MarkerStart + "\nstuff\n" + MarkerEnd)) {
		t.Error("should detect section with markers")
	}
}

func TestReplaceSection(t *testing.T) {
	original := "# Title\n\nSome intro.\n\n" + MarkerStart + "\nold content\n" + MarkerEnd + "\n\n## Footer\n"
	newContent := MarkerStart + "\nnew content\n" + MarkerEnd + "\n"

	result := ReplaceSection([]byte(original), newContent)
	out := string(result)

	if !strings.Contains(out, "new content") {
		t.Error("should contain new content")
	}
	if strings.Contains(out, "old content") {
		t.Error("should not contain old content")
	}
	if !strings.Contains(out, "# Title") {
		t.Error("should preserve content before markers")
	}
	if !strings.Contains(out, "## Footer") {
		t.Error("should preserve content after markers")
	}
}

func TestReplaceSection_NoMarkers(t *testing.T) {
	original := "# Title\n\nSome text.\n"
	result := ReplaceSection([]byte(original), "new stuff")
	if string(result) != original {
		t.Error("should return original when no markers present")
	}
}

func TestUpsertFile_NewFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "NEW.md")
	content := MarkerStart + "\ntest\n" + MarkerEnd + "\n"

	err := UpsertFile(path, content)
	if err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(path)
	if string(data) != content {
		t.Errorf("expected %q, got %q", content, string(data))
	}
}

func TestUpsertFile_AppendToExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "EXISTING.md")
	if err := os.WriteFile(path, []byte("# My Doc\n\nExisting content.\n"), 0644); err != nil {
		t.Fatal(err)
	}

	content := MarkerStart + "\nreport\n" + MarkerEnd + "\n"
	err := UpsertFile(path, content)
	if err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(path)
	out := string(data)
	if !strings.Contains(out, "Existing content.") {
		t.Error("should preserve existing content")
	}
	if !strings.Contains(out, MarkerStart) {
		t.Error("should append report section")
	}
}

func TestUpsertFile_ReplaceExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "UPDATE.md")
	original := "# Doc\n\n" + MarkerStart + "\nold\n" + MarkerEnd + "\n\n## End\n"
	if err := os.WriteFile(path, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}

	newContent := MarkerStart + "\nnew\n" + MarkerEnd + "\n"
	err := UpsertFile(path, newContent)
	if err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(path)
	out := string(data)
	if strings.Contains(out, "old") {
		t.Error("should replace old content")
	}
	if !strings.Contains(out, "new") {
		t.Error("should contain new content")
	}
	if !strings.Contains(out, "## End") {
		t.Error("should preserve content after section")
	}
}
