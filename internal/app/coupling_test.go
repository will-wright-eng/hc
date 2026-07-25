package app

import (
	"context"
	"testing"
	"time"
)

// initCouplingRepo builds a repo where a.go and b.go co-change 5 times
// (2020-01-01..05), c.go changes alone, and young.go co-changes with a.go 5
// times just days before "now" (2020-02-01) so the age floor drops it.
func initCouplingRepo(t *testing.T) string {
	t.Helper()
	dir := initGitRepo(t, "2020-01-01T00:00:00Z")

	for i := 1; i <= 5; i++ {
		date := time.Date(2020, 1, i, 0, 0, 0, 0, time.UTC).Format(time.RFC3339)
		writeTestFile(t, dir, "a.go", "package main\n// rev "+date+"\n")
		writeTestFile(t, dir, "b.go", "package main\n// rev "+date+"\n")
		mustRunGit(t, dir, date, "add", ".")
		mustRunGit(t, dir, date, "commit", "-q", "-m", "co-change")
	}
	writeTestFile(t, dir, "c.go", "package main\n")
	mustRunGit(t, dir, "2020-01-06T00:00:00Z", "add", ".")
	mustRunGit(t, dir, "2020-01-06T00:00:00Z", "commit", "-q", "-m", "solo")

	for i := 27; i <= 31; i++ {
		date := time.Date(2020, 1, i, 0, 0, 0, 0, time.UTC).Format(time.RFC3339)
		writeTestFile(t, dir, "a.go", "package main\n// young rev "+date+"\n")
		writeTestFile(t, dir, "young.go", "package main\n// rev "+date+"\n")
		mustRunGit(t, dir, date, "add", ".")
		mustRunGit(t, dir, date, "commit", "-q", "-m", "young co-change")
	}

	return dir
}

var couplingNow = time.Date(2020, 2, 1, 0, 0, 0, 0, time.UTC)

func TestAnalyze_Coupling_PairsAndAgeFloor(t *testing.T) {
	root := initCouplingRepo(t)

	result, err := Analyze(context.Background(), AnalyzeOptions{
		Path:     root,
		Coupling: true,
		Now:      couplingNow,
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Coupling) != 1 {
		t.Fatalf("expected 1 pair (young.go age-floored despite 5 co-changes), got %+v", result.Coupling)
	}
	p := result.Coupling[0]
	if p.A != "a.go" || p.B != "b.go" || p.Support != 5 {
		t.Errorf("pair = %+v, want (a.go, b.go) support 5", p)
	}
	// a.go has 10 commits (5 with b.go, 5 with young.go); b.go has 5.
	if p.ConfidenceAB != 0.5 || p.ConfidenceBA != 1.0 {
		t.Errorf("confidences = (%f, %f), want (0.5, 1.0)", p.ConfidenceAB, p.ConfidenceBA)
	}
}

func TestAnalyze_Coupling_OffMeansNil(t *testing.T) {
	root := initCouplingRepo(t)

	result, err := Analyze(context.Background(), AnalyzeOptions{Path: root, Now: couplingNow})
	if err != nil {
		t.Fatal(err)
	}
	if result.Coupling != nil {
		t.Errorf("coupling should be nil when not requested, got %+v", result.Coupling)
	}
}

func TestAnalyze_Coupling_FilesFromKeepsOneSidePairs(t *testing.T) {
	root := initCouplingRepo(t)

	// a.go is in the selection: the (a.go, b.go) pair survives even though
	// b.go is outside it — that is the point of the partner signal.
	result, err := Analyze(context.Background(), AnalyzeOptions{
		Path:      root,
		Coupling:  true,
		FilesFrom: []string{"a.go"},
		Now:       couplingNow,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Coupling) != 1 {
		t.Fatalf("one-side pair should be kept under --files-from, got %+v", result.Coupling)
	}

	// c.go never co-changes: every pair has zero sides selected and drops.
	result, err = Analyze(context.Background(), AnalyzeOptions{
		Path:      root,
		Coupling:  true,
		FilesFrom: []string{"c.go"},
		Now:       couplingNow,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Coupling) != 0 {
		t.Errorf("zero-side pairs should drop under --files-from, got %+v", result.Coupling)
	}
	if result.Coupling == nil {
		t.Error("coupling should be non-nil (empty) when requested but filtered")
	}
}
