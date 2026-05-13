package md

import "testing"

func TestEscapeTableCell(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"plain path", "src/parser.go", "src/parser.go"},
		{"pipe", "a|b", `a\|b`},
		{"backtick", "a`b`c", "a\\`b\\`c"},
		{"backslash", `a\b`, `a\\b`},
		{"backslash then pipe", `a\|b`, `a\\\|b`},
		{"newline", "a\nb", "a b"},
		{"crlf", "a\r\nb", "a b"},
		{"cr alone", "a\rb", "a b"},
		{"mixed", "weird|path`with\\stuff", "weird\\|path\\`with\\\\stuff"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := escapeTableCell(tc.in)
			if got != tc.want {
				t.Errorf("escapeTableCell(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
