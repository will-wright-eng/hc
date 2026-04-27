package prompt

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// summaryOpts controls what the repo summary includes.
type summaryOpts struct {
	MaxFiles int
}

// writeSummary writes a compact repo summary to w as a fenced text block.
func writeSummary(root string, w io.Writer, opts summaryOpts) error {
	maxFiles := opts.MaxFiles
	if maxFiles <= 0 {
		maxFiles = 200
	}

	var (
		dirCounts  = make(map[string]int) // relative dir → file count
		extCounts  = make(map[string]int) // extension → file count
		totalFiles int
		files      []fileEntry
	)

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}

		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)

		// Skip .git directory.
		if d.IsDir() && (d.Name() == ".git") {
			return filepath.SkipDir
		}

		if d.IsDir() {
			return nil
		}

		totalFiles++
		if totalFiles > maxFiles {
			return nil // keep walking for counts but don't collect entries
		}

		// Directory counts (top 2 levels).
		dir := filepath.ToSlash(filepath.Dir(rel))
		parts := strings.Split(dir, "/")
		if len(parts) > 2 {
			dir = strings.Join(parts[:2], "/")
		}
		dirCounts[dir]++

		// Extension histogram.
		ext := filepath.Ext(d.Name())
		if ext == "" {
			ext = "(no ext)"
		}
		extCounts[ext]++

		// Collect file info for largest-files list.
		info, err := d.Info()
		if err != nil {
			return nil
		}
		files = append(files, fileEntry{path: rel, size: info.Size()})

		return nil
	})
	if err != nil {
		return err
	}

	if _, err := fmt.Fprintln(w, "```text"); err != nil {
		return err
	}

	// 1. Top-level tree (directories with file counts).
	if err := writeTree(w, dirCounts); err != nil {
		return err
	}

	// 2. Extension histogram (top 20).
	if err := writeExtensions(w, extCounts); err != nil {
		return err
	}

	// 3. Largest files (top 15).
	if err := writeLargestFiles(w, files); err != nil {
		return err
	}

	// 4. Notable root files.
	if err := writeNotableFiles(w, root); err != nil {
		return err
	}

	_, err = fmt.Fprintln(w, "```")
	return err
}

type fileEntry struct {
	path string
	size int64
}

// writeTree writes directory names with file counts, sorted by count descending.
func writeTree(w io.Writer, dirCounts map[string]int) error {
	type dirCount struct {
		dir   string
		count int
	}

	sorted := make([]dirCount, 0, len(dirCounts))
	for d, c := range dirCounts {
		sorted = append(sorted, dirCount{d, c})
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].count > sorted[j].count
	})

	if _, err := fmt.Fprintln(w, "Directory structure (file counts):"); err != nil {
		return err
	}
	for _, dc := range sorted {
		if _, err := fmt.Fprintf(w, "  %-40s %d files\n", dc.dir, dc.count); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintln(w)
	return err
}

// writeExtensions writes the top N extensions by count.
func writeExtensions(w io.Writer, extCounts map[string]int) error {
	type extCount struct {
		ext   string
		count int
	}

	sorted := make([]extCount, 0, len(extCounts))
	for e, c := range extCounts {
		sorted = append(sorted, extCount{e, c})
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].count > sorted[j].count
	})

	limit := min(20, len(sorted))

	if _, err := fmt.Fprintln(w, "Extensions by file count:"); err != nil {
		return err
	}
	for _, ec := range sorted[:limit] {
		if _, err := fmt.Fprintf(w, "  %-20s %d\n", ec.ext, ec.count); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintln(w)
	return err
}

// writeLargestFiles writes the top N files by byte size.
func writeLargestFiles(w io.Writer, files []fileEntry) error {
	sort.Slice(files, func(i, j int) bool {
		return files[i].size > files[j].size
	})

	limit := min(15, len(files))

	if _, err := fmt.Fprintln(w, "Largest files:"); err != nil {
		return err
	}
	for _, f := range files[:limit] {
		if _, err := fmt.Fprintf(w, "  %-50s %s\n", f.path, formatSize(f.size)); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintln(w)
	return err
}

// notableRootFiles are filenames commonly associated with generated or vendored content.
var notableRootFiles = []string{
	"go.sum",
	"go.mod",
	"package.json",
	"package-lock.json",
	"yarn.lock",
	"pnpm-lock.yaml",
	"Cargo.lock",
	"Gemfile.lock",
	"composer.lock",
	"Pipfile.lock",
	"poetry.lock",
	"requirements.txt",
	".hcignore",
}

// writeNotableFiles lists which notable files exist at the repo root.
func writeNotableFiles(w io.Writer, root string) error {
	var found []string
	for _, name := range notableRootFiles {
		if _, err := os.Stat(filepath.Join(root, name)); err == nil {
			found = append(found, name)
		}
	}
	if len(found) == 0 {
		return nil
	}
	if _, err := fmt.Fprintln(w, "Notable root files:"); err != nil {
		return err
	}
	for _, f := range found {
		if _, err := fmt.Fprintf(w, "  %s\n", f); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintln(w)
	return err
}

// formatSize formats byte count as a human-readable string.
func formatSize(b int64) string {
	switch {
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(b)/float64(1<<10))
	default:
		return fmt.Sprintf("%d B", b)
	}
}
