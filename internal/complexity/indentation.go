package complexity

import (
	"bufio"
	"os"
	"strings"
)

// scanFile reads a file once and returns the count of non-blank, non-comment
// lines along with the sum of their indent levels. Indent unit is detected
// per-file by scanning leading-whitespace deltas.
func scanFile(path string) (lines, indentSum int, err error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()

	var raw []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		raw = append(raw, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return 0, 0, err
	}

	useTabs := detectIndentChar(raw)
	indentUnit := 1
	if !useTabs {
		indentUnit = detectIndentUnit(raw)
	}

	for _, line := range raw {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || isCommentLine(trimmed) {
			continue
		}
		lines++
		indentSum += indentLevel(line, useTabs, indentUnit)
	}
	return lines, indentSum, nil
}

// IndentSum returns the indent-sum complexity for a single file.
// Retained for direct tests and external callers.
func IndentSum(path string) (int, error) {
	_, sum, err := scanFile(path)
	return sum, err
}

// detectIndentChar returns true if the file predominantly uses tabs for indentation.
func detectIndentChar(lines []string) bool {
	tabs, spaces := 0, 0
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		if line[0] == '\t' {
			tabs++
		} else if line[0] == ' ' && strings.TrimSpace(line) != "" {
			spaces++
		}
	}
	return tabs > spaces
}

// detectIndentUnit finds the most common leading-space delta between adjacent
// non-blank lines in the first 100 such lines. Returns 4 as default.
func detectIndentUnit(lines []string) int {
	deltaCounts := make(map[int]int)
	prevIndent := 0
	seen := 0

	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		seen++
		if seen > 100 {
			break
		}

		cur := countLeadingSpaces(line)
		delta := cur - prevIndent
		if delta > 0 {
			deltaCounts[delta]++
		}
		prevIndent = cur
	}

	bestDelta, bestCount := 4, 0
	for d, c := range deltaCounts {
		if c > bestCount {
			bestDelta = d
			bestCount = c
		}
	}
	return bestDelta
}

// indentLevel returns the indent level of a line.
func indentLevel(line string, useTabs bool, indentUnit int) int {
	if useTabs {
		count := 0
		for _, ch := range line {
			if ch == '\t' {
				count++
			} else {
				break
			}
		}
		return count
	}
	spaces := countLeadingSpaces(line)
	if indentUnit == 0 {
		return 0
	}
	return spaces / indentUnit
}

func countLeadingSpaces(line string) int {
	count := 0
	for _, ch := range line {
		if ch == ' ' {
			count++
		} else {
			break
		}
	}
	return count
}
