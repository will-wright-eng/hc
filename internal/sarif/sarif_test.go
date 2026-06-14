package sarif

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sampleJSON = `{
  "schema_version": "1",
  "generated_at": "2026-01-01T00:00:00Z",
  "options": {"decay": true},
  "thresholds": {"churn": 5, "complexity": 130},
  "files": [
    {"path":"a.go","commits":5,"weighted_commits":4.2,"lines":100,"complexity":120,"authors":2,"quadrant":"cold-complex"},
    {"path":"b.go","commits":12,"weighted_commits":9.1,"lines":200,"complexity":250,"authors":3,"quadrant":"hot-critical"},
    {"path":"c.go","commits":3,"weighted_commits":2.0,"lines":40,"complexity":50,"authors":1,"quadrant":"hot-simple"},
    {"path":"d.go","commits":8,"weighted_commits":6.5,"lines":150,"complexity":180,"authors":2,"quadrant":"hot-critical"},
    {"path":"e.go","commits":1,"weighted_commits":0.5,"lines":10,"complexity":5,"authors":1,"quadrant":"cold-simple"}
  ]
}`

func render(t *testing.T, in string, opts Options) sarifLog {
	t.Helper()
	var buf bytes.Buffer
	if err := Render(strings.NewReader(in), &buf, opts); err != nil {
		t.Fatalf("Render: %v", err)
	}
	var log sarifLog
	if err := json.Unmarshal(buf.Bytes(), &log); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, buf.String())
	}
	return log
}

func resultPaths(log sarifLog) []string {
	var p []string
	for _, r := range log.Runs[0].Results {
		p = append(p, r.Locations[0].PhysicalLocation.ArtifactLocation.URI)
	}
	return p
}

func TestRender_DefaultsFilterAndOrder(t *testing.T) {
	log := render(t, sampleJSON, Options{})

	// Default = hot-critical + cold-complex. hot-simple (c) and cold-simple (e)
	// are dropped. hot-critical first (weighted desc: b 9.1, d 6.5), then
	// cold-complex (a).
	got := resultPaths(log)
	want := []string{"b.go", "d.go", "a.go"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("result order = %v, want %v", got, want)
	}

	results := log.Runs[0].Results
	if results[0].RuleID != "hot-critical" || results[0].Level != "warning" {
		t.Errorf("b.go: ruleId/level = %q/%q, want hot-critical/warning", results[0].RuleID, results[0].Level)
	}
	if results[2].RuleID != "cold-complex" || results[2].Level != "note" {
		t.Errorf("a.go: ruleId/level = %q/%q, want cold-complex/note", results[2].RuleID, results[2].Level)
	}

	// Whole-file anchor + stable fingerprint.
	for _, r := range results {
		uri := r.Locations[0].PhysicalLocation.ArtifactLocation.URI
		if got := r.Locations[0].PhysicalLocation.Region.StartLine; got != 1 {
			t.Errorf("%s: startLine = %d, want 1", uri, got)
		}
		wantFP := r.RuleID + ":" + uri
		if got := r.PartialFingerprints["primaryLocationLineHash"]; got != wantFP {
			t.Errorf("%s: fingerprint = %q, want %q", uri, got, wantFP)
		}
	}

	// Rules present in rank order, only for emitted quadrants.
	var ruleIDs []string
	for _, rd := range log.Runs[0].Tool.Driver.Rules {
		ruleIDs = append(ruleIDs, rd.ID)
	}
	if strings.Join(ruleIDs, ",") != "hot-critical,cold-complex" {
		t.Errorf("rules = %v, want [hot-critical cold-complex]", ruleIDs)
	}
}

func TestRender_Envelope(t *testing.T) {
	log := render(t, sampleJSON, Options{Version: "1.2.3"})
	if log.Schema == "" || log.Version != "2.1.0" {
		t.Errorf("schema/version = %q/%q", log.Schema, log.Version)
	}
	d := log.Runs[0].Tool.Driver
	if d.Name != "hc" || d.Version != "1.2.3" {
		t.Errorf("driver name/version = %q/%q, want hc/1.2.3", d.Name, d.Version)
	}
	if log.Runs[0].AutomationDetails.ID != "hc" {
		t.Errorf("automationDetails.id = %q, want hc", log.Runs[0].AutomationDetails.ID)
	}
}

