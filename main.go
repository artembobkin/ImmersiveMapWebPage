// Command blogen renders the whole website into a directory of static files:
// the landing page, the articles and tutorials written in Markdown, and the
// feed, manifest, sitemap and robots.txt that go with them. Every page shares
// one shell, so there is exactly one nav, one footer and one set of design
// tokens.
//
// The output is the deployable artefact in full. Nothing here talks to a
// server at runtime: the site is informational, so a page that needed an API
// to say anything — the account dashboard, the operator console, the sign-in
// screen and the plans page — is not part of it.
//
// Usage:
//
//	blogen -content articles -site site -out dist
//	blogen -emit-code-css > site/templates/code.css
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Site holds the values every page needs.
type Site struct {
	BaseURL string
	Year    int
}

// stylesheets are the CSS files a page can inline, read once per build.
//
// Linking them from /assets/ is not an option (that path is served immutable
// for a year — see site/root/_headers), so whatever a page carries, it carries
// in full. That is why they are handed out per page rather
// than concatenated once: the landing page's hero and quick start are ten
// kilobytes that no article has any use for, and Chroma's palette is thirteen
// that a page without a code block never reads.
type stylesheets struct {
	theme template.CSS // tokens, nav, footer, buttons; every page
	pages template.CSS // components of the generated pages, the prose rules included
	code  template.CSS // Chroma; pages that render a code block
	home  template.CSS // what the README needs on top of the prose rules; / only
}

func loadStyles(tmplDir string) (stylesheets, error) {
	var s stylesheets
	for _, f := range []struct {
		name string
		dst  *template.CSS
		hint string
	}{
		{"theme.css", &s.theme, ""},
		{"pages.css", &s.pages, ""},
		{"code.css", &s.code, " (generate it with -emit-code-css)"},
		{"home.css", &s.home, ""},
	} {
		b, err := os.ReadFile(filepath.Join(tmplDir, f.name))
		if err != nil {
			return s, fmt.Errorf("reading %s%s: %w", f.name, f.hint, err)
		}
		*f.dst = template.CSS(b)
	}
	return s, nil
}

// Article is one rendered entry — an article or a tutorial — ready for the
// templates. Tutorials are the same shape with a Video attached and, usually,
// a much shorter body.
type Article struct {
	Meta
	Slug        string
	URL         string // site-relative, e.g. /articles/foo/
	AbsURL      string
	CoverAbs    string
	HTML        template.HTML
	HTMLRaw     string // same markup, unescaped, for the RSS <content:encoded>
	HasBody     bool
	ReadingTime int
	DateISO     string
	DateHuman   string
	DateRFC822  string
	TagsCSV     string
	JSONLD      template.JS
}

type pageData struct {
	Site     Site
	Section  string         // "home", "articles", …; drives the nav highlight
	Styles   []template.CSS // inlined in order by base.html.tmpl
	JSONLD   template.JS    // structured data for pages without an Article
	Articles []Article
	// The landing page only: the rendered README and what is derived from it.
	Readme      template.HTML
	Description string
	Article     *Article
	Tutorials   []Article
	Tutorial    *Article
}

// builder carries what every render step shares. It exists so the helpers stop
// threading the same four values through their signatures, and so the choice of
// stylesheets is made in one place rather than repeated per page.
type builder struct {
	tmplDir string
	outDir  string
	site    Site
	css     stylesheets
}

// page starts the data for one page. theme.css is on every page by definition;
// extra is whatever else that page actually needs.
func (b *builder) page(section string, extra ...template.CSS) pageData {
	return pageData{
		Site:    b.site,
		Section: section,
		Styles:  append([]template.CSS{b.css.theme}, extra...),
	}
}

func (b *builder) render(tmpl, path string, d pageData) error {
	t, err := parsePage(b.tmplDir, tmpl)
	if err != nil {
		return err
	}
	return writeTemplate(t, filepath.Join(b.outDir, path), d)
}

var slugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// reservedSlugs would collide with a path the build already writes. An article
// named articles.md would render to dist/articles/index.html and quietly
// replace the article index, since resetOutput wipes the tree and both writers
// claim the same path.
var reservedSlugs = map[string]bool{
	"assets": true, "index": true, "feed": true,
	"sitemap": true, "robots": true,
	"articles": true, "tutorials": true,
}

