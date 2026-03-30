package complexity

import (
	"bufio"
	"os"
	"strings"
)

// IndentSum computes the sum of indent levels across all non-blank, non-comment
// lines in a file. The indent unit is detected per-file by scanning leading
// whitespace deltas. This serves as a language-agnostic proxy for nesting complexity.
func IndentSum(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}

	useTabs := detectIndentChar(lines)
	indentUnit := 1
	if !useTabs {
		indentUnit = detectIndentUnit(lines)
	}

	sum := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || isCommentLine(trimmed) {
			continue
		}
		level := indentLevel(line, useTabs, indentUnit)
		sum += level
	}
	return sum, nil
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
