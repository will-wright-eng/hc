package git

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/will-wright-eng/hc/internal/ignore"
)

// FileChurn represents git history analysis for a single file.
type FileChurn struct {
	Path            string
	Commits         int
	WeightedCommits float64
	Authors         int
	// FirstSeen is the earliest commit date touching this path within the
	// scanned window. Bounded by --since when set; zero when the file has
	// no commits in the window.
	FirstSeen time.Time
}

// commitInfo holds the date and files for a single commit.
type commitInfo struct {
	Date  time.Time
	Files []string
}

// LogOptions controls git history extraction.
type LogOptions struct {
	// RepoPath is the root of the git repository.
	RepoPath string
	// Since is an optional time window (e.g. "6 months") passed to git --since.
	Since string
	// Ignore filters resolved paths from the final churn result.
	Ignore *ignore.Matcher
	// Decay enables recency weighting in churn computation.
	Decay bool
	// Now is the reference time for decay weighting. Zero means time.Now().
	Now time.Time
}

// Log runs git log and returns per-file churn data.
// repoPath is the root of the git repository.
// since is an optional time window (e.g. "6 months") passed to --since.
func Log(ctx context.Context, repoPath string, since string, ig *ignore.Matcher, decay bool) ([]FileChurn, error) {
	return LogWithOptions(ctx, LogOptions{
		RepoPath: repoPath,
		Since:    since,
		Ignore:   ig,
		Decay:    decay,
	})
}

// LogWithOptions runs git log and returns per-file churn data. ctx cancels the
// underlying git invocations.
func LogWithOptions(ctx context.Context, opts LogOptions) ([]FileChurn, error) {
	commitFiles, err := gitLogFiles(ctx, opts.RepoPath, opts.Since)
	if err != nil {
		return nil, err
	}

	authorMap, err := gitLogAuthors(ctx, opts.RepoPath, opts.Since)
	if err != nil {
		return nil, err
	}

	renames, err := DetectRenames(ctx, opts.RepoPath, opts.Since)
	if err != nil {
		return nil, fmt.Errorf("detecting renames: %w", err)
	}

	type stats struct {
		commits         int
		weightedCommits float64
		authors         map[string]struct{}
		firstSeen       time.Time
	}

	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	var halfLifeDays float64
	if opts.Decay {
		halfLifeDays = defaultHalfLifeDays(commitFiles, now)
	}

	// Build raw churn map (no ignore filtering yet — need resolved paths first).
	raw := make(map[string]*stats)
	for _, ci := range commitFiles {
		w := DecayWeight(ci.Date, now, halfLifeDays)
		for _, f := range ci.Files {
			if f == "" {
				continue
			}
			s, ok := raw[f]
			if !ok {
				s = &stats{authors: make(map[string]struct{})}
				raw[f] = s
			}
			s.commits++
			s.weightedCommits += w
			if s.firstSeen.IsZero() || ci.Date.Before(s.firstSeen) {
				s.firstSeen = ci.Date
			}
		}
	}

	// Rewrite churn map keys through rename resolution, merging stats.
	m := make(map[string]*stats)
	for path, s := range raw {
		resolved := renames.Resolve(path)
		if existing, ok := m[resolved]; ok {
			existing.commits += s.commits
			existing.weightedCommits += s.weightedCommits
			if existing.firstSeen.IsZero() || (!s.firstSeen.IsZero() && s.firstSeen.Before(existing.firstSeen)) {
				existing.firstSeen = s.firstSeen
			}
		} else {
			m[resolved] = s
		}
	}

	// Rewrite author map keys through rename resolution, then merge into stats.
	for path, authors := range authorMap {
		resolved := renames.Resolve(path)
		s, ok := m[resolved]
		if !ok {
			continue
		}
		for _, a := range authors {
			s.authors[a] = struct{}{}
		}
	}

	// Build result, applying ignore filter on resolved paths.
	result := make([]FileChurn, 0, len(m))
	for path, s := range m {
		if opts.Ignore.Match(path) {
			continue
		}
		result = append(result, FileChurn{
			Path:            path,
			Commits:         s.commits,
			WeightedCommits: s.weightedCommits,
			Authors:         len(s.authors),
			FirstSeen:       s.firstSeen,
		})
	}
	return result, nil
}

// gitLogFiles returns commit info (date + files) for each commit.
func gitLogFiles(ctx context.Context, repoPath string, since string) ([]commitInfo, error) {
	args := []string{"log", "--format=format:__DATE__%cI", "--name-only"}
	if since != "" {
		args = append(args, "--since="+since)
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = repoPath
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, gitError("git log --name-only", err, &stderr)
	}

	var commits []commitInfo
	var current commitInfo
	hasDate := false
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			if len(current.Files) > 0 {
				commits = append(commits, current)
				current = commitInfo{}
				hasDate = false
			}
			continue
		}
		if strings.HasPrefix(line, "__DATE__") {
			if len(current.Files) > 0 {
				commits = append(commits, current)
				current = commitInfo{}
			}
			dateStr := line[len("__DATE__"):]
			t, err := time.Parse(time.RFC3339, dateStr)
			if err == nil {
				current.Date = t
			}
			hasDate = true
			continue
		}
		if hasDate {
			current.Files = append(current.Files, line)
		}
	}
	if len(current.Files) > 0 {
		commits = append(commits, current)
	}
	return commits, scanner.Err()
}

// gitLogAuthors returns a map of file path -> list of author names.
func gitLogAuthors(ctx context.Context, repoPath string, since string) (map[string][]string, error) {
	args := []string{"log", "--format=format:__AUTHOR__%aN", "--name-only"}
	if since != "" {
		args = append(args, "--since="+since)
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = repoPath
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, gitError("git log authors", err, &stderr)
	}

	result := make(map[string][]string)
	var currentAuthor string
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "__AUTHOR__") {
			currentAuthor = line[len("__AUTHOR__"):]
			continue
		}
		if currentAuthor != "" {
			result[line] = append(result[line], currentAuthor)
		}
	}

	// Deduplicate authors per file.
	for path, authors := range result {
		seen := make(map[string]struct{})
		deduped := authors[:0]
		for _, a := range authors {
			if _, ok := seen[a]; !ok {
				seen[a] = struct{}{}
				deduped = append(deduped, a)
			}
		}
		result[path] = deduped
	}

	return result, scanner.Err()
}

// CountAuthors is a helper for testing — counts unique authors from a shortlog.
func CountAuthors(ctx context.Context, repoPath string, path string) (int, error) {
	args := []string{"shortlog", "-sn", "--", path}
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = repoPath
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return 0, gitError("git shortlog", err, &stderr)
	}
	count := 0
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			count++
		}
	}
	return count, scanner.Err()
}

// gitError wraps a git invocation failure with stderr context when available.
// Context-cancellation errors are returned as-is so callers can match them
// with errors.Is(err, context.Canceled).
func gitError(op string, err error, stderr *bytes.Buffer) error {
	msg := strings.TrimSpace(stderr.String())
	if msg == "" {
		return fmt.Errorf("%s: %w", op, err)
	}
	return fmt.Errorf("%s: %w: %s", op, err, msg)
}
