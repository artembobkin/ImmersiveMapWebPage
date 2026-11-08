package main

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

// renderFeed writes the RSS 2.0 feed. Newsletters and readers all speak RSS,
// and it is the channel a curator is most likely to subscribe to, so the feed
// carries the full article body rather than just a teaser.
//
// This one uses text/template, not html/template: the escaping rules for XML
// are different, and the two helpers below apply them explicitly.
func renderFeed(tmplDir, outDir string, site Site, articles []Article) error {
	t, err := template.New("feed.xml.tmpl").Funcs(template.FuncMap{
		"xesc":  xmlEscape,
		"cdata": cdataSafe,
	}).ParseFiles(filepath.Join(tmplDir, "feed.xml.tmpl"))
	if err != nil {
		return err
	}
	path := filepath.Join(outDir, "articles", "feed.xml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := t.Execute(f, pageData{Site: site, Articles: articles}); err != nil {
		return err
	}
	return f.Close()
}

func xmlEscape(s string) string {
	var buf bytes.Buffer
	if err := xml.EscapeText(&buf, []byte(s)); err != nil {
		return ""
	}
	return buf.String()
}

// cdataSafe makes a body safe to drop inside <![CDATA[ ... ]]> by splitting any
// literal "]]>" across two sections — the one sequence CDATA cannot contain.
func cdataSafe(s string) string {
	return strings.ReplaceAll(s, "]]>", "]]]]><![CDATA[>")
}

// manifestArticle is one entry of articles/index.json.
type manifestArticle struct {
	Slug        string   `json:"slug"`
	Title       string   `json:"title"`
	URL         string   `json:"url"`
	Date        string   `json:"date"`
	Description string   `json:"description"`
	Tags        []string `json:"tags,omitempty"`
}

// writeManifest emits articles/index.json: the machine-readable index of what
// the site has published, for anything that would rather read one file than
// crawl the pages.
func writeManifest(outDir string, articles []Article) error {
	list := make([]manifestArticle, 0, len(articles))
	for _, a := range articles {
		list = append(list, manifestArticle{
			Slug:        a.Slug,
			Title:       a.Title,
			URL:         a.URL,
			Date:        a.DateISO,
			Description: a.Description,
			Tags:        a.Tags,
		})
	}
	body, err := json.MarshalIndent(struct {
		Articles []manifestArticle `json:"articles"`
	}{Articles: list}, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(outDir, "articles", "index.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(body, '\n'), 0o644)
}

func writeSitemap(outDir string, site Site, docs []Doc, articles, tutorials []Article) error {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">` + "\n")

	entry := func(loc, lastmod, priority string) {
		b.WriteString("  <url>\n    <loc>" + xmlEscape(loc) + "</loc>\n")
		if lastmod != "" {
			b.WriteString("    <lastmod>" + lastmod + "</lastmod>\n")
		}
		b.WriteString("    <priority>" + priority + "</priority>\n  </url>\n")
	}

	newest := func(list []Article) string {
		if len(list) == 0 {
			return ""
		}
		return list[0].Date.Format("2006-01-02")
	}
	entry(site.BaseURL+"/", "", "1.0")
	for _, d := range docs {
		entry(d.AbsURL, "", "0.7")
	}
	entry(site.BaseURL+"/articles/", newest(articles), "0.8")
	for _, a := range articles {
		entry(a.AbsURL, a.Date.Format("2006-01-02"), "0.7")
	}
	if len(tutorials) > 0 {
		entry(site.BaseURL+"/tutorials/", newest(tutorials), "0.8")
		for _, t := range tutorials {
			entry(t.AbsURL, t.Date.Format("2006-01-02"), "0.7")
		}
	}
	b.WriteString("</urlset>\n")

	return os.WriteFile(filepath.Join(outDir, "sitemap.xml"), []byte(b.String()), 0o644)
}

func writeRobots(outDir string, site Site) error {
	// Every path the build writes is a page meant to be read, so nothing is
	// disallowed here.
	body := fmt.Sprintf("User-agent: *\nAllow: /\n\nSitemap: %s/sitemap.xml\n", site.BaseURL)
	return os.WriteFile(filepath.Join(outDir, "robots.txt"), []byte(body), 0o644)
}