func main() {
	log.SetFlags(0)
	log.SetPrefix("blogen: ")

	var (
		contentDir  = flag.String("content", "articles", "directory holding the article Markdown files")
		tutorialDir = flag.String("tutorials", "tutorials", "directory holding the tutorial Markdown files")
		siteDir     = flag.String("site", "site", "directory holding assets/ and templates/")
		outDir      = flag.String("out", "dist", "output directory (wiped and rebuilt on every run)")
		baseURL     = flag.String("base-url", "https://immersivemap.dev", "absolute site root, no trailing slash")
		emitCSS     = flag.Bool("emit-code-css", false, "print the syntax-highlighting stylesheet and exit")
		includeDraf = flag.Bool("drafts", false, "also publish articles marked draft: true")
		readmePath  = flag.String("readme", "landing/README.md", "the README that is the landing page")
		repo        = flag.String("repo", "artembobkin/ImmersiveMap", "GitHub repository the README's relative links point into")
		branch      = flag.String("branch", "main", "branch of -repo they point into")
	)
	flag.Parse()

	if *emitCSS {
		if err := emitCodeCSS(os.Stdout); err != nil {
			log.Fatal(err)
		}
		return
	}
	rs := readmeSource{Path: *readmePath, Repo: *repo, Branch: *branch}
	if err := build(*contentDir, *tutorialDir, *siteDir, *outDir, strings.TrimRight(*baseURL, "/"), *includeDraf, rs); err != nil {
		log.Fatal(err)
	}
}

func build(contentDir, tutorialDir, siteDir, outDir, baseURL string, drafts bool, rs readmeSource) error {
	tmplDir := filepath.Join(siteDir, "templates")

	css, err := loadStyles(tmplDir)
	if err != nil {
		return err
	}
	b := &builder{
		tmplDir: tmplDir,
		outDir:  outDir,
		site:    Site{BaseURL: baseURL, Year: time.Now().UTC().Year()},
		css:     css,
	}

	articles, err := loadEntries(contentDir, baseURL, "/articles/", drafts, false)
	if err != nil {
		return err
	}
	tutorials, err := loadEntries(tutorialDir, baseURL, "/tutorials/", drafts, true)
	if err != nil {
		return err
	}
	// Before the output is touched: if the README cannot be read, the build
	// stops here with the previous output intact.
	readmeSrc, err := loadReadme(rs)
	if err != nil {
		return err
	}
	rm, err := renderReadme(newMarkdown(), readmeSrc, rs)
	if err != nil {
		return err
	}

	if err := resetOutput(outDir, siteDir); err != nil {
		return err
	}
	if err := copyTree(filepath.Join(siteDir, "assets"), filepath.Join(outDir, "assets")); err != nil {
		return err
	}
	// Files the host reads from the root of the output rather than the site
	// itself: _headers, which is how the cache policy per path is declared.
	// Optional, so a checkout without the directory still builds.
	if err := copyTreeIfPresent(filepath.Join(siteDir, "root"), outDir); err != nil {
		return err
	}

	// The landing page is the engine's README, through the same shell as
	// everything else. It used to be a hand-written page of its own, which
	// meant a second telling of what the README already says; the two drifted,
	// and keeping them in step was work nobody wanted. Now there is one text,
	// kept on GitHub, and this page follows it.
	home := b.page("home", b.css.pages, b.css.code, b.css.home)
	home.Readme = rm.HTML
	home.Description = rm.Description
	if home.JSONLD, err = softwareApplicationJSONLD(baseURL, rm.Description); err != nil {
		return err
	}
	if err := b.render("home.html.tmpl", "index.html", home); err != nil {
		return err
	}
	if err := b.renderIndex(articles); err != nil {
		return err
	}
	for i := range articles {
		if err := b.renderArticle(articles[i]); err != nil {
			return fmt.Errorf("rendering %s: %w", articles[i].Slug, err)
		}
	}
	if err := b.renderTutorialIndex(tutorials); err != nil {
		return err
	}
	for i := range tutorials {
		if err := b.renderTutorial(tutorials[i]); err != nil {
			return fmt.Errorf("rendering tutorial %s: %w", tutorials[i].Slug, err)
		}
	}
	// Served for any address that matches nothing, so it is a page like the
	// others but has no URL of its own: out of the sitemap, and noindex in the
	// template. The host is told to use it by not_found_handling in
	// wrangler.jsonc; anything else serving this output wants the same setting.
	if err := b.render("404.html.tmpl", "404.html", b.page("404", b.css.pages)); err != nil {
		return err
	}
	if err := renderFeed(tmplDir, outDir, b.site, articles); err != nil {
		return err
	}
	if err := writeManifest(outDir, articles); err != nil {
		return err
	}
	if err := writeSitemap(outDir, b.site, articles, tutorials); err != nil {
		return err
	}
	if err := writeRobots(outDir, b.site); err != nil {
		return err
	}

	log.Printf("built the README, %d article(s) and %d tutorial(s) into %s", len(articles), len(tutorials), outDir)
	return nil
}

