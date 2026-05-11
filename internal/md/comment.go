package md

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

//go:embed templates/comment/*.md
var commentTemplates embed.FS

const commentTagFormat = "<!-- hc-pr-comment:%s -->"

// Canonical comment-emission order. Matches the quadrant priority used
// elsewhere in hc (hot-critical first).
var quadrantRank = map[string]int{
	"hot-critical": 0,
	"hot-simple":   1,
	"cold-complex": 2,
	"cold-simple":  3,
}

// Quadrants without a template are silently dropped (today only hot-critical
// and cold-complex have one).
var quadrantTemplate = map[string]string{
	"hot-critical": "hotcritical.md",
	"cold-complex": "coldcomplex.md",
}

type CommentOpts struct {
	// Quadrants restricts output to the listed quadrant keys. Empty means
	// the default set (hot-critical, cold-complex).
	Quadrants []string
}

// CommentEntry is one rendered PR comment, ready for the shell loop to post.
type CommentEntry struct {
	Path     string `json:"path"`
	Quadrant string `json:"quadrant"`
	Tag      string `json:"tag"`
	Body     string `json:"body"`
}

// RenderComments reads analyze JSON from r, filters and sorts entries, renders
// each one through its quadrant template, and writes one NDJSON object per
// comment to w.
func RenderComments(r io.Reader, w io.Writer, opts CommentOpts) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("reading input: %w", err)
	}

	var raws []json.RawMessage
	if err := json.Unmarshal(data, &raws); err != nil {
		return fmt.Errorf("parsing JSON: %w", err)
	}

	quadrantSet := buildQuadrantSet(opts.Quadrants)

	type kept struct {
		head fileEntry
		raw  json.RawMessage
	}
	var keep []kept
	for _, raw := range raws {
		var head fileEntry
		if err := json.Unmarshal(raw, &head); err != nil {
			return fmt.Errorf("parsing entry: %w", err)
		}
		if _, ok := quadrantTemplate[head.Quadrant]; !ok {
			continue
		}
		if _, ok := quadrantSet[head.Quadrant]; !ok {
			continue
		}
		keep = append(keep, kept{head: head, raw: raw})
	}

	sort.SliceStable(keep, func(i, j int) bool {
		if quadrantRank[keep[i].head.Quadrant] != quadrantRank[keep[j].head.Quadrant] {
			return quadrantRank[keep[i].head.Quadrant] < quadrantRank[keep[j].head.Quadrant]
		}
		return keep[i].head.WeightedCommits > keep[j].head.WeightedCommits
	})

	enc := json.NewEncoder(w)
	for _, k := range keep {
		body, err := renderCommentBody(k.head.Quadrant, k.raw)
		if err != nil {
			return fmt.Errorf("rendering %s: %w", k.head.Path, err)
		}
		tag := fmt.Sprintf(commentTagFormat, k.head.Path)
		entry := CommentEntry{
			Path:     k.head.Path,
			Quadrant: k.head.Quadrant,
			Tag:      tag,
			Body:     body + "\n" + tag + "\n",
		}
		if err := enc.Encode(entry); err != nil {
			return fmt.Errorf("encoding entry: %w", err)
		}
	}
	return nil
}

func buildQuadrantSet(in []string) map[string]struct{} {
	set := make(map[string]struct{})
	if len(in) == 0 {
		set["hot-critical"] = struct{}{}
		set["cold-complex"] = struct{}{}
		return set
	}
	for _, q := range in {
		set[q] = struct{}{}
	}
	return set
}

func renderCommentBody(quadrant string, raw json.RawMessage) (string, error) {
	tmplName, ok := quadrantTemplate[quadrant]
	if !ok {
		return "", fmt.Errorf("no template for quadrant %q", quadrant)
	}
	tmplBytes, err := commentTemplates.ReadFile("templates/comment/" + tmplName)
	if err != nil {
		return "", fmt.Errorf("reading template: %w", err)
	}
	table, err := renderStatsTable(raw)
	if err != nil {
		return "", err
	}
	return strings.Replace(string(tmplBytes), "<!-- hc-stats -->", table, 1), nil
}

// renderStatsTable produces a markdown table from the JSON entry, in JSON-field
// order. Keys are humanized (snake_case → Title Case). Null values are skipped;
// nested objects/arrays are emitted as their JSON form.
func renderStatsTable(raw json.RawMessage) (string, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()

	tok, err := dec.Token()
	if err != nil {
		return "", err
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return "", fmt.Errorf("expected JSON object, got %v", tok)
	}

	var sb strings.Builder
	sb.WriteString("| Field | Value |\n")
	sb.WriteString("| --- | --- |\n")

	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return "", err
		}
		key, ok := keyTok.(string)
		if !ok {
			return "", fmt.Errorf("expected string key, got %v", keyTok)
		}

		var valRaw json.RawMessage
		if err := dec.Decode(&valRaw); err != nil {
			return "", err
		}

		val, skip := formatRawValue(valRaw)
		if skip {
			continue
		}
		fmt.Fprintf(&sb, "| %s | %s |\n", humanize(key), val)
	}
	return sb.String(), nil
}

func formatRawValue(raw json.RawMessage) (string, bool) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return string(raw), false
	}
	switch t := v.(type) {
	case nil:
		return "", true
	case bool:
		return strconv.FormatBool(t), false
	case string:
		return t, false
	case json.Number:
		if i, err := t.Int64(); err == nil {
			return strconv.FormatInt(i, 10), false
		}
		f, _ := t.Float64()
		return strconv.FormatFloat(f, 'f', 2, 64), false
	}
	return string(raw), false
}

func humanize(key string) string {
	parts := strings.Split(key, "_")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p == "" {
			continue
		}
		out = append(out, strings.ToUpper(p[:1])+p[1:])
	}
	return strings.Join(out, " ")
}
