#!/usr/bin/env bash
# Fetch the SQLBoiler documentation. Unlike Bob, SQLBoiler's entire manual
# is its README (features, query mods, relationships, hooks, FAQ) — one
# file. The project is in maintenance mode; Bob is its designated
# successor, which makes this the "settled" form of the SQLBoiler design.
# © Aaron L and contributors, BSD-3-Clause.
set -euo pipefail
cd "$(dirname "$0")"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
git clone --depth 1 --filter=blob:none --sparse https://github.com/aarondl/sqlboiler "$tmp"
cp "$tmp/README.md" sqlboiler_readme.md