// loadEntries reads every *.md in dir, newest first. urlPrefix is the section
// the entries live under ("/articles/" or "/tutorials/"); requireVideo marks a
// section whose entries are screencasts and must name a YouTube id.
func loadEntries(dir, baseURL, urlPrefix string, drafts, requireVideo bool) ([]Article, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		// A section with no directory yet simply renders empty — a site may
		// legitimately have tutorials before it has articles, or the reverse.
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", dir, err)
	}
	md := newMarkdown()

	var out []Article
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		slug := strings.TrimSuffix(e.Name(), ".md")
		if !slugRe.MatchString(slug) {
			return nil, fmt.Errorf("%s: filename must be a slug matching %s", path, slugRe)
		}
		if reservedSlugs[slug] {
			return nil, fmt.Errorf("%s: %q is reserved and would collide with a server path", path, slug)
		}

		src, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		front, body, err := splitFrontMatter(src)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		meta, err := parseMeta(front)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		if meta.Draft && !drafts {
			log.Printf("skipping draft %s", slug)
			continue
		}
		if requireVideo && meta.Video == "" {
			return nil, fmt.Errorf("%s: a tutorial needs a `video:` id from its YouTube URL", path)
		}
		rendered, err := renderMarkdown(md, body)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}

		cover := meta.Cover
		if cover == "" {
			cover = "/assets/og-cover.png"
		}
		a := Article{
			Meta:        meta,
			Slug:        slug,
			URL:         urlPrefix + slug + "/",
			AbsURL:      baseURL + urlPrefix + slug + "/",
			CoverAbs:    absolutise(baseURL, cover),
			HTML:        template.HTML(rendered),
			HTMLRaw:     string(rendered),
			HasBody:     len(bytes.TrimSpace(rendered)) > 0,
			ReadingTime: readingTime(body),
			DateISO:     meta.Date.Format(time.RFC3339),
			DateHuman:   meta.Date.Format("January 2, 2006"),
			DateRFC822:  meta.Date.Format(time.RFC1123Z),
			TagsCSV:     strings.Join(meta.Tags, ", "),
		}
		if requireVideo {
			a.JSONLD, err = videoObjectJSONLD(a)
		} else {
			a.JSONLD, err = blogPostingJSONLD(a, baseURL)
		}
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}

	sort.SliceStable(out, func(i, j int) bool { return out[i].Date.After(out[j].Date) })
	return out, nil
}

func blogPostingJSONLD(a Article, baseURL string) (template.JS, error) {
	doc := map[string]any{
		"@context":         "https://schema.org",
		"@type":            "BlogPosting",
		"headline":         a.Title,
		"description":      a.Description,
		"datePublished":    a.DateISO,
		"url":              a.AbsURL,
		"mainEntityOfPage": map[string]any{"@type": "WebPage", "@id": a.AbsURL},
		"image":            a.CoverAbs,
		"author":           map[string]any{"@type": "Person", "name": "Artem Bobkin", "url": "https://github.com/artembobkin"},
		"publisher":        map[string]any{"@type": "Organization", "name": "ImmersiveMap", "url": baseURL + "/"},
	}
	if len(a.Tags) > 0 {
		doc["keywords"] = a.TagsCSV
	}
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", err
	}
	return template.JS(b), nil
}

// videoObjectJSONLD describes a tutorial as a video rather than a post, which is
// what lets it show up as one in search results.
func videoObjectJSONLD(a Article) (template.JS, error) {
	doc := map[string]any{
		"@context":     "https://schema.org",
		"@type":        "VideoObject",
		"name":         a.Title,
		"description":  a.Description,
		"uploadDate":   a.DateISO,
		"thumbnailUrl": "https://i.ytimg.com/vi/" + a.Video + "/maxresdefault.jpg",
		"embedUrl":     "https://www.youtube-nocookie.com/embed/" + a.Video,
		"contentUrl":   "https://www.youtube.com/watch?v=" + a.Video,
		"url":          a.AbsURL,
		"publisher":    map[string]any{"@type": "Organization", "name": "ImmersiveMap"},
	}
	if len(a.Tags) > 0 {
		doc["keywords"] = a.TagsCSV
	}
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", err
	}
	return template.JS(b), nil
}

