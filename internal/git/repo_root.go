package git

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// RepoRoot returns the absolute path to the top-level directory of the git
// repository containing path. Returns an error if path is not inside a git
// repository or git fails. The returned path is the canonical root that
// matches the prefix of paths emitted by `git log --name-only`.
func RepoRoot(ctx context.Context, path string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel")
	cmd.Dir = path
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			return "", fmt.Errorf("resolving git repo root for %s: %w", path, err)
		}
		return "", fmt.Errorf("resolving git repo root for %s: %s", path, msg)
	}
	return strings.TrimSpace(string(out)), nil
}
