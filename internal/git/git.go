package git

import (
	"bufio"
	"bytes"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/will/hc/internal/ignore"
)

// FileChurn represents git history analysis for a single file.
type FileChurn struct {
	Path    string
	Commits int
	Authors int
}

// Log runs git log and returns per-file churn data.
// repoPath is the root of the git repository.
// since is an optional time window (e.g. "6 months") passed to --since.
func Log(repoPath string, since string, ig *ignore.Matcher) ([]FileChurn, error) {
	commitFiles, err := gitLogFiles(repoPath, since)
	if err != nil {
		return nil, err
	}

	authorMap, err := gitLogAuthors(repoPath, since)
	if err != nil {
		return nil, err
	}

	renames, err := DetectRenames(repoPath, since)
	if err != nil {
		return nil, fmt.Errorf("detecting renames: %w", err)
	}

	type stats struct {
		commits int
		authors map[string]struct{}
	}

	// Build raw churn map (no ignore filtering yet — need resolved paths first).
	raw := make(map[string]*stats)
	for _, files := range commitFiles {
		for _, f := range files {
			if f == "" {
				continue
			}
			s, ok := raw[f]
			if !ok {
				s = &stats{authors: make(map[string]struct{})}
				raw[f] = s
			}
			s.commits++
		}
	}

	// Rewrite churn map keys through rename resolution, merging stats.
	m := make(map[string]*stats)
	for path, s := range raw {
		resolved := renames.Resolve(path)
		if existing, ok := m[resolved]; ok {
			existing.commits += s.commits
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
		if ig.Match(path) {
			continue
		}
		result = append(result, FileChurn{
			Path:    path,
			Commits: s.commits,
			Authors: len(s.authors),
		})
	}
	return result, nil
}

// gitLogFiles returns a slice of file lists, one per commit.
func gitLogFiles(repoPath string, since string) ([][]string, error) {
	args := []string{"log", "--format=format:", "--name-only"}
	if since != "" {
		args = append(args, "--since="+since)
	}

	cmd := exec.Command("git", args...)
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var commits [][]string
	var current []string
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			if len(current) > 0 {
				commits = append(commits, current)
				current = nil
			}
			continue
		}
		current = append(current, line)
	}
	if len(current) > 0 {
		commits = append(commits, current)
	}
	return commits, scanner.Err()
}

// gitLogAuthors returns a map of file path -> list of author names.
func gitLogAuthors(repoPath string, since string) (map[string][]string, error) {
	args := []string{"log", "--format=format:__AUTHOR__%aN", "--name-only"}
	if since != "" {
		args = append(args, "--since="+since)
	}

	cmd := exec.Command("git", args...)
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return nil, err
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
func CountAuthors(repoPath string, path string) (int, error) {
	args := []string{"shortlog", "-sn", "--", path}
	cmd := exec.Command("git", args...)
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return 0, err
	}
	count := 0
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			count++
		}
	}
	_ = strconv.Itoa(count) // just to keep strconv imported if needed
	return count, scanner.Err()
}
