package main

import (
	"fmt"
	"html/template"
	"os"
	"path"
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

// blobBase is where a relative link to something with no page on this site
// goes: the file's page on GitHub.
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
// its links and images are repointed (see repoLinks), and the images load
// lazily, because the gallery at the top is a dozen full-size frames.
func renderReadme(md goldmark.Markdown, src []byte, s readmeSource, pages map[string]string) (readme, error) {
	rendered, err := renderMarkdown(md, src)
	if err != nil {
		return readme{}, fmt.Errorf("rendering README: %w", err)
	}
	html := repoLinks(string(rendered), "", s, pages)
	html = lazyImages(html)
	return readme{
		HTML:        template.HTML(html),
		Description: readmeDescription(src),
	}, nil
}

var attrRe = regexp.MustCompile(`\b(src|href)="([^"]*)"`)

// repoLinks repoints the src and href attributes of a page rendered from a
// file in the repository's dir ("" for its root). A visitor on this site stays
// on this site: a link to anything in pages, which maps a path in the
// repository to its URL here, goes to that URL, whether it was written as a
// relative path or as the full address on GitHub. Only what has no page here,
// such as a source file or an image, is pointed at GitHub.
func repoLinks(rendered, dir string, rs readmeSource, pages map[string]string) string {
	return attrRe.ReplaceAllStringFunc(rendered, func(m string) string {
		sub := attrRe.FindStringSubmatch(m)
		attr, ref := sub[1], sub[2]

		if rest, ok := cutRepoURL(ref, rs); ok {
			file, fragment := splitFragment(rest)
			if url, ok := pages[path.Clean(file)]; ok && attr == "href" {
				return `href="` + url + fragment + `"`
			}
			return m
		}
		if !isRelativeRef(ref) {
			return m
		}

		file, fragment := splitFragment(ref)
		repoPath := path.Join(dir, file)
		if strings.HasPrefix(file, "/") {
			repoPath = strings.TrimLeft(file, "/")
		}
		if url, ok := pages[repoPath]; ok && attr == "href" {
			return `href="` + url + fragment + `"`
		}
		base := rs.blobBase()
		if attr == "src" {
			base = rs.rawBase()
		}
		return attr + `="` + base + repoPath + fragment + `"`
	})
}

// cutRepoURL returns the path in the repository of a full GitHub address for a
// file or folder on the branch.
func cutRepoURL(ref string, rs readmeSource) (string, bool) {
	if rest, ok := strings.CutPrefix(ref, rs.blobBase()); ok {
		return rest, true
	}
	return strings.CutPrefix(ref, "https://github.com/"+rs.Repo+"/tree/"+rs.Branch+"/")
}

// splitFragment separates "file#part" into "file" and "#part".
func splitFragment(ref string) (file, fragment string) {
	if i := strings.IndexByte(ref, '#'); i >= 0 {
		return ref[:i], ref[i:]
	}
	return ref, ""
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
