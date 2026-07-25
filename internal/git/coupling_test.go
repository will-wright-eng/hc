package git

import (
	"context"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/will-wright-eng/hc/internal/ignore"
)

func TestLogWithCoupling_CountsSameCommitPairs(t *testing.T) {
	repo := initLogTestRepo(t)
	commitPaths(t, repo, "2020-01-01T00:00:00Z", map[string]string{"a.go": "package a\n", "b.go": "package b\n"})
	commitPaths(t, repo, "2020-01-02T00:00:00Z", map[string]string{"a.go": "package a // v2\n", "b.go": "package b // v2\n"})
	commitPaths(t, repo, "2020-01-03T00:00:00Z", map[string]string{"a.go": "package a // v3\n"})

	res, err := LogWithOptions(context.Background(), LogOptions{RepoPath: repo, Coupling: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Pairs) != 1 {
		t.Fatalf("expected 1 pair, got %d: %+v", len(res.Pairs), res.Pairs)
	}
	p := res.Pairs[0]
	if p.A != "a.go" || p.B != "b.go" {
		t.Errorf("pair = (%s, %s), want (a.go, b.go)", p.A, p.B)
	}
	if p.Support != 2 {
		t.Errorf("support = %d, want 2", p.Support)
	}
	// No decay: weighted == raw.
	if math.Abs(p.WeightedSupport-2.0) > 0.001 {
		t.Errorf("weighted support = %f, want 2.0", p.WeightedSupport)
	}
}

func TestLogWithCoupling_IgnoreRevsSkipsPairsNotChurn(t *testing.T) {
	repo := initLogTestRepo(t)
	commitPaths(t, repo, "2020-01-01T00:00:00Z", map[string]string{"a.go": "package a\n", "b.go": "package b\n"})
	noisy := commitPaths(t, repo, "2020-01-02T00:00:00Z", map[string]string{"a.go": "package a // v2\n", "b.go": "package b // v2\n"})

	revsFile := "# format-everything commit\n" + noisy + "\nnot-a-sha\n\n"
	if err := os.WriteFile(filepath.Join(repo, ignoreRevsFile), []byte(revsFile), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := LogWithOptions(context.Background(), LogOptions{RepoPath: repo, Coupling: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Pairs) != 1 || res.Pairs[0].Support != 1 {
		t.Fatalf("expected pair support 1 (noisy commit skipped), got %+v", res.Pairs)
	}
	for _, c := range res.Churn {
		if c.Path == "a.go" && c.Commits != 2 {
			t.Errorf("a.go churn = %d, want 2 (ignore-revs must not affect churn)", c.Commits)
		}
	}
}

func TestLogWithCoupling_RenameMergesPairKeys(t *testing.T) {
	repo := initLogTestRepo(t)
	commitPaths(t, repo, "2020-01-01T00:00:00Z", map[string]string{"a.go": "package main\nfunc A() {}\n", "b.go": "package main\n"})
	runGit(t, repo, "2020-01-02T00:00:00Z", "mv", "a.go", "c.go")
	runGit(t, repo, "2020-01-02T00:00:00Z", "commit", "-q", "-m", "rename a.go to c.go")
	commitPaths(t, repo, "2020-01-03T00:00:00Z", map[string]string{"c.go": "package main\nfunc A() {}\nfunc B() {}\n", "b.go": "package main\n// v2\n"})

	res, err := LogWithOptions(context.Background(), LogOptions{RepoPath: repo, Coupling: true})
	if err != nil {
		t.Fatal(err)
	}
	// Pre-rename (a.go, b.go) and post-rename (c.go, b.go) merge into one key.
	// The rename commit itself lists a.go and c.go, which resolve to the same
	// path — the self-pair is dropped.
	if len(res.Pairs) != 1 {
		t.Fatalf("expected 1 pair, got %d: %+v", len(res.Pairs), res.Pairs)
	}
	p := res.Pairs[0]
	if p.A != "b.go" || p.B != "c.go" || p.Support != 2 {
		t.Errorf("pair = %+v, want (b.go, c.go) support 2", p)
	}
}

func TestLogWithCoupling_IgnoredPathsNeverPair(t *testing.T) {
	repo := initLogTestRepo(t)
	files := map[string]string{"a.go": "package a\n", "b.go": "package b\n", "gen.pb.go": "package gen\n"}
	commitPaths(t, repo, "2020-01-01T00:00:00Z", files)
	commitPaths(t, repo, "2020-01-02T00:00:00Z", map[string]string{"a.go": "package a // v2\n", "b.go": "package b // v2\n", "gen.pb.go": "package gen // v2\n"})

	res, err := LogWithOptions(context.Background(), LogOptions{
		RepoPath: repo,
		Ignore:   ignore.New([]string{"*.pb.go"}),
		Coupling: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Pairs) != 1 {
		t.Fatalf("expected 1 pair, got %d: %+v", len(res.Pairs), res.Pairs)
	}
	if p := res.Pairs[0]; p.A != "a.go" || p.B != "b.go" {
		t.Errorf("pair = (%s, %s), want (a.go, b.go)", p.A, p.B)
	}
}

func TestLogWithCoupling_DecayWeightsSupport(t *testing.T) {
	repo := initLogTestRepo(t)
	commitPaths(t, repo, "2020-01-01T00:00:00Z", map[string]string{"a.go": "package a\n", "b.go": "package b\n"})
	commitPaths(t, repo, "2020-01-06T00:00:00Z", map[string]string{"a.go": "package a // v2\n", "b.go": "package b // v2\n"})

	res, err := LogWithOptions(context.Background(), LogOptions{
		RepoPath: repo,
		Decay:    true,
		Now:      time.Date(2020, 1, 11, 0, 0, 0, 0, time.UTC),
		Coupling: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Pairs) != 1 {
		t.Fatalf("expected 1 pair, got %d", len(res.Pairs))
	}
	p := res.Pairs[0]
	if p.Support != 2 {
		t.Errorf("support = %d, want 2 (raw count ignores decay)", p.Support)
	}
	// Half-life = window = 10 days: weights 0.5 (10d old) + 2^-0.5 (5d old).
	want := 0.5 + math.Sqrt(0.5)
	if math.Abs(p.WeightedSupport-want) > 0.001 {
		t.Errorf("weighted support = %f, want %f", p.WeightedSupport, want)
	}
}

func TestLoadIgnoreRevs(t *testing.T) {
	dir := t.TempDir()

	revs, err := loadIgnoreRevs(dir)
	if err != nil || revs != nil {
		t.Fatalf("missing file: want (nil, nil), got (%v, %v)", revs, err)
	}

	sha := strings.Repeat("a", 40)
	upper := strings.Repeat("B", 39) + "0"
	content := "# comment\n\n" + sha + "\n" + upper + "\nshort\nzz" + strings.Repeat("f", 38) + "\n"
	if err := os.WriteFile(filepath.Join(dir, ignoreRevsFile), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	revs, err = loadIgnoreRevs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(revs) != 2 {
		t.Fatalf("expected 2 revs (comments/blank/invalid skipped), got %d: %v", len(revs), revs)
	}
	if _, ok := revs[sha]; !ok {
		t.Error("lowercase sha missing")
	}
	if _, ok := revs[strings.ToLower(upper)]; !ok {
		t.Error("uppercase sha should be stored lowercase")
	}
}

// commitPaths writes and commits the given files as a single commit and
// returns its SHA.
func commitPaths(t *testing.T, repo, date string, files map[string]string) string {
	t.Helper()
	for rel, body := range files {
		full := filepath.Join(repo, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
		runGit(t, repo, date, "add", rel)
	}
	runGit(t, repo, date, "commit", "-q", "-m", "commit")
	return headSHA(t, repo)
}

func headSHA(t *testing.T, repo string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = repo
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git rev-parse HEAD: %v", err)
	}
	return strings.TrimSpace(string(out))
}
