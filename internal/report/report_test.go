package report

import (
	"bytes"
	"strings"
	"testing"
)

const sampleFileJSON = `[
  {"path":"src/parser.go","commits":87,"lines":1240,"complexity":1240,"authors":6,"quadrant":"hot-critical"},
  {"path":"src/handlers.go","commits":45,"lines":120,"complexity":120,"authors":3,"quadrant":"hot-simple"},
  {"path":"lib/legacy.go","commits":2,"lines":900,"complexity":900,"authors":1,"quadrant":"cold-complex"},
  {"path":"lib/utils.go","commits":1,"lines":30,"complexity":30,"authors":1,"quadrant":"cold-simple"}
]`

const sampleDirJSON = `[
  {"path":"src","files":5,"total_commits":100,"total_lines":2000,"total_complexity":2000,"quadrant":"hot-critical"},
  {"path":"lib","files":3,"total_commits":5,"total_lines":500,"total_complexity":500,"quadrant":"cold-complex"}
]`

func TestRender_FileEntries(t *testing.T) {
	var buf bytes.Buffer
	err := Render(strings.NewReader(sampleFileJSON), &buf)
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

func TestRender_DirEntries(t *testing.T) {
	var buf bytes.Buffer
	err := Render(strings.NewReader(sampleDirJSON), &buf)
	if err != nil {
		t.Fatal(err)
	}
	out := buf.String()

	if !strings.Contains(out, "by directory") {
		t.Error("dir output should indicate directory-level analysis")
	}
	if !strings.Contains(out, "Total Commits") {
		t.Error("dir output should have dir-level column headers")
	}
	if !strings.Contains(out, "src") {
		t.Error("dir output should contain dir paths")
	}
}

func TestRender_EmptyInput(t *testing.T) {
	var buf bytes.Buffer
	err := Render(strings.NewReader("[]"), &buf)
	if err == nil {
		t.Error("expected error for empty input")
	}
}

func TestRender_WithDecayScores(t *testing.T) {
	input := `[{"path":"a.go","commits":10,"weighted_commits":8.5,"lines":100,"complexity":100,"authors":1,"quadrant":"hot-critical"}]`
	var buf bytes.Buffer
	err := Render(strings.NewReader(input), &buf)
	if err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "Score") {
		t.Error("output should contain Score column header when decay scores present")
	}
	if !strings.Contains(out, "8.5") {
		t.Error("output should contain the weighted score value")
	}
}

func TestRender_WithoutDecayScores(t *testing.T) {
	input := `[{"path":"a.go","commits":10,"lines":100,"complexity":100,"authors":1,"quadrant":"hot-critical"}]`
	var buf bytes.Buffer
	err := Render(strings.NewReader(input), &buf)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "Score") {
		t.Error("output should not contain Score column when no decay scores")
	}
}
