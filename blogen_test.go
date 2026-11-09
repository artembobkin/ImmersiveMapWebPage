package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// navLinks pulls the header nav out of a rendered page as "href → label" pairs.
// Deliberately a regexp over the markup rather than a DOM parse: the point is to
// compare pages against each other, and any parser good enough to be worth the
// dependency would also normalise away the differences worth catching.
var navLinkRe = regexp.MustCompile(`(?s)<nav class="nav-links">(.*?)</nav>`)
var linkRe = regexp.MustCompile(`(?s)<a href="([^"]+)"[^>]*>(.*?)</a>`)

func navLinks(t *testing.T, page string) []string {
	t.Helper()
	block := navLinkRe.FindStringSubmatch(page)
	if block == nil {
		return nil
	}
	var out []string
	for _, m := range linkRe.FindAllStringSubmatch(block[1], -1) {
		out = append(out, m[1]+" → "+strings.TrimSpace(m[2]))
	}
	return out
}

// siteDir is the real site directory: the end-to-end test renders throwaway
// content through the actual templates, so a broken template fails the tests
// rather than only showing up in a browser.
const siteDir = "site"

func writeFile(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

const goodArticle = `---
title: "Morphing a globe"
date: 2026-08-04
description: "One sentence."
tags: [metal, swift]
---

## Heading

Body text with ` + "`code`" + ` in it.

` + "```swift\nlet x = 1\n```" + `
`

// goodReadme stands in for the engine's README: the title, the badge row, a
// paragraph of prose, GitHub-style gallery markup with relative image paths,
// relative and absolute links, and a code block.
const goodReadme = `# ImmersiveMap

[![CI](https://example.org/ci.svg)](https://example.org/ci) [![License](https://example.org/l.svg)](LICENSE)

ImmersiveMap is a **pure Swift + Metal map** engine for [SwiftUI](https://developer.apple.com/swiftui/) apps.

<p align="center">
  <img src="Assets/readme-globe.png" width="49%" alt="The globe">
  <img src="./Assets/readme-city.png" width="49%" alt="A city">
</p>

## Quick Start

` + "```swift\nimport ImmersiveMap\n```" + `

![diagram](docs/pipeline.png)

See [LICENSE](LICENSE), [the sources](/Sources/Map.swift), [the start](#quick-start) and [the tracker](https://github.com/o/r/issues).

Read [the documentation](Documentation), [Camera](Documentation/Camera.md#limits) and [the style](https://github.com/o/r/blob/main/Documentation/CustomStyle.md).
`

// testReadme writes goodReadme into dir and returns the source that reads it
// from there, so the tests never touch the network.
func testReadme(t *testing.T, dir string) readmeSource {
	t.Helper()
	writeFile(t, dir, "README.md", goodReadme)
	return readmeSource{Path: filepath.Join(dir, "README.md"), Repo: "o/r", Branch: "main"}
}

// testDocs writes a small documentation folder into dir and returns where it
// is. Between them the files link to each other, to the rest of the repository
// and to an image, which is everything the link rewriting has to tell apart.
func testDocs(t *testing.T, dir string) string {
	t.Helper()
	docs := filepath.Join(dir, "documentation")
	writeFile(t, docs, "Camera.md", "# Camera\n\nThe camera looks at a point.\n\n## Limits\n\n```swift\nlet x = 1\n```\n\nSee [Custom style](CustomStyle.md#a-schema) and ![a frame](images/frame.png).\n")
	writeFile(t, docs, "CustomStyle.md", "# Custom *style*\n\nWhen the theme is not enough, see [Camera](./Camera.md), [the overview](../README.md#quick-start) and [the sources](../Sources/Map.swift).\n")
	writeFile(t, docs, "Attribution.md", "# Attribution\n\nThe list of next pages does not mention this page.\n")
	writeFile(t, docs, "GettingStarted.md", "# Getting started\n\nIn passing, see [Custom style](CustomStyle.md).\n\n## Next\n\n- [Camera](Camera.md)\n- [Custom style](CustomStyle.md)\n")
	writeFile(t, docs, "not-markdown.txt", "ignored")
	return docs
}

// TestParseMeta pins the front matter subset the generator accepts, and that a
// file missing anything required fails the build instead of publishing.
func TestParseMeta(t *testing.T) {
	t.Run("accepts_the_documented_fields", func(t *testing.T) {
		front, body, err := splitFrontMatter([]byte(goodArticle))
		if err != nil {
			t.Fatal(err)
		}
		m, err := parseMeta(front)
		if err != nil {
			t.Fatal(err)
		}
		if m.Title != "Morphing a globe" {
			t.Fatalf("Title = %q", m.Title)
		}
		if got := m.Date.Format("2006-01-02"); got != "2026-08-04" {
			t.Fatalf("Date = %s", got)
		}
		if len(m.Tags) != 2 || m.Tags[0] != "metal" || m.Tags[1] != "swift" {
			t.Fatalf("Tags = %v", m.Tags)
		}
		if !strings.Contains(string(body), "## Heading") {
			t.Fatal("body was not returned after the front matter")
		}
	})

	t.Run("optional_fields", func(t *testing.T) {
		m, err := parseMeta([]byte(`title: T
date: 2026-01-02
description: D
video: dQw4w9WgXcQ
duration: "4:12"
draft: true
cover: /assets/x.png   # trailing comment
`))
		if err != nil {
			t.Fatal(err)
		}
		if m.Video != "dQw4w9WgXcQ" || m.Duration != "4:12" || !m.Draft {
			t.Fatalf("got %+v", m)
		}
		if m.Cover != "/assets/x.png" {
			t.Fatalf("Cover = %q — the trailing comment was not stripped", m.Cover)
		}
	})

	t.Run("rejects_incomplete_or_unknown", func(t *testing.T) {
		for name, front := range map[string]string{
			"no title":       "date: 2026-01-02\ndescription: D\n",
			"no date":        "title: T\ndescription: D\n",
			"no description": "title: T\ndate: 2026-01-02\n",
			"bad date":       "title: T\ndate: 31.07.2026\ndescription: D\n",
			"unknown key":    "title: T\ndate: 2026-01-02\ndescription: D\nauthor: me\n",
			"bad draft":      "title: T\ndate: 2026-01-02\ndescription: D\ndraft: maybe\n",
		} {
			if _, err := parseMeta([]byte(front)); err == nil {
				t.Fatalf("%s: expected an error", name)
			}
		}
	})
}

func TestSplitFrontMatter(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		if _, _, err := splitFrontMatter([]byte("# Just markdown\n")); err == nil {
			t.Fatal("a file without front matter must fail the build")
		}
	})
	t.Run("unterminated", func(t *testing.T) {
		if _, _, err := splitFrontMatter([]byte("---\ntitle: T\n")); err == nil {
			t.Fatal("expected an error for a front matter block that never closes")
		}
	})
	t.Run("tolerates_a_bom", func(t *testing.T) {
		src := append([]byte{0xEF, 0xBB, 0xBF}, []byte(goodArticle)...)
		if _, _, err := splitFrontMatter(src); err != nil {
			t.Fatalf("a UTF-8 BOM should be tolerated: %v", err)
		}
	})
}

