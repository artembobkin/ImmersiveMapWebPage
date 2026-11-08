package main

import (
	stdhtml "html"
	"regexp"
	"strings"
	"unicode"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
)

// Chroma's Swift lexer knows the keywords and the literals, but it calls every
// other identifier a plain Name, so in code that is mostly types and method
// calls almost nothing is coloured. swiftLexer wraps it and classifies those
// names by what surrounds them, the way Xcode colours them. It replaces the
// stock lexer in the registry, which is where the Markdown renderer finds it.
type swiftLexer struct{ chroma.Lexer }

func init() {
	lexers.Register(swiftLexer{lexers.Get("swift")})
}

func (l swiftLexer) Tokenise(options *chroma.TokeniseOptions, text string) (chroma.Iterator, error) {
	it, err := l.Lexer.Tokenise(options, text)
	if err != nil {
		return nil, err
	}
	return chroma.Literator(classifySwiftNames(it.Tokens())...), nil
}

// Words Swift treats as keywords only in context, which the lexer leaves as
// names.
var swiftContextualKeywords = map[string]bool{
	"some": true, "any": true, "async": true, "await": true,
}

// classifySwiftNames gives each plain Name a type from its spelling and its
// neighbours. What is left as a Name is meant to be: argument labels and local
// values, which Xcode does not colour either.
func classifySwiftNames(tokens []chroma.Token) []chroma.Token {
	prev := chroma.Token{} // the last token that was not whitespace
	for i := range tokens {
		t := &tokens[i]
		if isSwiftSpace(*t) {
			continue
		}
		if t.Type == chroma.Name {
			t.Type = swiftNameType(*t, prev, nextSwiftToken(tokens, i))
			if t.Type == chroma.NameDecorator {
				tokens[prevIndex(tokens, i)].Type = chroma.NameDecorator // the "@"
			}
		}
		prev = *t
	}
	return tokens
}

func swiftNameType(name, prev, next chroma.Token) chroma.TokenType {
	switch {
	case prev.Type == chroma.Punctuation && prev.Value == "@":
		return chroma.NameDecorator
	case swiftContextualKeywords[name.Value]:
		return chroma.Keyword
	case unicode.IsUpper([]rune(name.Value)[0]):
		return chroma.NameClass
	case next.Type == chroma.Punctuation && strings.HasPrefix(next.Value, "("):
		return chroma.NameFunction
	case prev.Type == chroma.Punctuation && strings.HasSuffix(prev.Value, ".") && !strings.HasSuffix(prev.Value, ".."):
		return chroma.NameAttribute
	}
	return chroma.Name
}

func isSwiftSpace(t chroma.Token) bool {
	return t.Type == chroma.Text || t.Type == chroma.TextWhitespace
}

func nextSwiftToken(tokens []chroma.Token, i int) chroma.Token {
	for _, t := range tokens[i+1:] {
		if !isSwiftSpace(t) {
			return t
		}
	}
	return chroma.Token{}
}

func prevIndex(tokens []chroma.Token, i int) int {
	for j := i - 1; j >= 0; j-- {
		if !isSwiftSpace(tokens[j]) {
			return j
		}
	}
	return i
}

var inlineCodeRe = regexp.MustCompile(`(<pre[^>]*>\s*)?<code>([^<]*)</code>`)

// highlightInlineCode colours the code spans of a rendered page as Swift. It is
// for the documentation, where inline code is the engine's API; the README and
// the articles also put commands and file names in backticks. A block of code
// that was not highlighted is left alone, which is what the <pre> is matched for.
func highlightInlineCode(rendered string) string {
	lexer := chroma.Coalesce(lexers.Get("swift"))
	return inlineCodeRe.ReplaceAllStringFunc(rendered, func(m string) string {
		sub := inlineCodeRe.FindStringSubmatch(m)
		if sub[1] != "" {
			return m
		}
		spans, ok := swiftSpans(lexer, stdhtml.UnescapeString(sub[2]))
		if !ok {
			return m
		}
		return `<code class="chroma">` + spans + `</code>`
	})
}

// swiftSpans renders one code span. The class goes on the <code> because the
// colours are all scoped to .chroma. It reports false for anything that does
// not come back from the lexer intact, so a span is never altered.
func swiftSpans(lexer chroma.Lexer, code string) (string, bool) {
	it, err := lexer.Tokenise(nil, code)
	if err != nil {
		return "", false
	}
	tokens := it.Tokens()
	// The lexer ends its input with a newline of its own.
	if n := len(tokens); n > 0 && !strings.HasSuffix(code, "\n") {
		tokens[n-1].Value = strings.TrimSuffix(tokens[n-1].Value, "\n")
	}
	// A name on its own in a sentence is a property or a parameter being
	// referred to, not a local value.
	if len(tokens) == 1 && tokens[0].Type == chroma.Name {
		tokens[0].Type = chroma.NameAttribute
	}

	var out, seen strings.Builder
	for _, t := range tokens {
		seen.WriteString(t.Value)
		text := stdhtml.EscapeString(t.Value)
		class := chroma.StandardTypes[t.Type]
		if class == "" || t.Type == chroma.Error { // Error would paint a red box
			out.WriteString(text)
			continue
		}
		out.WriteString(`<span class="` + class + `">` + text + `</span>`)
	}
	return out.String(), seen.String() == code
}
