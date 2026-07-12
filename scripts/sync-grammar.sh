#!/usr/bin/env bash
# Publish the tree-sitter-edbml grammar to a local git mirror and point the
# Zed extension at it.
#
# Zed loads extension grammars from a git repository (repository + commit in
# extension.toml) and supports file:// URLs for local development. This
# script keeps that contract without polluting the main repo with a nested
# git checkout:
#
#   1. (optional) regenerate src/parser.c if a tree-sitter CLI is available
#   2. mirror tree-sitter-edbml/ into ~/.cache/edbml/tree-sitter-edbml-git
#      and commit the current state there
#   3. rewrite [grammars.edbml] in zed-extension/extension.toml with the
#      mirror's file:// URL and fresh commit hash
#
# Run it after cloning and after every grammar change, then use
# `zed: install dev extension` (first time) or Rebuild (afterwards).
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
grammar_src="$repo_root/tree-sitter-edbml"
extension_toml="$repo_root/zed-extension/extension.toml"
mirror="${EDBML_GRAMMAR_MIRROR:-${XDG_CACHE_HOME:-$HOME/.cache}/edbml/tree-sitter-edbml-git}"

# 1. Regenerate the parser when a CLI is around (committed src/ still works
#    when it is not).
if command -v tree-sitter >/dev/null 2>&1; then
    echo "Regenerating parser with $(tree-sitter --version)..."
    (cd "$grammar_src" && tree-sitter generate)
else
    echo "tree-sitter CLI not found; using committed src/parser.c as-is."
fi

# 2. Mirror into a standalone git repo.
mkdir -p "$mirror"
find "$mirror" -mindepth 1 -maxdepth 1 ! -name .git -exec rm -rf {} +
(cd "$grammar_src" && tar cf - --exclude .git --exclude node_modules --exclude build .) |
    (cd "$mirror" && tar xf -)
cd "$mirror"
if [ ! -d .git ]; then
    git init -q
fi
git add -A
if ! git diff --cached --quiet; then
    git -c user.name=edbml-sync -c user.email=sync@localhost \
        commit -q -m "sync $(date -u +%Y-%m-%dT%H:%M:%SZ)"
fi
sha="$(git rev-parse HEAD)"

# 3. Point the extension at the mirror.
python3 - "$extension_toml" "file://$mirror" "$sha" <<'PY'
import re, sys
path, url, sha = sys.argv[1:]
text = open(path).read()
text = re.sub(r'(?ms)(\[grammars\.edbml\]\n)repository = "[^"]*"\ncommit = "[^"]*"',
              lambda m: f'{m.group(1)}repository = "{url}"\ncommit = "{sha}"', text)
open(path, "w").write(text)
PY

echo "Grammar mirrored at: $mirror"
echo "extension.toml now pins commit ${sha:0:12}"
echo
echo "Next: in Zed run 'zed: extensions' and click Install Dev Extension"
echo "(first time) or Rebuild on the EDBML extension."
echo "Note: extension.toml's grammar lines are machine-local; avoid"
echo "committing that change."