func TestReadingTime(t *testing.T) {
	if got := readingTime([]byte("one two three")); got != 1 {
		t.Fatalf("a short body gave %d minutes, want 1 (never zero)", got)
	}
	if got := readingTime([]byte(strings.Repeat("word ", 600))); got != 3 {
		t.Fatalf("600 words gave %d minutes, want 3", got)
	}
}

func TestCDATAAndXMLEscaping(t *testing.T) {
	if got := cdataSafe("a]]>b"); strings.Contains(strings.ReplaceAll(got, "]]><![CDATA[>", ""), "]]>") {
		t.Fatalf("cdataSafe left a raw ]]> in %q", got)
	}
	if got := xmlEscape(`a & b <c>`); strings.ContainsAny(got, "&<>") && !strings.Contains(got, "&amp;") {
		t.Fatalf("xmlEscape produced %q", got)
	}
}

// TestBuild runs the whole generator over throwaway content, through the real
// templates, and checks the artefacts the deploy and the server depend on.
func TestBuild(t *testing.T) {
	dir := t.TempDir()
	content, tutorials, out := filepath.Join(dir, "articles"), filepath.Join(dir, "tutorials"), filepath.Join(dir, "dist")
	writeFile(t, content, "morphing-a-globe.md", goodArticle)
	writeFile(t, content, "hidden.md", "---\ntitle: T\ndate: 2026-01-01\ndescription: D\ndraft: true\n---\n")
	writeFile(t, tutorials, "first-steps.md", "---\ntitle: First steps\ndate: 2026-08-01\ndescription: D\nvideo: abc123\n---\n")
	rs := testReadme(t, dir)
	docs := testDocs(t, dir)

	if err := build(content, tutorials, docs, siteDir, out, "https://example.com", false, rs); err != nil {
		t.Fatalf("build: %v", err)
	}

	for _, p := range []string{
		"index.html", "robots.txt", "sitemap.xml",
		"articles/index.html", "articles/index.json", "articles/feed.xml",
		"articles/morphing-a-globe/index.html",
		"tutorials/index.html", "tutorials/first-steps/index.html",
		"documentation/index.html", "documentation/camera/index.html", "documentation/custom-style/index.html",
		"404.html",
	} {
		if _, err := os.Stat(filepath.Join(out, p)); err != nil {
			t.Errorf("missing artefact %s", p)
		}
	}
	// The site is informational: a page that could only speak through an API is
	// not part of it, and nothing here may quietly render one again.
	for _, p := range []string{"login", "account", "admin", "price"} {
		if _, err := os.Stat(filepath.Join(out, p)); err == nil {
			t.Errorf("%s/ was rendered, but it needs a backend the site does not have", p)
		}
	}

	// Every page wears the same chrome, so every page must show the same nav.
	// This is the check that was missing: the landing page used to be a
	// hand-written file copied in verbatim, its nav grew two links that the
	// generated pages never got, and nothing noticed until someone clicked
	// through from the home page and watched half the tabs disappear.
	t.Run("the_nav_is_one_nav", func(t *testing.T) {
		pages := []string{
			"index.html", "articles/index.html", "articles/morphing-a-globe/index.html",
			"tutorials/index.html", "tutorials/first-steps/index.html",
			"documentation/index.html", "documentation/camera/index.html",
		}
		var want []string
		for _, p := range pages {
			got := navLinks(t, readString(t, filepath.Join(out, p)))
			if len(got) == 0 {
				t.Fatalf("%s has no nav links at all", p)
			}
			if want == nil {
				want = got
				continue
			}
			if !slices.Equal(got, want) {
				t.Errorf("%s nav = %v, want the same as %s: %v", p, got, pages[0], want)
			}
		}
		t.Logf("shared nav: %v", want)
	})

	// The landing page is the README, rendered through the shell: nothing on it
	// is written in this repository except the chrome around it.
	t.Run("landing_page_is_the_readme", func(t *testing.T) {
		page := readString(t, filepath.Join(out, "index.html"))
		for _, want := range []string{
			// The test base URL, so these can only come from the template.
			`<link rel="canonical" href="https://example.com/"`,
			`"url": "https://example.com/"`, // and unescaped: ld+json is a JS context
			`"@type": "SoftwareApplication"`,
			`<h1 id="immersivemap">ImmersiveMap</h1>`,
			`class="post-body"`,
			`class="chroma"`, // the README's code block, highlighted at build time
			// Relative paths now point at the repository: images at the raw
			// file, links at the file's page on GitHub.
			`src="https://raw.githubusercontent.com/o/r/main/Assets/readme-globe.png"`,
			`src="https://raw.githubusercontent.com/o/r/main/Assets/readme-city.png"`,
			`src="https://raw.githubusercontent.com/o/r/main/docs/pipeline.png"`,
			`href="https://github.com/o/r/blob/main/LICENSE"`,
			`href="https://github.com/o/r/blob/main/Sources/Map.swift"`,
			`href="#quick-start"`,
			`href="https://github.com/o/r/issues"`,
			// The site has these pages, so a visitor stays on the site.
			`href="/documentation/">the documentation`,
			`href="/documentation/camera/#limits"`,
			`href="/documentation/custom-style/">the style`,
			`<img loading="lazy" src="https://raw.githubusercontent.com/o/r/main/Assets/readme-globe.png" width="49%"`,
			// The first paragraph, as text, is the description everywhere. In an
			// attribute html/template writes the plus sign as an entity.
			`<meta name="description" content="ImmersiveMap is a pure Swift &#43; Metal map engine for SwiftUI apps." />`,
			`"description": "ImmersiveMap is a pure Swift + Metal map engine for SwiftUI apps."`,
		} {
			if !strings.Contains(page, want) {
				t.Errorf("landing page is missing %s", want)
			}
		}
		for _, stale := range []string{`id="stars"`, `href="/#partners"`, `class="hero"`} {
			if strings.Contains(page, stale) {
				t.Errorf("landing page still carries %s from the hand-written page", stale)
			}
		}
	})

	// The documentation is the folder on GitHub, rendered: the pages link to
	// each other here, and to everything else over there.
	t.Run("documentation_is_the_folder_on_github", func(t *testing.T) {
		index := readString(t, filepath.Join(out, "documentation", "index.html"))
		for _, want := range []string{
			`<link rel="canonical" href="https://example.com/documentation/getting-started/"`, // it opens on the first page
			`<a href="/documentation/getting-started/" aria-current="page">Getting started</a>`,
			`<a href="/documentation/custom-style/">Custom style</a>`,
			`<p>In passing, see`,
			`<a href="/documentation/" aria-current="page">Documentation</a>`,
		} {
			if !strings.Contains(index, want) {
				t.Errorf("documentation index is missing %s", want)
			}
		}
		camera := readString(t, filepath.Join(out, "documentation", "camera", "index.html"))
		for _, want := range []string{
			`<link rel="canonical" href="https://example.com/documentation/camera/"`,
			`<h1>Camera</h1>`,
			`class="docs-nav"`,
			`<a href="#limits">Limits</a>`,
			`<a class="next" rel="next" href="/documentation/custom-style/"><span>Next</span>Custom style</a>`,
			`class="chroma"`,
			`href="/documentation/custom-style/#a-schema"`,
			`src="https://raw.githubusercontent.com/o/r/main/Documentation/images/frame.png"`,
		} {
			if !strings.Contains(camera, want) {
				t.Errorf("camera page is missing %s", want)
			}
		}
		// The first page, then its Next section, then whatever that does not
		// mention. Custom style is linked earlier on the first page, which
		// must not move it ahead of Camera.
		order := []string{">Getting started</a>", ">Camera</a>", ">Custom style</a>", ">Attribution</a>"}
		for i := 1; i < len(order); i++ {
			if strings.Index(camera, order[i-1]) >= strings.Index(camera, order[i]) {
				t.Errorf("%s should come before %s in the sidebar", order[i-1], order[i])
			}
		}
		if strings.Contains(index, `rel="prev"`) {
			t.Error("the first page has a previous page")
		}
		if strings.Contains(camera, `<h1 id="camera">`) {
			t.Error("camera page carries its title twice")
		}
		custom := readString(t, filepath.Join(out, "documentation", "custom-style", "index.html"))
		for _, want := range []string{
			`href="/documentation/camera/"`,
			`href="/#quick-start"`,
			`href="https://github.com/o/r/blob/main/Sources/Map.swift"`,
		} {
			if !strings.Contains(custom, want) {
				t.Errorf("custom style page is missing %s", want)
			}
		}
		for _, p := range []string{"index.html", "documentation/index.html", "documentation/camera/index.html"} {
			page := readString(t, filepath.Join(out, p))
			if strings.Contains(page, "blob/main/Documentation") || strings.Contains(page, "tree/main/Documentation") {
				t.Errorf("%s links to the documentation on GitHub", p)
			}
		}
		if _, err := os.Stat(filepath.Join(out, "documentation", "not-markdown")); err == nil {
			t.Error("a file that is not Markdown was rendered")
		}
		sitemap := readString(t, filepath.Join(out, "sitemap.xml"))
		if strings.Contains(sitemap, "<loc>https://example.com/documentation/</loc>") {
			t.Error("sitemap lists /documentation/, which is the first page under a second address")
		}
		for _, want := range []string{"https://example.com/documentation/camera/", "https://example.com/documentation/custom-style/"} {
			if !strings.Contains(sitemap, "<loc>"+want+"</loc>") {
				t.Errorf("sitemap is missing %s", want)
			}
		}
	})

	// Stylesheets are handed out per page. Getting this wrong is invisible in a
	// browser and only shows up as pages that are needlessly large.
	t.Run("stylesheets_go_only_where_needed", func(t *testing.T) {
		for _, c := range []struct {
			page, want, notWant string
		}{
			{"index.html", ".readme .post-body", "#stars"},                 // the README's own rules, not the old hero's
			{"index.html", ".chroma", ".hero"},                             // and Chroma, for its code block
			{"articles/index.html", ".post-card", ".chroma"},               // a listing has no code
			{"articles/index.html", ".post-card", ".readme"},               // and is not the README
			{"articles/index.html", ".post-card", ".docs-layout"},          // nor documentation
			{"documentation/camera/index.html", ".docs-layout", ".readme"}, // documentation has its columns
			{"articles/morphing-a-globe/index.html", ".chroma", ".readme"}, // an article has code, no README rules
		} {
			page := readString(t, filepath.Join(out, c.page))
			if !strings.Contains(page, c.want) {
				t.Errorf("%s is missing %s", c.page, c.want)
			}
			if strings.Contains(page, c.notWant) {
				t.Errorf("%s carries %s, which it cannot use", c.page, c.notWant)
			}
		}
	})

	t.Run("drafts_stay_unpublished", func(t *testing.T) {
		if _, err := os.Stat(filepath.Join(out, "articles", "hidden")); err == nil {
			t.Fatal("a draft was published")
		}
	})

	t.Run("manifest_is_the_allowlist", func(t *testing.T) {
		// The tile server reads this file to decide which slugs may be voted on,
		// so its shape is a contract, not an implementation detail.
		body, err := os.ReadFile(filepath.Join(out, "articles", "index.json"))
		if err != nil {
			t.Fatal(err)
		}
		var mf struct {
			Articles []struct {
				Slug, Title, URL string
			} `json:"articles"`
		}
		if err := json.Unmarshal(body, &mf); err != nil {
			t.Fatal(err)
		}
		if len(mf.Articles) != 1 || mf.Articles[0].Slug != "morphing-a-globe" {
			t.Fatalf("manifest = %+v, want exactly the one published article", mf.Articles)
		}
		if mf.Articles[0].URL != "/articles/morphing-a-globe/" {
			t.Fatalf("URL = %q", mf.Articles[0].URL)
		}
	})

	t.Run("article_page_is_complete", func(t *testing.T) {
		page := readString(t, filepath.Join(out, "articles", "morphing-a-globe", "index.html"))
		for _, want := range []string{
			`<link rel="canonical" href="https://example.com/articles/morphing-a-globe/"`,
			`property="og:title"`,
			`"@type": "BlogPosting"`,
			`class="chroma"`,    // code was highlighted at build time
			`class="post-body"`, // the rendered Markdown itself
		} {
			if !strings.Contains(page, want) {
				t.Errorf("article page is missing %s", want)
			}
		}
	})

	t.Run("tutorial_embeds_its_video", func(t *testing.T) {
		page := readString(t, filepath.Join(out, "tutorials", "first-steps", "index.html"))
		for _, want := range []string{"youtube-nocookie.com/embed/abc123", `"@type": "VideoObject"`} {
			if !strings.Contains(page, want) {
				t.Errorf("tutorial page is missing %s", want)
			}
		}
	})

	// The 404 page answers for addresses that are not its own, so a canonical
	// link or a sitemap entry would point search engines at a page that is only
	// ever a stand-in.
	t.Run("the_404_page_claims_no_url", func(t *testing.T) {
		page := readString(t, filepath.Join(out, "404.html"))
		if !strings.Contains(page, `name="robots" content="noindex"`) {
			t.Error("404 page is not marked noindex")
		}
		if strings.Contains(page, `rel="canonical"`) {
			t.Error("404 page carries a canonical link")
		}
		if sitemap := readString(t, filepath.Join(out, "sitemap.xml")); strings.Contains(sitemap, "404") {
			t.Error("sitemap lists the 404 page")
		}
	})

	t.Run("feed_and_sitemap_are_absolute", func(t *testing.T) {
		feed := readString(t, filepath.Join(out, "articles", "feed.xml"))
		if !strings.Contains(feed, "<link>https://example.com/articles/morphing-a-globe/</link>") {
			t.Error("feed item is missing its absolute link")
		}
		if !strings.Contains(feed, "<content:encoded><![CDATA[") {
			t.Error("feed does not carry the full article body")
		}
		sitemap := readString(t, filepath.Join(out, "sitemap.xml"))
		for _, want := range []string{"https://example.com/", "https://example.com/tutorials/first-steps/"} {
			if !strings.Contains(sitemap, "<loc>"+want+"</loc>") {
				t.Errorf("sitemap is missing %s", want)
			}
		}
		if !strings.Contains(sitemap, "<loc>https://example.com/articles/</loc>") {
			t.Error("sitemap is missing the article index")
		}
	})

	t.Run("rebuild_drops_removed_content", func(t *testing.T) {
		if err := os.Remove(filepath.Join(content, "morphing-a-globe.md")); err != nil {
			t.Fatal(err)
		}
		if err := build(content, tutorials, docs, siteDir, out, "https://example.com", false, rs); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(out, "articles", "morphing-a-globe")); err == nil {
			t.Fatal("a deleted article survived the rebuild — the output directory was not reset")
		}
	})
}

