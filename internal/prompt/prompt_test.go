package prompt

import (
	"bytes"
	"strings"
	"testing"
)

func TestRenderIgnoreFileSpec_ContainsSyntaxRules(t *testing.T) {
	root := setupFixtureTree(t)
	var buf bytes.Buffer
	err := RenderIgnoreFileSpec(root, &buf, IgnoreOpts{})
	if err != nil {
		t.Fatal(err)
	}
	output := buf.String()

	mustContain := []string{
		".hcignore",
		"filepath.Match",
		"Negation",
		"```text",       // summary fence
		"vendor/lib.go", // from summary largest files
	}
	for _, s := range mustContain {
		if !strings.Contains(output, s) {
			t.Errorf("rendered prompt missing expected content: %q", s)
		}
	}
}

func TestRenderIgnoreFileSpec_NoSummary(t *testing.T) {
	root := setupFixtureTree(t)
	var buf bytes.Buffer
	err := RenderIgnoreFileSpec(root, &buf, IgnoreOpts{NoSummary: true})
	if err != nil {
		t.Fatal(err)
	}
	output := buf.String()

	if strings.Contains(output, "{{REPO_SUMMARY}}") {
		t.Error("--no-summary output should not contain {{REPO_SUMMARY}} placeholder")
	}
	if strings.Contains(output, "```text") {
		t.Error("--no-summary output should not contain repo summary block")
	}
	if !strings.Contains(output, ".hcignore") {
		t.Error("--no-summary output should still contain prompt instructions")
	}
}

func TestRenderIgnoreFileSpec_MaxFiles(t *testing.T) {
	root := setupFixtureTree(t)
	var buf bytes.Buffer
	err := RenderIgnoreFileSpec(root, &buf, IgnoreOpts{MaxFiles: 5})
	if err != nil {
		t.Fatal(err)
	}
	// Should succeed without error; summary is generated with capped file list.
	if buf.Len() == 0 {
		t.Error("expected non-empty output")
	}
}

func TestRenderIgnoreFileSpec_NoPlaceholderRemains(t *testing.T) {
	root := setupFixtureTree(t)
	var buf bytes.Buffer
	err := RenderIgnoreFileSpec(root, &buf, IgnoreOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "{{REPO_SUMMARY}}") {
		t.Error("rendered output should not contain raw {{REPO_SUMMARY}} placeholder")
	}
}
