#!/usr/bin/env bash
# Build edbml-ls and install it where Zed can find it.
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
gobin="${GOBIN:-$(go env GOPATH)/bin}"

# Version managers (asdf, mise, goenv) often relocate GOPATH, so say where
# the binary is going before it goes there. Override with:
#   GOBIN="$HOME/go/bin" ./scripts/install-ls.sh
echo "Installing to: $gobin  (GOBIN='${GOBIN:-}', GOPATH='$(go env GOPATH)')"

cd "$repo_root"
go build -o "$gobin/edbml-ls" ./cmd/edbml-ls
echo "Installed: $gobin/edbml-ls ($("$gobin/edbml-ls" --version))"

case ":$PATH:" in
*":$gobin:"*) ;;
*)
    echo
    echo "WARNING: $gobin is not on your PATH. Either add it, or point Zed"
    echo "directly at the binary in settings.json:"
    echo '  "lsp": { "edbml-ls": { "binary": { "path": "'"$gobin"'/edbml-ls" } } }'
    ;;
esac
