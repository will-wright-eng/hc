package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/will-wright-eng/hc/internal/analysis"
)

func sampleScores() []analysis.FileScore {
	return []analysis.FileScore{
		{Path: "src/parser.go", Commits: 87, WeightedCommits: 87, Lines: 1240, Complexity: 1240, Authors: 6, Quadrant: analysis.HotCritical},
		{Path: "src/handlers.go", Commits: 45, WeightedCommits: 45, Lines: 120, Complexity: 120, Authors: 3, Quadrant: analysis.HotSimple},
	}
}

func TestFormatFilesTable(t *testing.T) {
	var buf bytes.Buffer
	err := FormatFiles(&buf, sampleScores(), "table", "loc", false)
	if err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "QUADRANT") {
		t.Error("table should contain header")
	}
	if !strings.Contains(out, "Hot Critical") {
		t.Error("table should contain quadrant name")
	}
	if !strings.Contains(out, "src/parser.go") {
		t.Error("table should contain file path")
	}
}

func TestFormatFilesJSON(t *testing.T) {
	var buf bytes.Buffer
	err := FormatFiles(&buf, sampleScores(), "json", "loc", false)
	if err != nil {
		t.Fatal(err)
	}

	var items []map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &items); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0]["quadrant"] != "hot-critical" {
		t.Errorf("expected hot-critical, got %v", items[0]["quadrant"])
	}
}

func TestFormatFilesCSV(t *testing.T) {
	var buf bytes.Buffer
	err := FormatFiles(&buf, sampleScores(), "csv", "loc", false)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 3 { // header + 2 rows
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}
	if !strings.Contains(lines[0], "QUADRANT") {
		t.Error("CSV should have header")
	}
}

func TestFormatDirsTable(t *testing.T) {
	dirs := []analysis.DirScore{
		{Path: "src", Files: 5, TotalCommits: 100, TotalLines: 2000, TotalComplexity: 2000, Quadrant: analysis.HotCritical},
	}
	var buf bytes.Buffer
	err := FormatDirs(&buf, dirs, "table", "loc", false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "TOTAL COMMITS") {
		t.Error("dir table should contain TOTAL COMMITS header")
	}
}
