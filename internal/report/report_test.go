package report

import (
	"bytes"
	"strings"
	"testing"
)

const sampleFileJSON = `[
  {"path":"src/parser.go","commits":87,"lines":1240,"complexity":1240,"authors":6,"quadrant":"hot-critical","metric":"loc"},
  {"path":"src/handlers.go","commits":45,"lines":120,"complexity":120,"authors":3,"quadrant":"hot-simple","metric":"loc"},
  {"path":"lib/legacy.go","commits":2,"lines":900,"complexity":900,"authors":1,"quadrant":"cold-complex","metric":"loc"},
  {"path":"lib/utils.go","commits":1,"lines":30,"complexity":30,"authors":1,"quadrant":"cold-simple","metric":"loc"}
]`

const sampleDirJSON = `[
  {"path":"src","files":5,"total_commits":100,"total_lines":2000,"total_complexity":2000,"quadrant":"hot-critical","metric":"loc"},
  {"path":"lib","files":3,"total_commits":5,"total_lines":500,"total_complexity":500,"quadrant":"cold-complex","metric":"loc"}
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
	if !strings.Contains(out, "Complexity metric: **loc**") {
		t.Error("output should contain metric label")
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

func TestRender_Truncation(t *testing.T) {
	// Build JSON with 20 entries in one quadrant.
	var entries []string
	for i := 0; i < 20; i++ {
		entries = append(entries, `{"path":"file`+strings.Repeat("x", i)+`.go","commits":10,"lines":100,"complexity":100,"authors":1,"quadrant":"hot-critical","metric":"loc"}`)
	}
	input := "[" + strings.Join(entries, ",") + "]"

	var buf bytes.Buffer
	err := Render(strings.NewReader(input), &buf)
	if err != nil {
		t.Fatal(err)
	}
	out := buf.String()

	if !strings.Contains(out, "and 5 more") {
		t.Error("output should indicate truncated entries")
	}
}

func TestRender_IndentationMetric(t *testing.T) {
	input := `[{"path":"a.go","commits":10,"lines":100,"complexity":500,"authors":1,"quadrant":"hot-critical","metric":"indentation"}]`
	var buf bytes.Buffer
	err := Render(strings.NewReader(input), &buf)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "Complexity metric: **indentation**") {
		t.Error("output should reflect indentation metric")
	}
}
