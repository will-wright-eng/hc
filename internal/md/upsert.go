package md

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
)

const (
	MarkerStart = "<!-- hc:report:start -->"
	MarkerEnd   = "<!-- hc:report:end -->"
)

// HasSection returns true if data contains the start marker.
func HasSection(data []byte) bool {
	return bytes.Contains(data, []byte(MarkerStart))
}

// ReplaceSection replaces everything from the start marker through the end marker
// (inclusive) with the new content. Returns the original data unchanged if markers
// are not found.
func ReplaceSection(data []byte, content string) []byte {
	startMarker := []byte(MarkerStart)
	endMarker := []byte(MarkerEnd)

	startIdx := bytes.Index(data, startMarker)
	if startIdx < 0 {
		return data
	}

	endIdx := bytes.Index(data[startIdx:], endMarker)
	if endIdx < 0 {
		return data
	}
	endIdx = startIdx + endIdx + len(endMarker)

	// Consume trailing newline after end marker if present.
	if endIdx < len(data) && data[endIdx] == '\n' {
		endIdx++
	}

	var result []byte
	result = append(result, data[:startIdx]...)
	result = append(result, []byte(content)...)
	result = append(result, data[endIdx:]...)
	return result
}

// UpsertFile reads the file at path, replaces the report section if present
// (or appends if not), and writes the file back atomically.
func UpsertFile(path string, content string) error {
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading %s: %w", path, err)
	}

	var output []byte
	if HasSection(existing) {
		output = ReplaceSection(existing, content)
	} else if len(existing) > 0 {
		// Append with a blank line separator.
		sep := "\n"
		if len(existing) > 0 && existing[len(existing)-1] != '\n' {
			sep = "\n\n"
		}
		output = append(existing, []byte(sep+content)...)
	} else {
		output = []byte(content)
	}

	// Atomic write: write to temp file in same directory, then rename.
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".hc-report-*.tmp")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(output); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("writing temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("closing temp file: %w", err)
	}

	// Preserve original file permissions if it exists.
	if info, err := os.Stat(path); err == nil {
		_ = os.Chmod(tmpPath, info.Mode())
	}

	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("replacing file: %w", err)
	}

	return nil
}
