package git

import (
	"context"
	"errors"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestLogWithOptions_UsesNowForDecay(t *testing.T) {
	repo := initLogTestRepo(t)
	commitFile(t, repo, "old.go", "package old\n", "2020-01-01T00:00:00Z")
	commitFile(t, repo, "new.go", "package new\n", "2020-01-06T00:00:00Z")

	churns, err := LogWithOptions(context.Background(), LogOptions{
		RepoPath: repo,
		Decay:    true,
		Now:      time.Date(2020, 1, 11, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}

	byPath := make(map[string]FileChurn)
	for _, c := range churns {
		byPath[c.Path] = c
	}

	old := byPath["old.go"]
	if math.Abs(old.WeightedCommits-0.5) > 0.001 {
		t.Fatalf("old.go weighted commits = %f, want about 0.5", old.WeightedCommits)
	}
	newer := byPath["new.go"]
	wantNewer := math.Sqrt(0.5)
	if math.Abs(newer.WeightedCommits-wantNewer) > 0.001 {
		t.Fatalf("new.go weighted commits = %f, want about %f", newer.WeightedCommits, wantNewer)
	}
}

func TestLogWithOptions_CancelledContext(t *testing.T) {
	repo := initLogTestRepo(t)
	commitFile(t, repo, "a.go", "package a\n", "2020-01-01T00:00:00Z")

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before invocation — git should exit non-zero immediately

	_, err := LogWithOptions(ctx, LogOptions{RepoPath: repo})
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled in error chain, got: %v", err)
	}
}

func TestRepoRoot_CapturesStderr(t *testing.T) {
	// Point at a directory that is definitely not a git repo.
	dir := t.TempDir()
	_, err := RepoRoot(context.Background(), dir)
	if err == nil {
		t.Fatal("expected error from non-repo directory")
	}
	// stderr should make it into the error message.
	if msg := err.Error(); msg == "" || !containsAny(msg, "not a git repository", "fatal") {
		t.Fatalf("expected stderr-bearing error, got: %v", err)
	}
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if sub != "" && (len(s) >= len(sub)) {
			for i := 0; i+len(sub) <= len(s); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
		}
	}
	return false
}

func initLogTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "", "init", "-q", "-b", "main")
	runGit(t, dir, "", "config", "user.email", "test@example.com")
	runGit(t, dir, "", "config", "user.name", "test")
	runGit(t, dir, "", "config", "commit.gpgsign", "false")
	return dir
}

func commitFile(t *testing.T, repo, rel, body, date string) {
	t.Helper()
	full := filepath.Join(repo, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, date, "add", rel)
	runGit(t, repo, date, "commit", "-q", "-m", rel)
}

func runGit(t *testing.T, repo, date string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = repo
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=test",
		"GIT_COMMITTER_EMAIL=test@example.com",
	)
	if date != "" {
		cmd.Env = append(cmd.Env,
			"GIT_AUTHOR_DATE="+date,
			"GIT_COMMITTER_DATE="+date,
		)
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}
