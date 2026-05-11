package md

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

const sampleAnalyzeJSON = `[
  {"path":"a.go","commits":5,"weighted_commits":4.2,"lines":100,"complexity":120,"authors":2,"quadrant":"cold-complex"},
  {"path":"b.go","commits":12,"weighted_commits":9.1,"lines":200,"complexity":250,"authors":3,"quadrant":"hot-critical"},
  {"path":"c.go","commits":3,"weighted_commits":2.0,"lines":40,"complexity":50,"authors":1,"quadrant":"hot-simple"},
  {"path":"d.go","commits":8,"weighted_commits":6.5,"lines":150,"complexity":180,"authors":2,"quadrant":"hot-critical"}
]`

func decodeEntries(t *testing.T, ndjson string) []CommentEntry {
	t.Helper()
	var entries []CommentEntry
	for _, line := range strings.Split(strings.TrimRight(ndjson, "\n"), "\n") {
		if line == "" {
			continue
		}
		var e CommentEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("decoding line %q: %v", line, err)
		}
		entries = append(entries, e)
	}
	return entries
}

func TestRenderComments_DefaultsFilterAndOrder(t *testing.T) {
	var buf bytes.Buffer
	err := RenderComments(strings.NewReader(sampleAnalyzeJSON), &buf, CommentOpts{})
	if err != nil {
		t.Fatal(err)
	}
	entries := decodeEntries(t, buf.String())

	// Default quadrants = hot-critical + cold-complex; hot-simple "c.go" is dropped.
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	// Hot-critical first (sorted by weighted_commits desc), then cold-complex.
	wantOrder := []string{"b.go", "d.go", "a.go"}
	for i, e := range entries {
		if e.Path != wantOrder[i] {
			t.Errorf("entry %d: got path %q, want %q", i, e.Path, wantOrder[i])
		}
	}
}

func TestRenderComments_QuadrantOverride(t *testing.T) {
	var buf bytes.Buffer
	err := RenderComments(strings.NewReader(sampleAnalyzeJSON), &buf, CommentOpts{
		Quadrants: []string{"cold-complex"},
	})
	if err != nil {
		t.Fatal(err)
	}
	entries := decodeEntries(t, buf.String())

	if len(entries) != 1 || entries[0].Path != "a.go" {
		t.Fatalf("expected only a.go, got %#v", entries)
	}
}

func TestRenderComments_BodyContainsTagAndTemplate(t *testing.T) {
	var buf bytes.Buffer
	err := RenderComments(strings.NewReader(sampleAnalyzeJSON), &buf, CommentOpts{})
	if err != nil {
		t.Fatal(err)
	}
	entries := decodeEntries(t, buf.String())

	for _, e := range entries {
		// Tag at top level matches embedded tag at end of body.
		wantTag := "<!-- hc-pr-comment:" + e.Path + " -->"
		if e.Tag != wantTag {
			t.Errorf("%s: tag = %q, want %q", e.Path, e.Tag, wantTag)
		}
		if !strings.HasSuffix(strings.TrimRight(e.Body, "\n"), e.Tag) {
			t.Errorf("%s: body does not end with tag", e.Path)
		}
		// Stats table replaced the placeholder.
		if strings.Contains(e.Body, "<!-- hc-stats -->") {
			t.Errorf("%s: placeholder not substituted", e.Path)
		}
		if !strings.Contains(e.Body, "| Field | Value |") {
			t.Errorf("%s: stats table missing", e.Path)
		}
		// Template wording survived.
		switch e.Quadrant {
		case "hot-critical":
			if !strings.Contains(e.Body, "Hot Critical") {
				t.Errorf("%s: hot-critical body missing template wording", e.Path)
			}
		case "cold-complex":
			if !strings.Contains(e.Body, "Cold Complex") {
				t.Errorf("%s: cold-complex body missing template wording", e.Path)
			}
		}
	}
}

func TestRenderComments_StatsTableDynamic(t *testing.T) {
	// Add an unknown field; the renderer should pick it up without code changes.
	input := `[{"path":"x.go","commits":1,"new_metric":42,"complexity":1,"quadrant":"hot-critical"}]`
	var buf bytes.Buffer
	if err := RenderComments(strings.NewReader(input), &buf, CommentOpts{}); err != nil {
		t.Fatal(err)
	}
	entries := decodeEntries(t, buf.String())
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	body := entries[0].Body
	for _, want := range []string{"| Path | x.go |", "| Commits | 1 |", "| New Metric | 42 |"} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing row %q\n%s", want, body)
		}
	}
}

func TestRenderComments_EmptyInputEmitsNothing(t *testing.T) {
	var buf bytes.Buffer
	if err := RenderComments(strings.NewReader("[]"), &buf, CommentOpts{}); err != nil {
		t.Fatal(err)
	}
	if buf.Len() != 0 {
		t.Errorf("expected zero output, got %q", buf.String())
	}
}

func TestHumanize(t *testing.T) {
	cases := map[string]string{
		"path":             "Path",
		"weighted_commits": "Weighted Commits",
		"author_count":     "Author Count",
		"":                 "",
	}
	for in, want := range cases {
		if got := humanize(in); got != want {
			t.Errorf("humanize(%q) = %q, want %q", in, got, want)
		}
	}
}
