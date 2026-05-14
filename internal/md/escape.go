package md

import (
	"strings"
	"unicode"
)

// looksLikeBareArray reports whether the first non-whitespace byte of data is
// '['. Used to detect legacy `hc analyze --json` output (a bare file array)
// so we can return a friendlier error than a deep unmarshal failure.
func looksLikeBareArray(data []byte) bool {
	for _, b := range data {
		if unicode.IsSpace(rune(b)) {
			continue
		}
		return b == '['
	}
	return false
}

// escapeTableCell escapes characters that would break a GitHub-flavored
// markdown table cell: backslashes, pipes, backticks, and embedded newlines.
// Backslashes are escaped first so subsequent escapes do not get themselves
// re-escaped.
func escapeTableCell(s string) string {
	if s == "" {
		return s
	}
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "|", `\|`)
	s = strings.ReplaceAll(s, "`", "\\`")
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	return s
}
