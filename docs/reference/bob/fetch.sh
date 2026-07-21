#!/usr/bin/env bash
# Fetch the Bob documentation (the website's docs tree, verbatim) that
# informs docs/orm-capability-matrix.md. Bob is the actively maintained
# successor to SQLBoiler by one of its maintainers — the closest Go
# relative of this project: database-first code generation, typed query
# mods, generated relationship loaders. © Stephen Afam-Osemene, MIT.
set -euo pipefail
cd "$(dirname "$0")"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
git clone --depth 1 --filter=blob:none --sparse https://github.com/stephenafamo/bob "$tmp"
git -C "$tmp" sparse-checkout set website/docs
find . -mindepth 1 -maxdepth 1 ! -name fetch.sh ! -name README.md -exec rm -rf {} +
cp -R "$tmp"/website/docs/* .
# website/docs has its own README.md (the docs landing page); keep it as
# docs_intro.md so this directory's curated README.md survives refreshes.
mv README.md docs_intro.md
git checkout -- README.md 2>/dev/null || true
