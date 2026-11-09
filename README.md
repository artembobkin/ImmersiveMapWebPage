# ImmersiveMap — website

The public site for [ImmersiveMap](https://github.com/artembobkin/ImmersiveMap),
the Swift + Metal map engine for SwiftUI. The landing page is the engine's
README, rendered, and the documentation is its Documentation folder, rendered;
around them are the articles and the tutorials, plus the RSS feed, sitemap and
manifest that go with them.

It is informational and entirely static. Nothing on a page talks to a server at
runtime, so the build output is the whole deployable artefact and any static
host can serve it.

## The landing page is the README

Nothing of the landing page is written in this repository. `landing/README.md`
is a copy of the README of the ImmersiveMap repository, and the build renders
it with the same Markdown renderer the articles use, inside the site's shell.
Its images point at the repository, so the page shows what GitHub shows. Its
links stay on the site wherever the site has the page, and only what it does
not have, such as a source file, goes to GitHub. Its first paragraph becomes the page's
description for search engines and link previews.

The copy is kept by the GitHub Actions workflow in
`.github/workflows/follow-readme.yml`: it fetches the README and, when the copy
differs, commits the new one and pushes. That push is what Cloudflare builds
from (see Deployment), and nobody edits `landing/README.md` by hand.

A push to the README or the Documentation folder of the ImmersiveMap repository
starts the workflow at once: `.github/workflows/notify-website.yml` over there
sends a `repository_dispatch` event, signed with the `WEBSITE_DISPATCH_TOKEN`
secret, a token that may write to this repository's contents. The workflow also
runs once an hour, in case a notification is lost.

To change the landing page, change the README. To preview a README edit before
pushing it over there, `make site README=../ImmersiveMap/README.md`. A README
that is missing or empty fails the build, on purpose: a landing page with
nothing on it must never replace the one that is live.

## The documentation is the Documentation folder

`/documentation/` works the same way. `documentation/` is a copy of the
Markdown files directly inside the
[Documentation folder](https://github.com/artembobkin/ImmersiveMap/tree/main/Documentation)
of the ImmersiveMap repository, written by the same workflow and never edited
here: a page added, changed, renamed or deleted on GitHub is added, changed,
renamed or deleted here. Each file becomes a page (`CustomStyle.md` is
`/documentation/custom-style/`), titled by the heading it opens with. The
sidebar follows the order in which the README links to the guides, and
`/documentation/` opens on the first of them. Links follow the same rule as the landing page's:
a link to a page of the site stays on the site, and only images and files with
no page here point at the repository.

Documentation that is missing, has no pages or has an empty page fails the
build, for the same reason a bad README does.

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
documentation/     the engine's Documentation folder; written by the workflow
articles/          one Markdown file per article
tutorials/         one per screencast; each needs a YouTube id
site/templates/    the page templates and the stylesheets they inline
site/assets/       the social card image, served under /assets/ and cached for a year
site/root/         files copied to the root of the output (_headers)
readme.go          renders the README into the landing page
docs.go            renders the documentation pages
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
follow-readme workflow committing a changed README or documentation page, and
Cloudflare treats them the same.

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
