#!/usr/bin/env bash
# Extract the upstream @dbml/parse compiler source from git history (the
# commit this fork branched from) and wire in dependency shims so it runs
# under bun with no npm access. Then cross-check the snippet corpus.
set -euo pipefail
cd "$(dirname "$0")"
COMMIT=76cd9e3 # last upstream commit before this repo became spec-only
rm -rf work && mkdir work
git -C ../.. archive "$COMMIT" packages/dbml-parse | tar x --strip-components=2 -C work
mkdir -p work/node_modules
cp -r shims/* work/node_modules/
cp refcheck.ts work/
echo "setup done — run: (cd work && bun refcheck.ts ../../snippets)"
