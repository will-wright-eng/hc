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

const couplingEnvelope = `{
  "schema_version": "1",
  "options": {"decay": true, "coupling": true},
  "thresholds": {"churn": 5, "complexity": 130},
  "files": [
    {"path":"internal/git/git.go","commits":15,"weighted_commits":9.1,"lines":300,"complexity":400,"authors":3,"quadrant":"cold-simple"}
  ],
  "coupling": {
    "min_support": 5,
    "min_confidence": 0.5,
    "pairs": [
      {"a":"internal/git/git.go","b":"internal/git/git_test.go","support":12,"weighted_support":7.3,"confidence_a_b":0.8,"confidence_b_a":0.6}
    ]
  }
}`

func TestRender_PartnerNoticeGolden(t *testing.T) {
	lines := annotationLines(t, couplingEnvelope, Options{
		AnchorLines: map[string]int{"internal/git/git.go": 12},
	})
	if len(lines) != 1 {
		t.Fatalf("expected 1 partner notice, got %d: %v", len(lines), lines)
	}
	// "80%" is emitted as "80%25" — '%' must be escaped in workflow-command
	// data; GitHub renders it back as '%'.
	want := "::notice file=internal/git/git.go,line=12,title=hc%3A Frequent co-change partner not in this PR::internal/git/git.go changes together with internal/git/git_test.go in 80%25 of its commits (12 co-changes), but this PR does not touch internal/git/git_test.go. Check whether it needs a matching change."
	if lines[0] != want {
		t.Errorf("partner notice mismatch:\n got: %s\nwant: %s", lines[0], want)
	}
}

func TestRender_PartnerNoticeDirection(t *testing.T) {
	// The other side changed: anchor on git_test.go, use confidence_b_a (0.6).
	lines := annotationLines(t, couplingEnvelope, Options{
		AnchorLines: map[string]int{"internal/git/git_test.go": 7},
	})
	if len(lines) != 1 {
		t.Fatalf("expected 1 partner notice, got %d: %v", len(lines), lines)
	}
	if !strings.Contains(lines[0], "file=internal/git/git_test.go,line=7,") {
		t.Errorf("notice should anchor on the changed side: %q", lines[0])
	}
	if !strings.Contains(lines[0], "in 60%25 of its commits") {
		t.Errorf("notice should use the changed-side confidence (0.6): %q", lines[0])
	}
	if !strings.Contains(lines[0], "does not touch internal/git/git.go.") {
		t.Errorf("notice should name the absent partner: %q", lines[0])
	}
}

func TestRender_PartnerNoticeBothSidesChangedIsSilent(t *testing.T) {
	lines := annotationLines(t, couplingEnvelope, Options{
		AnchorLines: map[string]int{"internal/git/git.go": 3, "internal/git/git_test.go": 9},
	})
	if len(lines) != 0 {
		t.Errorf("both sides changed should emit nothing, got %v", lines)
	}
}

func TestRender_PartnerNoticeFallsBackToEnvelopeFiles(t *testing.T) {
	// No anchor lines: the changed set is the envelope's files (git.go only),
	// and the anchor falls back to line 1.
	lines := annotationLines(t, couplingEnvelope, Options{})
	if len(lines) != 1 {
		t.Fatalf("expected 1 partner notice, got %d: %v", len(lines), lines)
	}
	if !strings.Contains(lines[0], "file=internal/git/git.go,line=1,") {
		t.Errorf("expected fallback anchor on git.go line 1: %q", lines[0])
	}
}

func TestRender_PartnerNoticeAfterHotspots(t *testing.T) {
	in := `{
	  "schema_version": "1",
	  "options": {"decay": true, "coupling": true},
	  "thresholds": {"churn": 5, "complexity": 130},
	  "files": [
	    {"path":"hot.go","commits":12,"weighted_commits":9.1,"lines":200,"complexity":250,"authors":3,"quadrant":"hot-critical"}
	  ],
	  "coupling": {"min_support":5,"min_confidence":0.5,"pairs":[
	    {"a":"hot.go","b":"hot_test.go","support":6,"weighted_support":4.0,"confidence_a_b":0.7,"confidence_b_a":0.9}
	  ]}
	}`
	lines := annotationLines(t, in, Options{AnchorLines: map[string]int{"hot.go": 5}})
	if len(lines) != 2 {
		t.Fatalf("expected hotspot warning + partner notice, got %d: %v", len(lines), lines)
	}
	if !strings.HasPrefix(lines[0], "::warning ") || !strings.Contains(lines[0], "Hot/Critical") {
		t.Errorf("hotspot warning should come first: %q", lines[0])
	}
	if !strings.HasPrefix(lines[1], "::notice ") || !strings.Contains(lines[1], "co-change") {
		t.Errorf("partner notice should come second: %q", lines[1])
	}
}

func TestRender_PartnerNoticeEscaping(t *testing.T) {
	in := `{
	  "schema_version": "1",
	  "options": {"decay": false, "coupling": true},
	  "thresholds": {"churn": 0, "complexity": 0},
	  "files": [{"path":"a.go","commits":9,"complexity":10,"authors":1,"quadrant":"cold-simple"}],
	  "coupling": {"min_support":5,"min_confidence":0.5,"pairs":[
	    {"a":"a.go","b":"weird,name:v%1.go","support":6,"weighted_support":6,"confidence_a_b":0.7,"confidence_b_a":0.7}
	  ]}
	}`
	lines := annotationLines(t, in, Options{})
	if len(lines) != 1 {
		t.Fatalf("expected 1 partner notice, got %d: %v", len(lines), lines)
	}
	// Partner path appears only in the message: '%' escaped, ':' and ',' kept.
	if !strings.Contains(lines[0], "changes together with weird,name:v%251.go in 70%") {
		t.Errorf("partner path escaping wrong: %q", lines[0])
	}
}

func TestRender_NoCouplingSectionUnchanged(t *testing.T) {
	// Same envelope minus the coupling section: output must be byte-identical
	// to the pre-coupling behavior (no partner notices at all).
	for _, ln := range annotationLines(t, sampleAnalyzeJSON, Options{}) {
		if strings.Contains(ln, "co-change") {
			t.Errorf("envelope without coupling section must not emit partner notices: %q", ln)
		}
	}
}
