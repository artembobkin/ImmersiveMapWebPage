# ImmersiveMap — website

The public site for [ImmersiveMap](https://github.com/artembobkin/ImmersiveMap),
the Swift + Metal map engine for SwiftUI: the landing page, the articles and the
tutorials, plus the RSS feed, sitemap and manifest that go with them.

It is informational and entirely static. Nothing on a page talks to a server at
runtime, so the build output is the whole deployable artefact and any static
host can serve it.

## Build

```sh
make site     # render everything into dist/
make serve    # build, then serve dist/ on http://localhost:8000
make check    # go vet + go test
```

The generator is the Go program at the repository root. `make site` wipes `dist/`
and writes it again from scratch, so a deleted article really disappears rather
than lingering from an earlier run. Drafts (`draft: true` in the front matter)
are skipped unless you build with `make drafts`.

## Layout

```
articles/          one Markdown file per article
tutorials/         one per screencast; each needs a YouTube id
site/templates/    the page templates and the stylesheets they inline
site/assets/       images, served under /assets/ and cached for a year
site/root/         files copied to the root of the output (_headers)
dist/              build output, not in git
```

Every page is rendered through `base.html.tmpl`, so there is exactly one nav,
one footer and one set of design tokens. Stylesheets are handed out per page —
an article carries the syntax-highlighting palette, the landing page does not —
which is why they are inlined rather than linked.

`site/templates/code.css` is generated from the Chroma theme rather than written
by hand; regenerate it with `make code-css`.

## Deployment

Cloudflare Pages, with `make site` as the build command and `dist` as the output
directory. `site/root/_headers` rides along into the output and sets the cache
policy: a year and `immutable` for `/assets/*`, five minutes for the pages, and
the RSS content type for the feed.

## What this site does not do

Earlier the same pages were served by the tile service behind Caddy, which also
answered `/tiles/*`, `/auth/*` and `/api/*`. The pages that could only speak
through those endpoints — the account dashboard, the sign-in screen, the
operator console and the plans page — are not part of this site, and neither are
the view counts and article votes. What is left needs no backend at all.
