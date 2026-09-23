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

	if err := build(content, tutorials, siteDir, out, "https://example.com", false); err != nil {
		t.Fatalf("build: %v", err)
	}

	for _, p := range []string{
		"index.html", "robots.txt", "sitemap.xml",
		"articles/index.html", "articles/index.json", "articles/feed.xml",
		"articles/morphing-a-globe/index.html",
		"tutorials/index.html", "tutorials/first-steps/index.html",
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

	t.Run("landing_page_is_rendered_not_copied", func(t *testing.T) {
		page := readString(t, filepath.Join(out, "index.html"))
		for _, want := range []string{
			// The test base URL, so these can only come from the template.
			`<link rel="canonical" href="https://example.com/"`,
			`"url": "https://example.com/"`, // and unescaped: ld+json is a JS context
			`"@type": "SoftwareApplication"`,
			`id="stars"`,
			`href="/#partners"`,
		} {
			if !strings.Contains(page, want) {
				t.Errorf("landing page is missing %s", want)
			}
		}
	})

	// Stylesheets are handed out per page. Getting this wrong is invisible in a
	// browser and only shows up as pages that are needlessly large.
	t.Run("stylesheets_go_only_where_needed", func(t *testing.T) {
		for _, c := range []struct {
			page, want, notWant string
		}{
			{"index.html", ".hero", ".chroma"},                           // no code block on /
			{"index.html", ".partner-panel", ".post-card"},               // no page components either
			{"articles/index.html", ".post-card", ".chroma"},             // a listing has no code
			{"articles/morphing-a-globe/index.html", ".chroma", ".hero"}, // …but an article does
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
			`class="chroma"`, // code was highlighted at build time
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
		if err := build(content, tutorials, siteDir, out, "https://example.com", false); err != nil {
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
		err := build(content, filepath.Join(dir, "none"), siteDir, filepath.Join(dir, "dist"), "https://example.com", false)
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
	err := build(filepath.Join(dir, "articles"), tutorials, siteDir, filepath.Join(dir, "dist"), "https://example.com", false)
	if err == nil || !strings.Contains(err.Error(), "video") {
		t.Fatalf("err = %v, want a complaint about the missing video id", err)
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
