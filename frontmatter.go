package main

import (
	"bufio"
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Meta is the front matter block at the top of an article or a tutorial.
type Meta struct {
	Title       string
	Date        time.Time
	Description string
	Tags        []string
	Cover       string // optional; falls back to the site-wide OG image
	Draft       bool

	// Tutorials only.
	Video    string // YouTube video id
	Duration string // human-readable running time, e.g. "4:12"
}

const fmDelim = "---"

// splitFrontMatter peels the leading "---" fenced block off a Markdown file.
// A file without front matter is an error: every article needs at least a
// title and a date, and silently publishing an untitled post is worse than
// failing the build.
func splitFrontMatter(src []byte) (front, body []byte, err error) {
	s := bytes.TrimSpace(bytes.TrimPrefix(src, []byte{0xEF, 0xBB, 0xBF})) // tolerate a UTF-8 BOM
	if !bytes.HasPrefix(s, []byte(fmDelim)) {
		return nil, nil, fmt.Errorf("missing front matter: file must start with %q", fmDelim)
	}
	rest := s[len(fmDelim):]
	// tolerate "---\r\n" and "---\n"
	if i := bytes.IndexByte(rest, '\n'); i >= 0 {
		rest = rest[i+1:]
	} else {
		return nil, nil, fmt.Errorf("unterminated front matter")
	}
	end := bytes.Index(rest, []byte("\n"+fmDelim))
	if end < 0 {
		return nil, nil, fmt.Errorf("unterminated front matter: no closing %q", fmDelim)
	}
	front = rest[:end+1]
	body = rest[end+1+len(fmDelim):]
	return front, body, nil
}

// parseMeta reads the flat "key: value" front matter. This is a deliberate
// subset of YAML — scalars plus an inline [a, b, c] list — so the generator
// needs no YAML dependency while the files still look like every other blog's.
func parseMeta(front []byte) (Meta, error) {
	var m Meta
	sc := bufio.NewScanner(bytes.NewReader(front))
	line := 0
	for sc.Scan() {
		line++
		raw := strings.TrimSpace(sc.Text())
		if raw == "" || strings.HasPrefix(raw, "#") {
			continue
		}
		key, val, ok := strings.Cut(raw, ":")
		if !ok {
			return m, fmt.Errorf("line %d: expected \"key: value\", got %q", line, raw)
		}
		key = strings.ToLower(strings.TrimSpace(key))
		val = trimComment(strings.TrimSpace(val))

		switch key {
		case "title":
			m.Title = unquote(val)
		case "description":
			m.Description = unquote(val)
		case "cover":
			m.Cover = unquote(val)
		case "video":
			m.Video = unquote(val)
		case "duration":
			m.Duration = unquote(val)
		case "date":
			d, err := parseDate(unquote(val))
			if err != nil {
				return m, fmt.Errorf("line %d: %w", line, err)
			}
			m.Date = d
		case "tags":
			m.Tags = parseList(val)
		case "draft":
			b, err := strconv.ParseBool(unquote(val))
			if err != nil {
				return m, fmt.Errorf("line %d: draft must be true or false, got %q", line, val)
			}
			m.Draft = b
		default:
			return m, fmt.Errorf("line %d: unknown front matter key %q", line, key)
		}
	}
	if err := sc.Err(); err != nil {
		return m, err
	}
	if m.Title == "" {
		return m, fmt.Errorf("front matter is missing a title")
	}
	if m.Date.IsZero() {
		return m, fmt.Errorf("front matter is missing a date")
	}
	if m.Description == "" {
		return m, fmt.Errorf("front matter is missing a description (it is the OG card and list subtitle)")
	}
	return m, nil
}

func parseDate(s string) (time.Time, error) {
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognised date %q, want 2006-01-02 or RFC3339", s)
}

// parseList reads an inline [a, b, c] list; a bare scalar counts as one item.
func parseList(s string) []string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "[")
	s = strings.TrimSuffix(s, "]")
	var out []string
	for _, part := range strings.Split(s, ",") {
		if v := unquote(strings.TrimSpace(part)); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func unquote(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			if unq, err := strconv.Unquote(s); err == nil {
				return unq
			}
			return s[1 : len(s)-1]
		}
	}
	return s
}

// trimComment drops a trailing "# ..." comment, but only outside quotes so a
// title containing a hash survives.
func trimComment(s string) string {
	inQuote := byte(0)
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case inQuote != 0:
			if c == inQuote {
				inQuote = 0
			}
		case c == '"' || c == '\'':
			inQuote = c
		case c == '#' && i > 0 && (s[i-1] == ' ' || s[i-1] == '\t'):
			return strings.TrimSpace(s[:i])
		}
	}
	return s
}
