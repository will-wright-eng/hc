package git

import (
	"bufio"
	"bytes"
	"os/exec"
	"strings"
)

// RenameMap maps old file paths to their current (on-disk) equivalents.
type RenameMap map[string]string

// DetectRenames runs git log to find all rename events in the given time
// window and returns a resolved mapping of old path -> current path.
// Chains are collapsed: if a -> b -> c, the map contains a -> c and b -> c.
func DetectRenames(repoPath string, since string) (RenameMap, error) {
	args := []string{"log", "--diff-filter=R", "--name-status", "--format=format:"}
	if since != "" {
		args = append(args, "--since="+since)
	}

	cmd := exec.Command("git", args...)
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	raw := make(map[string]string)
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "R") {
			continue
		}
		// Format: R###\told_path\tnew_path
		fields := strings.Split(line, "\t")
		if len(fields) != 3 {
			continue
		}
		oldPath := fields[1]
		newPath := fields[2]
		raw[oldPath] = newPath
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return resolveChains(raw), nil
}

// resolveChains collapses rename chains so that every key maps directly
// to its final (current) path. For example, if raw contains a->b and b->c,
// the result contains a->c and b->c.
func resolveChains(raw map[string]string) RenameMap {
	resolved := make(RenameMap, len(raw))
	for old := range raw {
		final := old
		seen := make(map[string]bool)
		for {
			next, ok := raw[final]
			if !ok {
				break
			}
			if seen[final] {
				break // cycle detected
			}
			seen[final] = true
			final = next
		}
		resolved[old] = final
	}
	return resolved
}

// Resolve returns the current path for the given path. If the path has not
// been renamed, it is returned unchanged.
func (rm RenameMap) Resolve(path string) string {
	if rm == nil {
		return path
	}
	if resolved, ok := rm[path]; ok {
		return resolved
	}
	return path
}