// softwareApplicationJSONLD describes the engine itself, for the landing page.
//
// Built here rather than written into the template because <script
// type="application/ld+json"> is a JavaScript context to html/template: a
// {{.Site.BaseURL}} placed inside it comes back as "https:\/\/…", which is legal
// JSON but not something anyone wants to meet in view-source. An ordered struct
// rather than a map, so the field order stays the one a human chose.
func softwareApplicationJSONLD(baseURL, description string) (template.JS, error) {
	type offer struct {
		Type     string `json:"@type"`
		Price    string `json:"price"`
		Currency string `json:"priceCurrency"`
	}
	doc := struct {
		Context     string `json:"@context"`
		Type        string `json:"@type"`
		Name        string `json:"name"`
		Category    string `json:"applicationCategory"`
		OS          string `json:"operatingSystem"`
		Description string `json:"description"`
		URL         string `json:"url"`
		License     string `json:"license"`
		Offers      offer  `json:"offers"`
	}{
		Context:     "https://schema.org",
		Type:        "SoftwareApplication",
		Name:        "ImmersiveMap",
		Category:    "DeveloperApplication",
		OS:          "iOS 18, macOS 15",
		Description: description,
		URL:         baseURL + "/",
		License:     "https://opensource.org/licenses/MIT",
		Offers:      offer{Type: "Offer", Price: "0", Currency: "USD"},
	}
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", err
	}
	return template.JS(b), nil
}

// The listing pages carry no code, so they stop at pages.css; the entry pages
// render Markdown that may contain a highlighted block, so they take Chroma's
// palette too. The landing page is rendered Markdown as well (the README), so
// it takes the same two and its own sheet on top.

func (b *builder) renderIndex(articles []Article) error {
	d := b.page("articles", b.css.pages)
	d.Articles = articles
	return b.render("index.html.tmpl", "articles/index.html", d)
}

func (b *builder) renderArticle(a Article) error {
	d := b.page("articles", b.css.pages, b.css.code)
	d.Article = &a
	return b.render("article.html.tmpl", filepath.Join("articles", a.Slug, "index.html"), d)
}

func (b *builder) renderTutorialIndex(tutorials []Article) error {
	d := b.page("tutorials", b.css.pages)
	d.Tutorials = tutorials
	return b.render("tutorials.html.tmpl", "tutorials/index.html", d)
}

func (b *builder) renderTutorial(t Article) error {
	d := b.page("tutorials", b.css.pages, b.css.code)
	d.Tutorial = &t
	return b.render("tutorial.html.tmpl", filepath.Join("tutorials", t.Slug, "index.html"), d)
}

// parsePage pairs the shared shell with one page template. The page's own
// {{define}} blocks override the defaults in base, so it must be parsed last.
func parsePage(tmplDir, page string) (*template.Template, error) {
	return template.ParseFiles(
		filepath.Join(tmplDir, "base.html.tmpl"),
		filepath.Join(tmplDir, page),
	)
}

func writeTemplate(t *template.Template, path string, data pageData) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return t.ExecuteTemplate(f, "base", data)
}

// resetOutput clears the output directory so deleted articles disappear from
// the bundle instead of lingering.
func resetOutput(outDir, siteDir string) error {
	abs, err := filepath.Abs(outDir)
	if err != nil {
		return err
	}
	siteAbs, err := filepath.Abs(siteDir)
	if err != nil {
		return err
	}
	// Guard against -out pointing at something that is not ours to delete.
	if abs == siteAbs || abs == filepath.Dir(abs) || filepath.Base(abs) == "" {
		return fmt.Errorf("refusing to wipe %s: pick a dedicated output directory", abs)
	}
	if err := os.RemoveAll(abs); err != nil {
		return err
	}
	return os.MkdirAll(abs, 0o755)
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

// copyTreeIfPresent is copyTree for a directory that may not exist.
func copyTreeIfPresent(src, dst string) error {
	if _, err := os.Stat(src); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return copyTree(src, dst)
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(p, target)
	})
}

func absolutise(baseURL, ref string) string {
	if strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") {
		return ref
	}
	return baseURL + "/" + strings.TrimPrefix(ref, "/")
}
