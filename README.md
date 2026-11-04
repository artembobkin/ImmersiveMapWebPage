# ImmersiveMap — website

The public site for [ImmersiveMap](https://github.com/artembobkin/ImmersiveMap),
the Swift + Metal map engine for SwiftUI. The landing page is the engine's
README, rendered; around it are the articles and the tutorials, plus the RSS
feed, sitemap and manifest that go with them.

It is informational and entirely static. Nothing on a page talks to a server at
runtime, so the build output is the whole deployable artefact and any static
host can serve it.

## The landing page is the README

Nothing of the landing page is written in this repository. `landing/README.md`
is a copy of the README of the ImmersiveMap repository, and the build renders
it with the same Markdown renderer the articles use, inside the site's shell.
Relative images and links in it are pointed back at the repository, so the
page shows what GitHub shows, and its first paragraph becomes the page's
description for search engines and link previews.

The copy is kept by the GitHub Actions workflow in
`.github/workflows/follow-readme.yml`: once an hour it fetches the README and,
when the copy differs, commits the new one and pushes. That push is what
Cloudflare builds from (see Deployment), so a README edit on GitHub is live
within the hour, and nobody edits `landing/README.md` by hand.

To change the landing page, change the README. To preview a README edit before
pushing it over there, `make site README=../ImmersiveMap/README.md`. A README
that is missing or empty fails the build, on purpose: a landing page with
nothing on it must never replace the one that is live.

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
landing/README.md  the engine's README, the landing page; written by the workflow
articles/          one Markdown file per article
tutorials/         one per screencast; each needs a YouTube id
site/templates/    the page templates and the stylesheets they inline
site/assets/       the social card image, served under /assets/ and cached for a year
site/root/         files copied to the root of the output (_headers)
readme.go          renders the README into the landing page
dist/              build output, not in git
```

Every page is rendered through `base.html.tmpl`, so there is exactly one nav,
one footer and one set of design tokens. Stylesheets are handed out per page —
a listing carries no syntax-highlighting palette, an article does — which is
why they are inlined rather than linked.

`site/templates/code.css` is generated from the Chroma theme rather than written
by hand; regenerate it with `make code-css`.

## Deployment

Cloudflare Workers with static assets, connected to this repository: every
push to `main` is built and published by Cloudflare, and nothing is deployed
by hand. The pushes come from two places, a person committing here and the
follow-readme workflow committing a changed README, and Cloudflare treats them
the same.

The only command the host needs is `npx wrangler deploy`: `wrangler.jsonc`
carries the build command as well as the upload, so the site is rendered and
then published from a fresh clone with nothing configured in the dashboard.
There is no Worker script, only files. A build that fails leaves the previous
deploy live; the Deployments list in the dashboard shows which commit failed
and why.

`site/root/_headers` rides along into the output, where Workers reads it as
configuration rather than serving it: a year and `immutable` for `/assets/*`,
five minutes for the pages, and the RSS content type for the feed.

`go.mod` names the Go version the build image already has, so no build
downloads a toolchain before it can start. That is what pins the dependencies
too: the newest Chroma wants a newer Go than the image ships, so this tracks
the last release that does not.

## What this site does not do

Earlier the same pages were served by the tile service behind Caddy, which also
answered `/tiles/*`, `/auth/*` and `/api/*`. The pages that could only speak
through those endpoints — the account dashboard, the sign-in screen, the
operator console and the plans page — are not part of this site, and neither are
the view counts and article votes. What is left needs no backend at all.

The hand-written landing page (hero, quick start, partner program) is gone as
well: it retold what the README says and had to be edited in step with it.
