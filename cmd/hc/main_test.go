package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// runRootCmd invokes the shipped CLI in-process, exercising the real command
// wiring from buildCommand.
func runRootCmd(t *testing.T, args ...string) error {
	t.Helper()
	return buildCommand().Run(context.Background(), args)
}

// initRepo creates a git repo with one committed file, dated well in the past
// so the file age floor never trips it.
func initRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com",
			"GIT_AUTHOR_DATE=2020-01-01T00:00:00Z", "GIT_COMMITTER_DATE=2020-01-01T00:00:00Z",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q", "-b", "main")
	run("config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-q", "-m", "init")
	// Symlink-resolved so the path matches what git rev-parse reports (macOS
	// TempDir lives under /var -> /private/var); app.Analyze rejects a target
	// that resolves outside the repo root otherwise.
	root, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// captureStdout runs fn with os.Stdout redirected to a pipe and returns what
// it wrote. runAnalyze writes to os.Stdout directly, so this is the seam.
func captureStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	done := make(chan string)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()
	runErr := fn()
	_ = w.Close()
	return <-done, runErr
}

// TestAnalyze_CouplingDefault pins the opt-out contract: JSON output carries
// the coupling section unless --no-coupling is given, and table/csv never
// error over it (they simply don't compute pairs).
func TestAnalyze_CouplingDefault(t *testing.T) {
	repo := initRepo(t)

	tests := []struct {
		name         string
		args         []string
		wantSection  bool
		wantOptsFlag bool
	}{
		{"json default", []string{"hc", "--json"}, true, true},
		{"json no-coupling", []string{"hc", "--json", "--no-coupling"}, false, false},
		{"output json default", []string{"hc", "-o", "json"}, true, true},
		{"analyze subcommand json default", []string{"hc", "analyze", "--json"}, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := captureStdout(t, func() error {
				return runRootCmd(t, append(tt.args, repo)...)
			})
			if err != nil {
				t.Fatalf("run: %v", err)
			}
			var env map[string]json.RawMessage
			if err := json.Unmarshal([]byte(out), &env); err != nil {
				t.Fatalf("parse envelope: %v\n%s", err, out)
			}
			if _, ok := env["coupling"]; ok != tt.wantSection {
				t.Errorf("coupling section present=%v, want %v", ok, tt.wantSection)
			}
			var opts struct {
				Coupling bool `json:"coupling"`
			}
			if err := json.Unmarshal(env["options"], &opts); err != nil {
				t.Fatalf("parse options: %v", err)
			}
			if opts.Coupling != tt.wantOptsFlag {
				t.Errorf("options.coupling=%v, want %v", opts.Coupling, tt.wantOptsFlag)
			}
		})
	}

	// Non-JSON formats: --no-coupling is accepted as a harmless no-op, and the
	// default never errors the way --coupling used to.
	for _, args := range [][]string{
		{"hc"},
		{"hc", "--no-coupling"},
		{"hc", "-o", "csv"},
		{"hc", "-o", "csv", "--no-coupling"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			_, err := captureStdout(t, func() error {
				return runRootCmd(t, append(args, repo)...)
			})
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestAnalyze_CouplingFlagRemoved(t *testing.T) {
	err := runRootCmd(t, "hc", "--coupling", "--json", t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "flag provided but not defined") {
		t.Errorf("--coupling should be an unknown flag now, got %v", err)
	}
}

func TestAnalyze_JSONOutputConflict(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		wantConflict bool
	}{
		{"json alone", []string{"hc", "--json"}, false},
		{"output csv alone", []string{"hc", "-o", "csv"}, false},
		{"json with output json (idempotent)", []string{"hc", "--json", "-o", "json"}, false},
		{"json with output csv", []string{"hc", "--json", "-o", "csv"}, true},
		{"json with output table", []string{"hc", "--json", "-o", "table"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runRootCmd(t, append(tt.args, t.TempDir())...)

			isConflict := err != nil && strings.Contains(err.Error(), "--json conflicts with --output")
			if tt.wantConflict && !isConflict {
				t.Errorf("want conflict error, got %v", err)
			}
			if !tt.wantConflict && isConflict {
				t.Errorf("did not expect conflict error, got %v", err)
			}
		})
	}
}
