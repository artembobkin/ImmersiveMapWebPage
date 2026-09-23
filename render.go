package main

import (
	"bytes"
	"unicode"

	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/yuin/goldmark"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	ghtml "github.com/yuin/goldmark/renderer/html"
)

// newMarkdown builds the Markdown renderer. Code is highlighted here, at build
// time, into plain <span class="..."> markup — the published pages carry no
// highlighting library and work with JavaScript off.
func newMarkdown() goldmark.Markdown {
	return goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			extension.Typographer,
			extension.Footnote,
			highlighting.NewHighlighting(
				// The style only decides colours, and colours live in
				// site/templates/code.css so both themes can be served; all we
				// need from the renderer are the class names.
				highlighting.WithFormatOptions(chromahtml.WithClasses(true)),
			),
		),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
		goldmark.WithRendererOptions(
			// Articles are written by hand in this repo, so inline HTML (embeds,
			// figures) is trusted.
			ghtml.WithUnsafe(),
		),
	)
}

func renderMarkdown(md goldmark.Markdown, body []byte) ([]byte, error) {
	var buf bytes.Buffer
	if err := md.Convert(body, &buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// readingTime estimates minutes at 200 words per minute, counting the Markdown
// source. Close enough for a byline, and never zero.
func readingTime(body []byte) int {
	words := 0
	inWord := false
	for _, r := range string(body) {
		if unicode.IsSpace(r) {
			inWord = false
			continue
		}
		if !inWord {
			words++
			inWord = true
		}
	}
	if m := (words + 199) / 200; m > 0 {
		return m
	}
	return 1
}