func TestRender_QuadrantOverride(t *testing.T) {
	log := render(t, sampleJSON, Options{Quadrants: []string{"hot-critical", "hot-simple"}})
	got := resultPaths(log)
	want := []string{"b.go", "d.go", "c.go"} // hot-critical (desc) then hot-simple
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("result order = %v, want %v", got, want)
	}
}

func TestRender_ColdSimpleNeverEmitted(t *testing.T) {
	log := render(t, sampleJSON, Options{Quadrants: []string{"cold-simple"}})
	if n := len(log.Runs[0].Results); n != 0 {
		t.Fatalf("cold-simple produced %d results, want 0", n)
	}
}

func TestRender_NoDecayMessageOmitsWeighted(t *testing.T) {
	in := `{"schema_version":"1","options":{"decay":false},"thresholds":{"churn":0,"complexity":0},
	  "files":[{"path":"x.go","commits":7,"lines":10,"complexity":200,"authors":1,"quadrant":"hot-critical"}]}`
	log := render(t, in, Options{})
	msg := log.Runs[0].Results[0].Message.Text
	if strings.Contains(msg, "weighted") {
		t.Errorf("message should omit weighted when decay off: %q", msg)
	}
	if !strings.Contains(msg, "7 commits") {
		t.Errorf("message missing commit count: %q", msg)
	}
}

func TestRender_EmptyEmitsValidEmptyLog(t *testing.T) {
	empty := `{"schema_version":"1","options":{"decay":true},"thresholds":{"churn":0,"complexity":0},"files":[]}`
	var buf bytes.Buffer
	if err := Render(strings.NewReader(empty), &buf, Options{}); err != nil {
		t.Fatal(err)
	}
	// Must be a valid, uploadable log that clears prior alerts: empty (not null)
	// results and rules arrays.
	if !strings.Contains(buf.String(), `"results": []`) {
		t.Errorf("expected empty results array, got:\n%s", buf.String())
	}
	var log sarifLog
	if err := json.Unmarshal(buf.Bytes(), &log); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(log.Runs) != 1 || log.Runs[0].Results == nil {
		t.Errorf("expected one run with non-nil results, got %#v", log.Runs)
	}
}

func TestRender_Deterministic(t *testing.T) {
	var a, b bytes.Buffer
	if err := Render(strings.NewReader(sampleJSON), &a, Options{Version: "1.2.3"}); err != nil {
		t.Fatal(err)
	}
	if err := Render(strings.NewReader(sampleJSON), &b, Options{Version: "1.2.3"}); err != nil {
		t.Fatal(err)
	}
	if a.String() != b.String() {
		t.Error("output is not deterministic across runs")
	}
}

func TestRender_RejectsBareArrayAndMissingVersion(t *testing.T) {
	for name, in := range map[string]string{
		"bare array":    `[{"path":"a.go"}]`,
		"no schema_ver": `{"files":[]}`,
		"invalid json":  `{not json`,
	} {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := Render(strings.NewReader(in), &buf, Options{}); err == nil {
				t.Errorf("expected error for %s, got none", name)
			}
		})
	}
}

// TestRenderGolden pins the full SARIF shape. Regenerate intentionally with:
//
//	UPDATE_GOLDEN=1 go test ./internal/sarif
func TestRenderGolden(t *testing.T) {
	var buf bytes.Buffer
	if err := Render(strings.NewReader(sampleJSON), &buf,
		Options{Quadrants: []string{"hot-critical", "hot-simple", "cold-complex"}, Version: "1.2.3"}); err != nil {
		t.Fatal(err)
	}
	got := buf.Bytes()

	goldenPath := filepath.Join("testdata", "hotspots.golden.sarif")
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
	if !bytes.Equal(got, want) {
		t.Errorf("SARIF shape drift — run UPDATE_GOLDEN=1 to inspect.\n--- want\n%s\n+++ got\n%s", want, got)
	}
}
