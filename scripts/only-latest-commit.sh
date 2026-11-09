#!/bin/sh
# Stops a Cloudflare build of a commit that is no longer the newest one on main.
#
# Cloudflare builds each commit on its own and can run a build long after its
# push, and whatever finishes last is what the site serves. A build of commit
# 0d5436a ran two hours late and replaced the site with that old version. A
# build that fails leaves the live site alone, so an outdated one fails here,
# and the build of the newer commit is the one that publishes.
#
# Outside a Cloudflare build, or on another branch, this does nothing. When
# GitHub cannot be asked it does nothing either: a missed check is rare and
# harmless, a site that cannot publish is neither.

[ -n "$WORKERS_CI_COMMIT_SHA" ] || exit 0
[ "$WORKERS_CI_BRANCH" = main ] || exit 0

latest=$(git ls-remote "${SITE_REPO_URL:-https://github.com/artembobkin/ImmersiveMapWebPage.git}" refs/heads/main | cut -f1)
[ -n "$latest" ] || exit 0

if [ "$latest" != "$WORKERS_CI_COMMIT_SHA" ]; then
  echo "This build is of $WORKERS_CI_COMMIT_SHA, but main is at $latest. Not publishing an outdated commit." >&2
  exit 1
fi
