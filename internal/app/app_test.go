package app

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/will-wright-eng/hc/internal/analysis"
)

// initTestRepo creates a temp git repo with two source files in different
// subdirectories, each with at least one commit, and returns the repo root.
// All commits are dated well in the past so the default min-age floor never
// trips them. Tests still pass NoMinAge=true to be explicit.
func initTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	mustRun := func(cmdName string, args ...string) {
		t.Helper()
		cmd := exec.Command(cmdName, args...)
		cmd.Dir = dir
		// Force commit dates well in the past so age-floor logic never triggers
		// even if a future test forgets NoMinAge.
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test",
			"GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=test",
			"GIT_COMMITTER_EMAIL=test@example.com",
			"GIT_AUTHOR_DATE=2020-01-01T00:00:00Z",
			"GIT_COMMITTER_DATE=2020-01-01T00:00:00Z",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s %v: %v\n%s", cmdName, args, err, out)
		}
	}

	mustRun("git", "init", "-q", "-b", "main")
	mustRun("git", "config", "user.email", "test@example.com")
	mustRun("git", "config", "user.name", "test")
	mustRun("git", "config", "commit.gpgsign", "false")

	writeFile := func(rel, body string) {
		t.Helper()
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	writeFile("internal/foo/foo.go", "package foo\n\nfunc Foo() {\n\treturn\n}\n")
	writeFile("internal/foo/bar.go", "package foo\n\nfunc Bar() {\n\treturn\n}\n")
	writeFile("cmd/main.go", "package main\n\nfunc main() {}\n")

	mustRun("git", "add", ".")
	mustRun("git", "commit", "-q", "-m", "initial")

	// Second commit touches one file so churn varies across files.
	writeFile("internal/foo/foo.go", "package foo\n\nfunc Foo() {\n\treturn\n}\n\nfunc Foo2() {}\n")
	mustRun("git", "add", ".")
	mustRun("git", "commit", "-q", "-m", "second")

	root, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func TestAnalyze_Root_ProducesChurn(t *testing.T) {
	root := initTestRepo(t)

	res, err := Analyze(context.Background(), AnalyzeOptions{
		Path:     root,
		NoMinAge: true,
	})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	if res.Subtree != "" {
		t.Errorf("expected empty subtree at root, got %q", res.Subtree)
	}
	if len(res.Files) == 0 {
		t.Fatalf("expected results, got none")
	}

	var totalCommits int
	for _, f := range res.Files {
		totalCommits += f.Commits
	}
	if totalCommits == 0 {
		t.Errorf("expected non-zero churn across files, got 0")
	}
}

// TestAnalyze_Subdirectory exercises the bug fix: previously, running against
// a subdirectory matched complexity (rel-to-scan-root) against churn
// (rel-to-repo-root) and produced zero churn for every file.
func TestAnalyze_Subdirectory_PreservesChurn(t *testing.T) {
	root := initTestRepo(t)

	res, err := Analyze(context.Background(), AnalyzeOptions{
		Path:     filepath.Join(root, "internal"),
		NoMinAge: true,
	})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	if res.Subtree != "internal" {
		t.Errorf("expected subtree=internal, got %q", res.Subtree)
	}

	if len(res.Files) == 0 {
		t.Fatal("expected results in internal subtree, got none")
	}

	for _, f := range res.Files {
		if !strings.HasPrefix(f.Path, "internal/") {
			t.Errorf("expected path under internal/, got %q", f.Path)
		}
		if f.Commits == 0 {
			t.Errorf("file %s has zero commits — subdirectory churn merge is broken", f.Path)
		}
	}
}

func TestRelSubtree(t *testing.T) {
	tests := []struct {
		name      string
		repoRoot  string
		absPath   string
		want      string
		wantError bool
	}{
		{name: "root itself", repoRoot: "/repo", absPath: "/repo", want: ""},
		{name: "subdirectory", repoRoot: "/repo", absPath: "/repo/internal", want: "internal"},
		{name: "nested file", repoRoot: "/repo", absPath: "/repo/internal/foo.go", want: "internal/foo.go"},
		{name: "outside repo", repoRoot: "/repo", absPath: "/elsewhere", wantError: true},
		{name: "parent of repo", repoRoot: "/repo", absPath: "/", wantError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := relSubtree(tt.repoRoot, tt.absPath)
			if tt.wantError {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// TestAnalyze_FilesFrom_MediansInvariant pins down the projection-vs-scan
// distinction: a file's quadrant when --files-from restricts output must
// match its quadrant in an unfiltered run on the same tree. If the filter
// accidentally short-circuits the walk, medians collapse and classifications
// drift.
func TestAnalyze_FilesFrom_MediansInvariant(t *testing.T) {
	root := initTestRepo(t)

	full, err := Analyze(context.Background(), AnalyzeOptions{
		Path:     root,
		NoMinAge: true,
	})
	if err != nil {
		t.Fatalf("full analyze: %v", err)
	}

	want := map[string]string{}
	for _, f := range full.Files {
		want[f.Path] = f.Quadrant.String()
	}

	for path, quadrant := range want {
		filtered, err := Analyze(context.Background(), AnalyzeOptions{
			Path:      root,
			NoMinAge:  true,
			FilesFrom: []string{path},
		})
		if err != nil {
			t.Fatalf("filtered analyze for %s: %v", path, err)
		}
		if len(filtered.Files) != 1 {
			t.Fatalf("--files-from=%s: expected 1 row, got %d", path, len(filtered.Files))
		}
		if got := filtered.Files[0].Quadrant.String(); got != quadrant {
			t.Errorf("quadrant for %s: filtered=%q full=%q (projection must not change classification)",
				path, got, quadrant)
		}
	}
}

func TestAnalyze_FilesFrom_RowCount(t *testing.T) {
	root := initTestRepo(t)

	res, err := Analyze(context.Background(), AnalyzeOptions{
		Path:      root,
		NoMinAge:  true,
		FilesFrom: []string{"internal/foo/foo.go", "cmd/main.go"},
	})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if len(res.Files) != 2 {
		t.Fatalf("expected 2 rows, got %d (%v)", len(res.Files), pathsOf(res.Files))
	}
}

func TestAnalyze_FilesFrom_MissingPaths(t *testing.T) {
	root := initTestRepo(t)

	res, err := Analyze(context.Background(), AnalyzeOptions{
		Path:      root,
		NoMinAge:  true,
		FilesFrom: []string{"does/not/exist.go"},
	})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if len(res.Files) != 0 {
		t.Fatalf("expected 0 rows for nonexistent path, got %d (%v)", len(res.Files), pathsOf(res.Files))
	}
}

func TestAnalyze_FilesFrom_NormalizesPaths(t *testing.T) {
	root := initTestRepo(t)

	// Mix of forms: ./prefix, plain, and absolute.
	res, err := Analyze(context.Background(), AnalyzeOptions{
		Path:     root,
		NoMinAge: true,
		FilesFrom: []string{
			"./internal/foo/foo.go",
			"cmd/main.go",
			filepath.Join(root, "internal/foo/bar.go"),
		},
	})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if len(res.Files) != 3 {
		t.Fatalf("expected 3 rows, got %d (%v)", len(res.Files), pathsOf(res.Files))
	}
}

func TestNormalizeFilesFrom(t *testing.T) {
	repo := "/repo"
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{name: "plain", in: []string{"a.go", "b/c.go"}, want: []string{"a.go", "b/c.go"}},
		{name: "dot prefix", in: []string{"./a.go"}, want: []string{"a.go"}},
		{name: "absolute inside repo", in: []string{"/repo/a.go"}, want: []string{"a.go"}},
		{name: "blank dropped", in: []string{"", "  ", "a.go"}, want: []string{"a.go"}},
		{name: "outside repo dropped", in: []string{"/elsewhere/a.go"}, want: nil},
		{name: "dot dropped", in: []string{"."}, want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeFilesFrom(tt.in, repo)
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", keysOf(got), tt.want)
			}
			for _, w := range tt.want {
				if _, ok := got[w]; !ok {
					t.Errorf("missing %q in %v", w, keysOf(got))
				}
			}
		})
	}
}

func pathsOf(files []analysis.FileScore) []string {
	out := make([]string, len(files))
	for i, f := range files {
		out[i] = f.Path
	}
	return out
}

func keysOf(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestAnalyze_NotAGitRepo(t *testing.T) {
	dir := t.TempDir()

	_, err := Analyze(context.Background(), AnalyzeOptions{
		Path:     dir,
		NoMinAge: true,
	})
	if err == nil {
		t.Fatal("expected error for non-git directory, got nil")
	}
}
