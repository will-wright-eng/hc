package schema_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/will-wright-eng/hc/internal/schema"
)

// TestEnvelopeGolden pins the on-the-wire shape. If you intentionally change
// the envelope, regenerate the golden with:
//
//	go test ./internal/schema -update
//
// Bump schema.SchemaVersion at the same time if the change is not additive.
func TestEnvelopeGolden(t *testing.T) {
	env := schema.Envelope{
		SchemaVersion: schema.SchemaVersion,
		GeneratedAt:   time.Date(2026, 5, 13, 14, 30, 0, 0, time.UTC),
		RepoRoot:      "/tmp/repo",
		AnalyzedPath:  "internal",
		Options: schema.Options{
			Since:    "6 months",
			Decay:    true,
			MinAge:   "336h0m0s",
			Excludes: []string{"vendor/**", "*.gen.go"},
		},
		Thresholds: schema.Thresholds{
			Churn:      12.5,
			Complexity: 200,
		},
		Files: []schema.File{
			{
				Path:            "internal/foo.go",
				Commits:         42,
				WeightedCommits: 31.4,
				Lines:           500,
				Complexity:      650,
				Authors:         4,
				Quadrant:        "hot-critical",
			},
			{
				Path:       "internal/bar.go",
				Commits:    3,
				Lines:      80,
				Complexity: 40,
				Authors:    1,
				Quadrant:   "cold-simple",
			},
		},
	}

	got, err := json.MarshalIndent(env, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')

	goldenPath := filepath.Join("testdata", "envelope.golden.json")
	if os.Getenv("UPDATE_GOLDEN") != "" {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(goldenPath, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("reading golden: %v (run with UPDATE_GOLDEN=1 to create)", err)
	}

	if string(got) != string(want) {
		t.Errorf("envelope shape drift — diff:\n--- want\n%s\n+++ got\n%s\n",
			snippet(string(want)), snippet(string(got)))
	}
}

func snippet(s string) string {
	const max = 1200
	if len(s) <= max {
		return s
	}
	return s[:max] + "\n... (truncated)"
}

// Verify decoding accepts what encoding produces.
func TestEnvelopeRoundTrip(t *testing.T) {
	original := schema.Envelope{
		SchemaVersion: schema.SchemaVersion,
		GeneratedAt:   time.Date(2026, 5, 13, 14, 30, 0, 0, time.UTC),
		Options:       schema.Options{Decay: true},
		Thresholds:    schema.Thresholds{Churn: 10, Complexity: 100},
		Files: []schema.File{
			{Path: "a.go", Commits: 1, Lines: 10, Complexity: 10, Authors: 1, Quadrant: "hot-critical"},
		},
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var decoded schema.Envelope
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.SchemaVersion != original.SchemaVersion {
		t.Errorf("schema_version: got %q, want %q", decoded.SchemaVersion, original.SchemaVersion)
	}
	if len(decoded.Files) != 1 || decoded.Files[0].Path != "a.go" {
		t.Errorf("files round-trip lost data: %#v", decoded.Files)
	}
}

// Ensure required field names are stable — caught by golden anyway, but this
// makes failures more readable when someone renames a field.
func TestEnvelopeFieldNames(t *testing.T) {
	data, _ := json.Marshal(schema.Envelope{SchemaVersion: schema.SchemaVersion})
	for _, key := range []string{`"schema_version"`, `"generated_at"`, `"repo_root"`, `"options"`, `"thresholds"`, `"files"`} {
		if !strings.Contains(string(data), key) {
			t.Errorf("missing required JSON key %s in: %s", key, data)
		}
	}
}