// TestBuildRejectsBadSlugs covers the names that would collide with a path the
// build already writes, or produce an unusable URL.
func TestBuildRejectsBadSlugs(t *testing.T) {
	for _, name := range []string{"articles.md", "assets.md", "Bad Name.md", "-leading.md"} {
		dir := t.TempDir()
		content := filepath.Join(dir, "articles")
		writeFile(t, content, name, goodArticle)
		err := build(content, filepath.Join(dir, "none"), testDocs(t, dir), siteDir, filepath.Join(dir, "dist"), "https://example.com", false, testReadme(t, dir))
		if err == nil {
			t.Errorf("%s: expected the build to fail", name)
		}
	}
}

// TestTutorialNeedsVideo pins that a tutorial without a video id is a build
// error: the page would render an empty player.
func TestTutorialNeedsVideo(t *testing.T) {
	dir := t.TempDir()
	tutorials := filepath.Join(dir, "tutorials")
	writeFile(t, tutorials, "no-video.md", "---\ntitle: T\ndate: 2026-01-01\ndescription: D\n---\n")
	err := build(filepath.Join(dir, "articles"), tutorials, testDocs(t, dir), siteDir, filepath.Join(dir, "dist"), "https://example.com", false, testReadme(t, dir))
	if err == nil || !strings.Contains(err.Error(), "video") {
		t.Fatalf("err = %v, want a complaint about the missing video id", err)
	}
}

