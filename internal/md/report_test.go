package md

import (
	"bytes"
	"strings"
	"testing"
)

const sampleEnvelopeJSON = `{
  "schema_version": "1",
  "generated_at": "2026-01-01T00:00:00Z",
  "repo_root": "/tmp/repo",
  "options": {"decay": false},
  "thresholds": {"churn": 10, "complexity": 200},
  "files": [
    {"path":"src/parser.go","commits":87,"lines":1240,"complexity":1240,"authors":6,"quadrant":"hot-critical"},
    {"path":"src/handlers.go","commits":45,"lines":120,"complexity":120,"authors":3,"quadrant":"hot-simple"},
    {"path":"lib/legacy.go","commits":2,"lines":900,"complexity":900,"authors":1,"quadrant":"cold-complex"},
    {"path":"lib/utils.go","commits":1,"lines":30,"complexity":30,"authors":1,"quadrant":"cold-simple"}
  ]
}`

func TestRender_FileEntries(t *testing.T) {
	var buf bytes.Buffer
	err := Render(strings.NewReader(sampleEnvelopeJSON), &buf, false)
	if err != nil {
		t.Fatal(err)
	}
	out := buf.String()

	if !strings.Contains(out, MarkerStart) {
		t.Error("output should contain start marker")
	}
	if !strings.Contains(out, MarkerEnd) {
		t.Error("output should contain end marker")
	}
	if !strings.Contains(out, "## Codebase Hotspot Analysis") {
		t.Error("output should contain main heading")
	}
	if !strings.Contains(out, "src/parser.go") {
		t.Error("output should contain file paths")
	}
	if !strings.Contains(out, "Critical Hotspots") {
		t.Error("output should contain quadrant headings")
	}
}

func TestRender_BareArrayRejected(t *testing.T) {
	bare := `[{"path":"a.go","commits":1,"lines":1,"complexity":1,"authors":1,"quadrant":"hot-critical"}]`
	var buf bytes.Buffer
	err := Render(strings.NewReader(bare), &buf, false)
	if err == nil {
		t.Fatal("expected bare-array input to be rejected")
	}
	if !strings.Contains(err.Error(), "envelope") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRender_EmptyInput(t *testing.T) {
	empty := `{"schema_version":"1","generated_at":"2026-01-01T00:00:00Z","options":{"decay":false},"thresholds":{"churn":0,"complexity":0},"files":[]}`
	var buf bytes.Buffer
	err := Render(strings.NewReader(empty), &buf, false)
	if err == nil {
		t.Error("expected error for empty files array")
	}
}

func TestRender_WithDecayScores(t *testing.T) {
	input := `{"schema_version":"1","generated_at":"2026-01-01T00:00:00Z","options":{"decay":true},"thresholds":{"churn":0,"complexity":0},"files":[{"path":"a.go","commits":10,"weighted_commits":8.5,"lines":100,"complexity":100,"authors":1,"quadrant":"hot-critical"}]}`
	var buf bytes.Buffer
	err := Render(strings.NewReader(input), &buf, false)
	if err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "Score") {
		t.Error("output should contain Score column header when decay is on")
	}
	if !strings.Contains(out, "8.5") {
		t.Error("output should contain the weighted score value")
	}
}

func TestRender_WithoutDecayScores(t *testing.T) {
	input := `{"schema_version":"1","generated_at":"2026-01-01T00:00:00Z","options":{"decay":false},"thresholds":{"churn":0,"complexity":0},"files":[{"path":"a.go","commits":10,"lines":100,"complexity":100,"authors":1,"quadrant":"hot-critical"}]}`
	var buf bytes.Buffer
	err := Render(strings.NewReader(input), &buf, false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "Score") {
		t.Error("output should not contain Score column when decay is off")
	}
}

func TestRender_EscapesMarkdownSpecialsInPaths(t *testing.T) {
	input := `{"schema_version":"1","generated_at":"2026-01-01T00:00:00Z","options":{"decay":false},"thresholds":{"churn":0,"complexity":0},"files":[
	  {"path":"weird|name.go","commits":5,"lines":10,"complexity":10,"authors":1,"quadrant":"hot-critical"},
	  {"path":"back` + "`" + `tick.go","commits":4,"lines":10,"complexity":10,"authors":1,"quadrant":"hot-critical"},
	  {"path":"slash\\path.go","commits":3,"lines":10,"complexity":10,"authors":1,"quadrant":"hot-critical"}
	]}`
	var buf bytes.Buffer
	if err := Render(strings.NewReader(input), &buf, false); err != nil {
		t.Fatal(err)
	}
	out := buf.String()

	if !strings.Contains(out, `weird\|name.go`) {
		t.Error("pipe in path should be escaped as \\|")
	}
	if !strings.Contains(out, "back\\`tick.go") {
		t.Error("backtick in path should be escaped as \\`")
	}
	if !strings.Contains(out, `slash\\path.go`) {
		t.Error("backslash in path should be escaped as \\\\")
	}

	for _, line := range strings.Split(out, "\n") {
		if !strings.Contains(line, "weird") {
			continue
		}
		bare := strings.ReplaceAll(line, `\|`, "")
		if got, want := strings.Count(bare, "|"), 6; got != want {
			t.Errorf("row for weird path has %d cell boundaries, want %d: %q", got, want, line)
		}
	}
}

func TestRender_Collapsible(t *testing.T) {
	var off bytes.Buffer
	if err := Render(strings.NewReader(sampleEnvelopeJSON), &off, false); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(off.String(), "<details>") {
		t.Error("output should not contain <details> when collapsible is false")
	}

	var on bytes.Buffer
	if err := Render(strings.NewReader(sampleEnvelopeJSON), &on, true); err != nil {
		t.Fatal(err)
	}
	out := on.String()
	if !strings.Contains(out, "<details>") || !strings.Contains(out, "</details>") {
		t.Error("output should wrap categories in <details>...</details> when collapsible is true")
	}
	if !strings.Contains(out, "<summary>Hotspot categories</summary>") {
		t.Error("output should contain a <summary> tag when collapsible is true")
	}
}
