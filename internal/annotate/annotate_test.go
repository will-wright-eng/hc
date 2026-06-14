package annotate

import (
	"bytes"
	"strings"
	"testing"
)

const sampleAnalyzeJSON = `{
  "schema_version": "1",
  "generated_at": "2026-01-01T00:00:00Z",
  "options": {"decay": true},
  "thresholds": {"churn": 5, "complexity": 130},
  "files": [
    {"path":"a.go","commits":5,"weighted_commits":4.2,"lines":100,"complexity":120,"authors":2,"quadrant":"cold-complex"},
    {"path":"b.go","commits":12,"weighted_commits":9.1,"lines":200,"complexity":250,"authors":3,"quadrant":"hot-critical"},
    {"path":"c.go","commits":3,"weighted_commits":2.0,"lines":40,"complexity":50,"authors":1,"quadrant":"hot-simple"},
    {"path":"d.go","commits":8,"weighted_commits":6.5,"lines":150,"complexity":180,"authors":2,"quadrant":"hot-critical"}
  ]
}`

func annotationLines(t *testing.T, in string, opts Options) []string {
	t.Helper()
	var buf bytes.Buffer
	if err := Render(strings.NewReader(in), &buf, opts); err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := strings.TrimRight(buf.String(), "\n")
	if out == "" {
		return nil
	}
	return strings.Split(out, "\n")
}

func TestRender_DefaultsFilterAndOrder(t *testing.T) {
	lines := annotationLines(t, sampleAnalyzeJSON, Options{})

	// Default = hot-critical + cold-complex; hot-simple "c.go" is dropped.
	// hot-critical first (weighted desc: b 9.1, d 6.5), then cold-complex a.
	if len(lines) != 3 {
		t.Fatalf("expected 3 annotations, got %d: %v", len(lines), lines)
	}
	wantFile := []string{"b.go", "d.go", "a.go"}
	wantLevel := []string{"warning", "warning", "notice"}
	for i, ln := range lines {
		if !strings.Contains(ln, "file="+wantFile[i]+",") {
			t.Errorf("annotation %d: want file %s, got %q", i, wantFile[i], ln)
		}
		if !strings.HasPrefix(ln, "::"+wantLevel[i]+" ") {
			t.Errorf("annotation %d: want level %s, got %q", i, wantLevel[i], ln)
		}
	}
}

func TestRender_Format(t *testing.T) {
	lines := annotationLines(t, sampleAnalyzeJSON, Options{Quadrants: []string{"hot-critical"}})
	want := "::warning file=b.go,line=1,title=Hot/Critical hotspot::b.go was already a Hot/Critical hotspot on the base branch: high churn and high complexity. Keep the diff focused, lean on tests, and review changes here carefully. (commits 12, weighted 9.1, complexity 250, authors 3)"
	if lines[0] != want {
		t.Errorf("format mismatch:\n got: %s\nwant: %s", lines[0], want)
	}
}

func TestRender_Escaping(t *testing.T) {
	// Path with a comma, colon, and percent: those must be escaped in the
	// `file=` property; in the message, only '%' is escaped (':' and ',' stay).
	in := `{"schema_version":"1","options":{"decay":false},"thresholds":{"churn":0,"complexity":0},
	  "files":[{"path":"weird,name:v%1.go","commits":2,"complexity":200,"authors":1,"quadrant":"hot-critical"}]}`
	lines := annotationLines(t, in, Options{})
	if len(lines) != 1 {
		t.Fatalf("expected 1 annotation, got %d", len(lines))
	}
	ln := lines[0]
	if !strings.Contains(ln, "file=weird%2Cname%3Av%251.go,line=1,") {
		t.Errorf("file property not fully escaped: %q", ln)
	}
	// Message data keeps ':' and ',' but escapes '%'.
	if !strings.Contains(ln, "::weird,name:v%251.go was already") {
		t.Errorf("message escaping wrong: %q", ln)
	}
}

func TestRender_AnchorLines(t *testing.T) {
	in := `{"schema_version":"1","options":{"decay":true},"thresholds":{"churn":0,"complexity":0},
	  "files":[
	    {"path":"anchored.go","commits":5,"weighted_commits":4.0,"complexity":200,"authors":1,"quadrant":"hot-critical"},
	    {"path":"fallback.go","commits":4,"weighted_commits":3.0,"complexity":200,"authors":1,"quadrant":"hot-critical"}
	  ]}`
	lines := annotationLines(t, in, Options{AnchorLines: map[string]int{"anchored.go": 42}})
	if !strings.Contains(lines[0], "file=anchored.go,line=42,") {
		t.Errorf("anchored.go should use line 42: %q", lines[0])
	}
	if !strings.Contains(lines[1], "file=fallback.go,line=1,") {
		t.Errorf("fallback.go should default to line 1: %q", lines[1])
	}
}

func TestRender_QuadrantOverride(t *testing.T) {
	lines := annotationLines(t, sampleAnalyzeJSON, Options{Quadrants: []string{"cold-complex"}})
	if len(lines) != 1 || !strings.Contains(lines[0], "file=a.go,") {
		t.Fatalf("expected only a.go, got %v", lines)
	}
}

func TestRender_EmptyQuadrantFallsBackToDefault(t *testing.T) {
	lines := annotationLines(t, sampleAnalyzeJSON, Options{Quadrants: []string{""}})
	if len(lines) != 3 {
		t.Fatalf("empty --quadrant should use the default set (3 annotations), got %d", len(lines))
	}
}

func TestRender_NoDecayStatsOmitWeighted(t *testing.T) {
	in := `{"schema_version":"1","options":{"decay":false},"thresholds":{"churn":0,"complexity":0},
	  "files":[{"path":"x.go","commits":7,"complexity":200,"authors":2,"quadrant":"hot-critical"}]}`
	lines := annotationLines(t, in, Options{})
	if strings.Contains(lines[0], "weighted") {
		t.Errorf("no-decay should omit weighted: %q", lines[0])
	}
	if !strings.Contains(lines[0], "(commits 7, complexity 200, authors 2)") {
		t.Errorf("stats suffix wrong: %q", lines[0])
	}
}

func TestRender_EmptyEmitsNothing(t *testing.T) {
	empty := `{"schema_version":"1","options":{"decay":false},"thresholds":{"churn":0,"complexity":0},"files":[]}`
	var buf bytes.Buffer
	if err := Render(strings.NewReader(empty), &buf, Options{}); err != nil {
		t.Fatal(err)
	}
	if buf.Len() != 0 {
		t.Errorf("expected no output, got %q", buf.String())
	}
}

func TestRender_RejectsBareArray(t *testing.T) {
	var buf bytes.Buffer
	if err := Render(strings.NewReader(`[{"path":"a.go"}]`), &buf, Options{}); err == nil {
		t.Error("expected an error for a bare JSON array")
	}
}