// TestBuildNeedsReadme pins that a README that cannot be read is a build
// error, and one raised before the output is touched: a deploy that failed to
// get the README must leave the previous site standing, not publish a landing
// page with nothing on it.
func TestBuildNeedsReadme(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "dist")
	writeFile(t, out, "sentinel.html", "the previous build")
	rs := readmeSource{Path: filepath.Join(dir, "missing.md"), Repo: "o/r", Branch: "main"}
	err := build(filepath.Join(dir, "articles"), filepath.Join(dir, "tutorials"), testDocs(t, dir), siteDir, out, "https://example.com", false, rs)
	if err == nil || !strings.Contains(err.Error(), "README") {
		t.Fatalf("err = %v, want a complaint about the README", err)
	}
	if _, err := os.Stat(filepath.Join(out, "sentinel.html")); err != nil {
		t.Fatal("the output was wiped although the build had nothing to put there")
	}
}

// TestBuildNeedsDocumentation pins that documentation that is missing or has
// nothing in it stops the build before the output is touched, as the README does.
func TestBuildNeedsDocumentation(t *testing.T) {
	for name, setup := range map[string]func(dir string) string{
		"missing": func(dir string) string { return filepath.Join(dir, "none") },
		"no pages": func(dir string) string {
			writeFile(t, filepath.Join(dir, "d"), "a.txt", "x")
			return filepath.Join(dir, "d")
		},
		"empty page": func(dir string) string {
			writeFile(t, filepath.Join(dir, "d"), "Camera.md", " \n")
			return filepath.Join(dir, "d")
		},
		"same url": func(dir string) string {
			writeFile(t, filepath.Join(dir, "d"), "CustomStyle.md", "# A\n")
			writeFile(t, filepath.Join(dir, "d"), "custom-style.md", "# B\n")
			return filepath.Join(dir, "d")
		},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			out := filepath.Join(dir, "dist")
			writeFile(t, out, "sentinel.html", "the previous build")
			err := build(filepath.Join(dir, "articles"), filepath.Join(dir, "tutorials"), setup(dir), siteDir, out, "https://example.com", false, testReadme(t, dir))
			if err == nil || !strings.Contains(err.Error(), "documentation") {
				t.Fatalf("err = %v, want a complaint about the documentation", err)
			}
			if _, err := os.Stat(filepath.Join(out, "sentinel.html")); err != nil {
				t.Fatal("the output was wiped although the build had nothing to put there")
			}
		})
	}
}

