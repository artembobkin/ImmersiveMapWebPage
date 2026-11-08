package main

import (
	"fmt"
	stdhtml "html"
	"html/template"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// The documentation is the Documentation folder of the engine's repository,
// rendered. It works the way the landing page does: GitHub holds the text, the
// copy in documentation/ is written by the follow-readme workflow, and the
// build turns that copy into pages. Nobody edits the copy here, and nothing on
// a published page fetches anything.

// docsRepoDir is the folder on GitHub that documentation/ is a copy of, which
// is what its relative links are relative to.
const docsRepoDir = "Documentation"

// Doc is one rendered page of the documentation.
type Doc struct {
	Slug        string
	URL         string // site-relative, e.g. /documentation/camera/
	AbsURL      string
	Title       string
	Description string
	Sections    []DocSection // for the "On this page" column
	HTML        template.HTML
	file        string // the name on GitHub
}

// DocSection is one second-level heading of a page.
type DocSection struct {
	ID    string
	Title string
}

// loadDocs reads and renders every *.md directly in dir, in reading order (see
// sortDocs). As with the README, documentation that is missing or empty fails the build: a
// section with nothing in it must never replace the one that is live. The map
// it also returns is what repoLinks needs: every file on GitHub that has a page
// on this site.
func loadDocs(dir, baseURL string, rs readmeSource, readmeSrc []byte) ([]Doc, map[string]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("reading the documentation: %w", err)
	}

	// Every slug is known before anything renders, because a page links to the
	// pages after it as well as the ones before.
	slugs := map[string]string{} // path in the repository → slug
	taken := map[string]string{} // slug → the file that claimed it
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		slug := docSlug(strings.TrimSuffix(e.Name(), ".md"))
		if !slugRe.MatchString(slug) {
			return nil, nil, fmt.Errorf("documentation %s: no usable URL can be made from this filename", e.Name())
		}
		if other, ok := taken[slug]; ok {
			return nil, nil, fmt.Errorf("documentation %s and %s would share the URL /documentation/%s/", other, e.Name(), slug)
		}
		taken[slug] = e.Name()
		slugs[docsRepoDir+"/"+e.Name()] = slug
	}
	if len(slugs) == 0 {
		return nil, nil, fmt.Errorf("reading the documentation: no Markdown files in %s", dir)
	}

	pages := map[string]string{"README.md": "/", docsRepoDir: "/documentation/"}
	for repoPath, slug := range slugs {
		pages[repoPath] = "/documentation/" + slug + "/"
	}

	md := newMarkdown()
	var docs []Doc
	for repoPath, slug := range slugs {
		name := path.Base(repoPath)
		src, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, nil, fmt.Errorf("reading the documentation: %w", err)
		}
		if len(strings.TrimSpace(string(src))) == 0 {
			return nil, nil, fmt.Errorf("reading the documentation: %s is empty", name)
		}
		rendered, err := renderMarkdown(md, src)
		if err != nil {
			return nil, nil, fmt.Errorf("rendering documentation %s: %w", name, err)
		}

		title, body := splitDocTitle(string(rendered), strings.TrimSuffix(name, ".md"))
		description := readmeDescription(src)
		if description == "" {
			description = title + " in the ImmersiveMap documentation."
		}
		docs = append(docs, Doc{
			Slug:        slug,
			URL:         "/documentation/" + slug + "/",
			AbsURL:      baseURL + "/documentation/" + slug + "/",
			Title:       title,
			Description: description,
			Sections:    docSections(body),
			HTML:        template.HTML(lazyImages(highlightInlineCode(repoLinks(body, docsRepoDir, rs, pages)))),
			file:        name,
		})
	}
	sortDocs(docs, readmeSrc)
	return docs, pages, nil
}

var readmeDocRe = regexp.MustCompile(docsRepoDir + `/([^)\s"#]+\.md)`)

// sortDocs puts the pages in reading order. The folder has no order of its own
// and an alphabetical one starts on whatever sorts first, but the README
// introduces the guides in the order they are meant to be read, so that order
// is used, and it stays GitHub's to change. Pages the README does not mention
// come after, by title.
func sortDocs(docs []Doc, readmeSrc []byte) {
	rank := map[string]int{}
	for _, m := range readmeDocRe.FindAllSubmatch(readmeSrc, -1) {
		if _, seen := rank[string(m[1])]; !seen {
			rank[string(m[1])] = len(rank)
		}
	}
	position := func(d Doc) int {
		if r, ok := rank[d.file]; ok {
			return r
		}
		return len(rank)
	}
	sort.Slice(docs, func(i, j int) bool {
		if a, b := position(docs[i]), position(docs[j]); a != b {
			return a < b
		}
		return strings.ToLower(docs[i].Title) < strings.ToLower(docs[j].Title)
	})
}

var (
	docWordRe = regexp.MustCompile(`([a-z0-9])([A-Z])`)
	docSepRe  = regexp.MustCompile(`[^a-z0-9]+`)
)

// docSlug makes a URL out of a filename: CustomStyle becomes custom-style. The
// names are GitHub's to choose, so they are converted rather than required to
// be slugs already.
func docSlug(name string) string {
	s := docWordRe.ReplaceAllString(name, "$1-$2")
	s = docSepRe.ReplaceAllString(strings.ToLower(s), "-")
	return strings.Trim(s, "-")
}

var (
	docTitleRe = regexp.MustCompile(`(?s)^\s*<h1[^>]*>(.*?)</h1>\s*`)
	docTagRe   = regexp.MustCompile(`<[^>]*>`)
)

// splitDocTitle separates the heading a page opens with from the rest, since
// the template sets the title in the page head. Rendered HTML rather than the
// source, so a title gets the same typography as everything else. A file that
// opens with anything else keeps its whole body and is titled by fallback.
func splitDocTitle(rendered, fallback string) (title, body string) {
	m := docTitleRe.FindStringSubmatch(rendered)
	if m == nil {
		return fallback, rendered
	}
	title = plainText(m[1])
	if title == "" {
		title = fallback
	}
	return title, rendered[len(m[0]):]
}

var docSectionRe = regexp.MustCompile(`(?s)<h2 id="([^"]*)">(.*?)</h2>`)

// docSections lists the second-level headings of a rendered page.
func docSections(rendered string) []DocSection {
	var out []DocSection
	for _, m := range docSectionRe.FindAllStringSubmatch(rendered, -1) {
		out = append(out, DocSection{ID: m[1], Title: plainText(m[2])})
	}
	return out
}

// plainText strips the markup from a fragment of rendered HTML.
func plainText(fragment string) string {
	return strings.TrimSpace(stdhtml.UnescapeString(docTagRe.ReplaceAllString(fragment, "")))
}
