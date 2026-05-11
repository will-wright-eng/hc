package md

import (
	"bytes"
	"strings"
	"testing"
)

func TestRenderIgnore_ContainsSyntaxRules(t *testing.T) {
	root := setupFixtureTree(t)
	var buf bytes.Buffer
	err := RenderIgnore(root, &buf)
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

func TestRenderIgnore_NoPlaceholderRemains(t *testing.T) {
	root := setupFixtureTree(t)
	var buf bytes.Buffer
	err := RenderIgnore(root, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "{{REPO_SUMMARY}}") {
		t.Error("rendered output should not contain raw {{REPO_SUMMARY}} placeholder")
	}
}
