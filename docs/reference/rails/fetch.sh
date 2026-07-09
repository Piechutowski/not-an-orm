#!/usr/bin/env bash
# Fetch the Rails guides that inform docs/orm-capability-matrix.md, as
# original Markdown from the rails/rails repository (guides/source/).
# The guides are © the Rails core team, licensed CC BY-SA 4.0.
set -euo pipefail
cd "$(dirname "$0")"
tmp=$(mktemp -d)
git clone --depth 1 --filter=blob:none --sparse https://github.com/rails/rails "$tmp"
git -C "$tmp" sparse-checkout set guides/source
for g in active_record_basics active_record_migrations active_record_validations \
         active_record_callbacks association_basics active_record_querying \
         active_model_basics active_record_encryption active_record_composite_primary_keys; do
  cp "$tmp/guides/source/$g.md" .
done
rm -rf "$tmp"
