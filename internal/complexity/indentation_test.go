package complexity

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIndentSum_Spaces(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "test.go")
	// indent 0, 1, 2, 3, 2, 1, 0 → code lines at levels: 0,1,2,3,2,1,0 = sum 9
	// but blank/comment lines are excluded
	content := `func complex() {
    for _, v := range x {
        if v > 0 {
            process(v)
        }
    }
}
`
	os.WriteFile(f, []byte(content), 0644)

	sum, err := IndentSum(f)
	if err != nil {
		t.Fatal(err)
	}
	// levels: 0 + 1 + 2 + 3 + 2 + 1 + 0 = 9
	if sum != 9 {
		t.Errorf("expected indent sum 9, got %d", sum)
	}
}

func TestIndentSum_Tabs(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "test.go")
	content := "func main() {\n\tfmt.Println(\"hello\")\n}\n"
	os.WriteFile(f, []byte(content), 0644)

	sum, err := IndentSum(f)
	if err != nil {
		t.Fatal(err)
	}
	// levels: 0 + 1 + 0 = 1
	if sum != 1 {
		t.Errorf("expected indent sum 1, got %d", sum)
	}
}

func TestIndentSum_SkipsBlanksAndComments(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "test.go")
	content := `func main() {

    // a comment
    x := 1
}
`
	os.WriteFile(f, []byte(content), 0644)

	sum, err := IndentSum(f)
	if err != nil {
		t.Fatal(err)
	}
	// code lines: func main() { (0) + x := 1 (1) + } (0) = 1
	if sum != 1 {
		t.Errorf("expected indent sum 1, got %d", sum)
	}
}

func TestIndentSum_FlatFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "flat.py")
	content := "x = 1\ny = 2\nz = 3\n"
	os.WriteFile(f, []byte(content), 0644)

	sum, err := IndentSum(f)
	if err != nil {
		t.Fatal(err)
	}
	if sum != 0 {
		t.Errorf("expected indent sum 0 for flat file, got %d", sum)
	}
}

func TestDetectIndentUnit(t *testing.T) {
	tests := []struct {
		name  string
		lines []string
		want  int
	}{
		{"2-space", []string{"func() {", "  x := 1", "  if true {", "    y := 2", "  }", "}"}, 2},
		{"4-space", []string{"def foo():", "    return 1"}, 4},
		{"no-indent", []string{"x", "y", "z"}, 4}, // default
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectIndentUnit(tt.lines)
			if got != tt.want {
				t.Errorf("detectIndentUnit = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestWalk_IndentSumComplexity(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "deep.go"), []byte("func f() {\n    if true {\n        x()\n    }\n}\n"), 0644)
	os.WriteFile(filepath.Join(dir, "flat.go"), []byte("package flat\nvar x = 1\nvar y = 2\n"), 0644)

	results, err := Walk(dir, nil)
	if err != nil {
		t.Fatal(err)
	}

	m := make(map[string]FileComplexity)
	for _, r := range results {
		m[r.Path] = r
	}

	deep := m["deep.go"]
	flat := m["flat.go"]

	if deep.Complexity <= flat.Complexity {
		t.Errorf("deep.go complexity (%d) should exceed flat.go (%d)", deep.Complexity, flat.Complexity)
	}
	if deep.Lines == 0 || flat.Lines == 0 {
		t.Error("Lines should always be populated alongside Complexity")
	}
}
