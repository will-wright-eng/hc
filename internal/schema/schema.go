// Package schema defines the on-the-wire DTOs for `hc analyze --json`.
// Producers (internal/output) and consumers (internal/md) both import these
// types so the JSON contract cannot drift silently.
package schema

import "time"

// SchemaVersion is the current envelope version. Bump on non-additive changes
// (renamed fields, type changes, removed fields). Additive changes — new
// optional fields — keep the same version.
const SchemaVersion = "1"

// Envelope is the top-level shape of `hc analyze --json`. The bare-array form
// shipped before this envelope was introduced is no longer produced or
// accepted.
type Envelope struct {
	SchemaVersion string     `json:"schema_version"`
	GeneratedAt   time.Time  `json:"generated_at"`
	RepoRoot      string     `json:"repo_root"`
	AnalyzedPath  string     `json:"analyzed_path,omitempty"`
	Options       Options    `json:"options"`
	Thresholds    Thresholds `json:"thresholds"`
	Files         []File     `json:"files"`
}

// Options snapshots the analyze inputs that affected the result. Empty
// strings/slices are omitted so the envelope stays readable for default runs.
type Options struct {
	Since    string   `json:"since,omitempty"`
	Decay    bool     `json:"decay"`
	MinAge   string   `json:"min_age,omitempty"`
	Excludes []string `json:"excludes,omitempty"`
}

// Thresholds are the median-split values used to classify files into quadrants.
type Thresholds struct {
	Churn      float64 `json:"churn"`
	Complexity int     `json:"complexity"`
}

// File is the per-file analysis row. The kebab-case Quadrant matches the JSON
// form produced by analysis.Quadrant.JSONString.
type File struct {
	Path            string  `json:"path"`
	Commits         int     `json:"commits"`
	WeightedCommits float64 `json:"weighted_commits,omitempty"`
	Lines           int     `json:"lines"`
	Complexity      int     `json:"complexity"`
	Authors         int     `json:"authors"`
	Quadrant        string  `json:"quadrant"`
}
