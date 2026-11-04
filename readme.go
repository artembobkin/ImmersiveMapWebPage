package main

import (
	"fmt"
	"html/template"
	"os"
	"regexp"
	"strings"
	"unicode"

	"github.com/yuin/goldmark"
)

// The landing page is the engine's README, rendered. It is the one document
// that is always kept current, because it is what GitHub shows, so the site
// shows that rather than a second telling that would have to be edited in
// step. The copy in landing/ is written by the follow-readme workflow, which
// fetches the README once an hour and commits it when it has changed; nobody
// edits it here.
//
// Nothing on the published page fetches anything: the README is read once,
// while building, and the page carries the rendered result like an article.

// readmeSource says where the README is and which repository its relative
// links point into.
type readmeSource struct {
	Path   string // the Markdown file
	Repo   string // "owner/name"
	Branch string
}

// rawBase is where a relative image lives: GitHub serves the file itself from
// here, with an image content type, which the repository page does not.
func (s readmeSource) rawBase() string {
	return "https://raw.githubusercontent.com/" + s.Repo + "/" + s.Branch + "/"
}

// blobBase is where a relative link goes: the file's page on GitHub, which is
// where the link lands when the README is read there too.
func (s readmeSource) blobBase() string {
	return "https://github.com/" + s.Repo + "/blob/" + s.Branch + "/"
}

// readme is the rendered README, ready for the landing page template.
type readme struct {
	HTML        template.HTML
	Description string // the first paragraph, as plain text, for the meta tags
}

// loadReadme reads the file. A README that is missing or empty fails the
// build: a page with nothing on it must never replace the one that is live.
func loadReadme(s readmeSource) ([]byte, error) {
	b, err := os.ReadFile(s.Path)
	if err != nil {
		return nil, fmt.Errorf("reading README: %w", err)
	}
	if len(strings.TrimSpace(string(b))) == 0 {
		return nil, fmt.Errorf("reading README: %s is empty", s.Path)
	}
	return b, nil
}

// renderReadme turns the source into the page body. The README is written to
// be read on GitHub, so two things are made good for a page that is not there:
// relative links and images are pointed back at the repository, and the
// images load lazily, because the gallery at the top is a dozen full-size
// frames.
func renderReadme(md goldmark.Markdown, src []byte, s readmeSource) (readme, error) {
	rendered, err := renderMarkdown(md, src)
	if err != nil {
		return readme{}, fmt.Errorf("rendering README: %w", err)
	}
	html := absoluteGitHubLinks(string(rendered), s.rawBase(), s.blobBase())
	html = lazyImages(html)
	return readme{
		HTML:        template.HTML(html),
		Description: readmeDescription(src),
	}, nil
}

var attrRe = regexp.MustCompile(`\b(src|href)="([^"]*)"`)

// absoluteGitHubLinks rewrites every relative src and href in the rendered
// HTML. Both the Markdown links and the raw <img> tags the README carries come
// through here, since both end up as attributes in the output.
func absoluteGitHubLinks(html, rawBase, blobBase string) string {
	return attrRe.ReplaceAllStringFunc(html, func(m string) string {
		sub := attrRe.FindStringSubmatch(m)
		attr, ref := sub[1], sub[2]
		if !isRelativeRef(ref) {
			return m
		}
		base := blobBase
		if attr == "src" {
			base = rawBase
		}
		ref = strings.TrimPrefix(ref, "./")
		return attr + `="` + base + strings.TrimLeft(ref, "/") + `"`
	})
}

// isRelativeRef is true for a path into the repository: not a URL with a
// scheme, not protocol-relative, not an in-page anchor.
func isRelativeRef(ref string) bool {
	if ref == "" || strings.HasPrefix(ref, "#") || strings.HasPrefix(ref, "//") {
		return false
	}
	// A scheme is letters followed by a colon before any slash, query or
	// fragment; "https:", "mailto:" and "data:" all stop here.
	if i := strings.IndexAny(ref, ":/?#"); i >= 0 && ref[i] == ':' {
		return false
	}
	return true
}

var imgRe = regexp.MustCompile(`<img\b[^>]*>`)

// lazyImages adds loading="lazy" to every image that does not say otherwise.
func lazyImages(html string) string {
	return imgRe.ReplaceAllStringFunc(html, func(tag string) string {
		if strings.Contains(tag, "loading=") {
			return tag
		}
		return `<img loading="lazy"` + tag[len("<img"):]
	})
}

var (
	mdLinkRe   = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	mdMarkRe   = regexp.MustCompile("[*_`]+")
	mdSpacesRe = regexp.MustCompile(`\s+`)
)

// readmeDescription is the first paragraph of prose, as plain text: the
// sentence that introduces the project on GitHub introduces it to search
// engines and link previews too. Headings, the badge row, HTML, lists and
// code fences all start with something other than a letter, so they are
// passed over.
func readmeDescription(src []byte) string {
	text := strings.ReplaceAll(string(src), "\r\n", "\n")
	for _, para := range strings.Split(text, "\n\n") {
		p := strings.TrimSpace(para)
		if p == "" {
			continue
		}
		first := []rune(p)[0]
		if !unicode.IsLetter(first) {
			continue
		}
		p = mdLinkRe.ReplaceAllString(p, "$1")
		p = mdMarkRe.ReplaceAllString(p, "")
		return mdSpacesRe.ReplaceAllString(p, " ")
	}
	return ""
}
