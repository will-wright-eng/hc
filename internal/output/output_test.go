package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/will-wright-eng/hc/internal/analysis"
	"github.com/will-wright-eng/hc/internal/schema"
)

func sampleScores() []analysis.FileScore {
	return []analysis.FileScore{
		{Path: "src/parser.go", Commits: 87, WeightedCommits: 87, Lines: 1240, Complexity: 1240, Authors: 6, Quadrant: analysis.HotCritical},
		{Path: "src/handlers.go", Commits: 45, WeightedCommits: 45, Lines: 120, Complexity: 120, Authors: 3, Quadrant: analysis.HotSimple},
	}
}

func sampleEnvelope(decay bool) schema.Envelope {
	return schema.Envelope{
		SchemaVersion: schema.SchemaVersion,
		Options:       schema.Options{Decay: decay},
		Thresholds:    schema.Thresholds{Churn: 50, Complexity: 500},
		Files:         BuildFiles(sampleScores(), decay),
	}
}

func TestFormatFilesTable(t *testing.T) {
	var buf bytes.Buffer
	err := FormatFiles(&buf, sampleScores(), "table", false, schema.Envelope{})
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
	err := FormatFiles(&buf, sampleScores(), "json", false, sampleEnvelope(false))
	if err != nil {
		t.Fatal(err)
	}

	var env schema.Envelope
	if err := json.Unmarshal(buf.Bytes(), &env); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if env.SchemaVersion != schema.SchemaVersion {
		t.Errorf("expected schema_version %q, got %q", schema.SchemaVersion, env.SchemaVersion)
	}
	if len(env.Files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(env.Files))
	}
	if env.Files[0].Quadrant != "hot-critical" {
		t.Errorf("expected hot-critical, got %v", env.Files[0].Quadrant)
	}
}

func TestFormatFilesCSV(t *testing.T) {
	var buf bytes.Buffer
	err := FormatFiles(&buf, sampleScores(), "csv", false, schema.Envelope{})
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
