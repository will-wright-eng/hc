// Package ignore matches relative file paths against gitignore-style patterns.
// Supports the gitignore feature set documented at https://git-scm.com/docs/gitignore:
// negation (!), leading-slash anchoring, comment escapes (\#), ** globbing,
// and last-match-wins ordering across the combined .hcignore + --exclude set.
//
// The pattern compilation in compilePattern is a port of
// github.com/sabhiram/go-gitignore (MIT, Copyright (c) 2015 Shaba Abhiram),
// trimmed to the surface this project actually uses.
package ignore

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Matcher tests relative paths against a compiled set of ignore patterns.
type Matcher struct {
	patterns []compiledPattern
}

type compiledPattern struct {
	re     *regexp.Regexp
	negate bool
}

// New compiles patterns into a Matcher. Returns nil for an empty pattern set
// so nil-check callers skip filtering. Patterns are evaluated in order, and
// later patterns can override earlier ones via gitignore's last-match-wins
// rule (e.g. `*.go` followed by `!keep.go`).
func New(patterns []string) *Matcher {
	if len(patterns) == 0 {
		return nil
	}
	m := &Matcher{}
	for _, line := range patterns {
		if cp, ok := compilePattern(line); ok {
			m.patterns = append(m.patterns, cp)
		}
	}
	if len(m.patterns) == 0 {
		return nil
	}
	return m
}

// Match returns true if the given relative path should be ignored.
// A nil Matcher never matches.
func (m *Matcher) Match(relPath string) bool {
	if m == nil {
		return false
	}
	relPath = filepath.ToSlash(relPath)
	matched := false
	for _, p := range m.patterns {
		if !p.re.MatchString(relPath) {
			continue
		}
		if !p.negate {
			matched = true
		} else if matched {
			matched = false
		}
	}
	return matched
}

// LoadFile reads patterns from a .hcignore file. Returns nil (no patterns)
// if the file does not exist. Returns an error only for read failures.
//
// Blank lines and unescaped comment lines are stripped here so the returned
// slice is clean for combination with --exclude flags; escapes like \# and \!
// are preserved and handled by the matcher.
func LoadFile(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var patterns []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, line)
	}
	return patterns, scanner.Err()
}

// magicStar is a placeholder used during regex assembly to protect literal
// `*` and `**` segments from later substitutions. Any sequence that does not
// appear in real paths works; "#$~" is the original choice from sabhiram.
const magicStar = "#$~"

var (
	reEscapedHashOrBang  = regexp.MustCompile(`^(\#|\!)`)
	reFolderGlob         = regexp.MustCompile(`([^/+])/.*\*\.`)
	reDot                = regexp.MustCompile(`\.`)
	reMidDoubleStar      = regexp.MustCompile(`/\*\*/`)
	reLeadingDoubleStar  = regexp.MustCompile(`\*\*/`)
	reTrailingDoubleStar = regexp.MustCompile(`/\*\*`)
	reEscapedStar        = regexp.MustCompile(`\\\*`)
	reStar               = regexp.MustCompile(`\*`)
)

// compilePattern translates a single gitignore line into a regex + negate flag.
// Returns ok=false for blank lines, unescaped comments, and unparseable input.
func compilePattern(line string) (compiledPattern, bool) {
	line = strings.TrimRight(line, "\r")

	// Rule 2: a line starting with # is a comment.
	if strings.HasPrefix(line, "#") {
		return compiledPattern{}, false
	}

	// Rule 3: trailing (and leading) spaces are dropped. Escape-with-backslash
	// is not implemented — same as upstream.
	line = strings.Trim(line, " ")
	if line == "" {
		return compiledPattern{}, false
	}

	// Rule 4: leading ! negates.
	negate := false
	if line[0] == '!' {
		negate = true
		line = line[1:]
	}

	// Rules 2 and 4: \# and \! are literal-character escapes; strip the
	// leading backslash now that we've recorded any negation.
	if reEscapedHashOrBang.MatchString(line) {
		line = line[1:]
	}

	// "foo/*.blah" anchors to the gitignore root: prepend "/" so the regex
	// builder treats it as anchored.
	if reFolderGlob.MatchString(line) && line[0] != '/' {
		line = "/" + line
	}

	// Escape regex metacharacter "." so it matches literally.
	line = reDot.ReplaceAllString(line, `\.`)

	// Rule 9: ** has special meaning in three positions. Order matters here
	// — handle "/**/" first, then "**/" prefix, then "/**" suffix.
	if strings.HasPrefix(line, "/**/") {
		line = line[1:]
	}
	line = reMidDoubleStar.ReplaceAllString(line, `(/|/.+/)`)
	line = reLeadingDoubleStar.ReplaceAllString(line, `(|.`+magicStar+`/)`)
	line = reTrailingDoubleStar.ReplaceAllString(line, `(|/.`+magicStar+`)`)

	// Stash escaped "*" so a later substitution doesn't double-handle it,
	// then turn unescaped "*" into a non-slash run.
	line = reEscapedStar.ReplaceAllString(line, `\`+magicStar)
	line = reStar.ReplaceAllString(line, `([^/]*)`)

	// "?" matches any single character — escape so the regex engine treats
	// it as a literal-then-restored star via the magic placeholder.
	line = strings.ReplaceAll(line, "?", `\?`)
	line = strings.ReplaceAll(line, magicStar, "*")

	// Anchor: trailing "/" matches a directory; otherwise allow a trailing
	// slash + remainder for descendants.
	var expr string
	if strings.HasSuffix(line, "/") {
		expr = line + "(|.*)$"
	} else {
		expr = line + "(|/.*)$"
	}
	// Leading "/" anchors to the repo root; otherwise the pattern matches at
	// any depth ("(|.*/)" prefix).
	if strings.HasPrefix(expr, "/") {
		expr = "^(|/)" + expr[1:]
	} else {
		expr = "^(|.*/)" + expr
	}

	re, err := regexp.Compile(expr)
	if err != nil {
		return compiledPattern{}, false
	}
	return compiledPattern{re: re, negate: negate}, true
}