func TestDocSlug(t *testing.T) {
	for in, want := range map[string]string{
		"Camera": "camera", "CustomStyle": "custom-style", "TileSource": "tile-source",
		"PMTilesGuide": "pmtiles-guide", "Two words": "two-words", "under_score": "under-score",
		"3DContent": "3d-content", "Layer3Data": "layer3-data", "MarkersAndTaps": "markers-and-taps",
	} {
		if got := docSlug(in); got != want {
			t.Errorf("docSlug(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestReadmeLinks covers the rewriting of what the README says relative to
// its repository, and what is left alone.
func TestReadmeLinks(t *testing.T) {
	raw, blob := "https://raw.githubusercontent.com/o/r/main/", "https://github.com/o/r/blob/main/"
	rs := readmeSource{Repo: "o/r", Branch: "main"}
	for _, c := range []struct{ in, want string }{
		{`<img src="Assets/a.png">`, `<img src="` + raw + `Assets/a.png">`},
		{`<img src="./Assets/a.png">`, `<img src="` + raw + `Assets/a.png">`},
		{`<img src="/Assets/a.png">`, `<img src="` + raw + `Assets/a.png">`},
		{`<a href="LICENSE">`, `<a href="` + blob + `LICENSE">`},
		{`<a href="docs/x.md#part">`, `<a href="` + blob + `docs/x.md#part">`},
		{`<a href="#quick-start">`, `<a href="#quick-start">`},
		{`<a href="https://example.org/a?b=1&amp;c=2">`, `<a href="https://example.org/a?b=1&amp;c=2">`},
		{`<a href="mailto:me@example.org">`, `<a href="mailto:me@example.org">`},
		{`<a href="//cdn.example.org/x">`, `<a href="//cdn.example.org/x">`},
		{`<img src="data:image/png;base64,AAAA">`, `<img src="data:image/png;base64,AAAA">`},
	} {
		if got := repoLinks(c.in, "", rs, nil); got != c.want {
			t.Errorf("repoLinks(%s) = %s, want %s", c.in, got, c.want)
		}
	}

	// From inside a folder, and with pages the site has.
	pages := map[string]string{"README.md": "/", "Docs": "/d/", "Docs/A.md": "/d/a/"}
	for _, c := range []struct{ in, want string }{
		{`<a href="A.md">`, `<a href="/d/a/">`},
		{`<a href="./A.md#part">`, `<a href="/d/a/#part">`},
		{`<a href="../README.md">`, `<a href="/">`},
		{`<a href="../Docs/">`, `<a href="/d/">`},
		{`<a href="/Docs/A.md">`, `<a href="/d/a/">`},
		{`<a href="` + blob + `Docs/A.md#part">`, `<a href="/d/a/#part">`},
		{`<a href="https://github.com/o/r/tree/main/Docs">`, `<a href="/d/">`},
		{`<a href="` + blob + `LICENSE">`, `<a href="` + blob + `LICENSE">`},
		{`<a href="B.swift">`, `<a href="` + blob + `Docs/B.swift">`},
		{`<a href="../Sources/C.swift">`, `<a href="` + blob + `Sources/C.swift">`},
		{`<img src="pic.png">`, `<img src="` + raw + `Docs/pic.png">`},
	} {
		if got := repoLinks(c.in, "Docs", rs, pages); got != c.want {
			t.Errorf("repoLinks(%s) = %s, want %s", c.in, got, c.want)
		}
	}

	if got := lazyImages(`<img src="a"> <img loading="eager" src="b">`); got != `<img loading="lazy" src="a"> <img loading="eager" src="b">` {
		t.Errorf("lazyImages = %s", got)
	}

	for _, c := range []struct{ in, want string }{
		{goodReadme, "ImmersiveMap is a pure Swift + Metal map engine for SwiftUI apps."},
		{"# Title\n\n- a list\n\n<p>html</p>\n\nProse at\nlast.\n", "Prose at last."},
		{"# Title only\n", ""},
	} {
		if got := readmeDescription([]byte(c.in)); got != c.want {
			t.Errorf("readmeDescription(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestCodeCSSCoversDarkTheme pins that every token the light theme colours is
// also set in the dark one. A token left out keeps its light colour, which is
// dark text on a dark background.
func TestCodeCSSCoversDarkTheme(t *testing.T) {
	const darkScope = `:root[data-theme="dark"] `
	css := readString(t, filepath.Join(siteDir, "templates", "code.css"))
	inDark := map[string]bool{}
	var light []string
	for _, line := range strings.Split(css, "\n") {
		if rule, ok := strings.CutPrefix(line, darkScope); ok {
			inDark[ruleSelector(rule)] = true
		} else if strings.HasPrefix(line, ".chroma") && strings.Contains(line, "color:") {
			light = append(light, ruleSelector(line))
		}
	}
	if len(light) == 0 || len(inDark) == 0 {
		t.Fatal("found no rules; has the format of code.css changed?")
	}
	for _, s := range light {
		if !inDark[s] {
			t.Errorf("%s is coloured in the light theme only", s)
		}
	}
}

// TestSwiftHighlighting pins how names in Swift are coloured, through the same
// renderer the pages use.
func TestSwiftHighlighting(t *testing.T) {
	src := "```swift\n@State private var camera = Controller()\nvar body: some View {\n    MapView()\n        .tileArchive(URL(string: \"x\")!)\n        .zoom(maximum: .pi)\n}\n```\n"
	rendered, err := renderMarkdown(newMarkdown(), []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`<span class="nd">@State</span>`,
		`<span class="k">some</span>`,
		`<span class="nc">View</span>`,
		`<span class="nc">MapView</span>`,
		`<span class="nc">URL</span>`,
		`<span class="nf">tileArchive</span>`,
		`<span class="na">pi</span>`,
		`<span class="n">string</span>`, // an argument label stays plain
	} {
		if !strings.Contains(string(rendered), want) {
			t.Errorf("rendered Swift is missing %s in:\n%s", want, rendered)
		}
	}
}

// TestInlineCodeHighlighting pins what is coloured in a sentence and what is
// left exactly as it was.
func TestInlineCodeHighlighting(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{`<code>MapView()</code>`, `<code class="chroma"><span class="nc">MapView</span><span class="p">()</span></code>`},
		{`<code>Service.tileURL</code>`, `<code class="chroma"><span class="nc">Service</span><span class="p">.</span><span class="na">tileURL</span></code>`},
		{`<code>.zoom(_:)</code>`, `<span class="nf">zoom</span>`},
		{`<code>headers</code>`, `<code class="chroma"><span class="na">headers</span></code>`},
		{`<code>true</code>`, `<code class="chroma"><span class="kc">true</span></code>`},
		{`<code>a &lt; b</code>`, `&lt;`},
		{`<pre><code>not highlighted</code></pre>`, `<pre><code>not highlighted</code></pre>`},
		{`<pre class="chroma"><code><span class="k">let</span></code></pre>`, `<pre class="chroma"><code><span class="k">let</span></code></pre>`},
	} {
		got := highlightInlineCode(c.in)
		if !strings.Contains(got, c.want) {
			t.Errorf("highlightInlineCode(%s) = %s, want it to contain %s", c.in, got, c.want)
		}
		if strings.Contains(got, `class="err"`) {
			t.Errorf("highlightInlineCode(%s) marks an error: %s", c.in, got)
		}
	}
}

func readString(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
