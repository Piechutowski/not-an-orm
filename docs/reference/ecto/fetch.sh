#!/usr/bin/env bash
# Fetch the Ecto documentation that informs docs/orm-capability-matrix.md.
# Unlike Rails, Ecto's manual is split: the guides are standalone Markdown,
# but the reference documentation lives in @moduledoc/@doc heredocs inside
# the Elixir sources (what hexdocs.pm renders) — moduledocs_extract.py
# copies those out verbatim. Migrations live in the separate ecto_sql repo.
# Both projects are © Plataformatec / the Ecto contributors, Apache-2.0.
set -euo pipefail
cd "$(dirname "$0")"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

git clone --depth 1 --filter=blob:none --sparse \
  https://github.com/elixir-ecto/ecto "$tmp/ecto"
git -C "$tmp/ecto" sparse-checkout set guides lib
git clone --depth 1 --filter=blob:none --sparse \
  https://github.com/elixir-ecto/ecto_sql "$tmp/ecto_sql"
git -C "$tmp/ecto_sql" sparse-checkout set guides lib
git clone --depth 1 --filter=blob:none --sparse \
  https://github.com/elixir-sqlite/ecto_sqlite3 "$tmp/ecto_sqlite3"
git -C "$tmp/ecto_sqlite3" sparse-checkout set lib

# Guides and cheatsheets: verbatim copies, filenames normalized to the
# snake_case used by the Rails reference ("Getting Started.md" ->
# getting_started.md, crud.cheatmd -> crud_cheatsheet.md). ecto_sql's
# guides (safe migrations, backfilling, squashing) are already snake_case.
for f in "$tmp"/ecto/guides/introduction/*.md "$tmp"/ecto/guides/howtos/*.md \
         "$tmp"/ecto/guides/testing/*.md "$tmp"/ecto_sql/guides/*.md; do
  cp "$f" "$(basename "$f" | tr '[:upper:] ' '[:lower:]_')"
done
for f in "$tmp"/ecto/guides/cheatsheets/*.cheatmd; do
  cp "$f" "$(basename "$f" .cheatmd)_cheatsheet.md"
done
# ecto_sqlite3 (MIT, elixir-sqlite org): the community SQLite3 adapter —
# how another project maps the full Ecto surface onto SQLite. Its hexdocs
# front page is the repo README; the rest of its manual is the adapter
# moduledoc plus the TypeExtension behaviour, extracted below.
cp "$tmp/ecto_sqlite3/README.md" ecto_sqlite3_readme.md

# Module reference: the modules a capability comparison needs — schema,
# changeset, query, repo, multi, types (ecto) and migrations, SQL adapter,
# test sandbox, plus the Mix tasks documenting the migration/database
# workflow (ecto_sql). Adapter-implementer behaviours are skipped.
python3 moduledocs_extract.py \
  "$tmp"/ecto/lib/ecto.ex \
  "$tmp"/ecto/lib/ecto/schema.ex \
  "$tmp"/ecto/lib/ecto/schema/metadata.ex \
  "$tmp"/ecto/lib/ecto/changeset.ex \
  "$tmp"/ecto/lib/ecto/query.ex \
  "$tmp"/ecto/lib/ecto/query/api.ex \
  "$tmp"/ecto/lib/ecto/query/window_api.ex \
  "$tmp"/ecto/lib/ecto/queryable.ex \
  "$tmp"/ecto/lib/ecto/repo.ex \
  "$tmp"/ecto/lib/ecto/multi.ex \
  "$tmp"/ecto/lib/ecto/type.ex \
  "$tmp"/ecto/lib/ecto/parameterized_type.ex \
  "$tmp"/ecto/lib/ecto/enum.ex \
  "$tmp"/ecto/lib/ecto/uuid.ex \
  "$tmp"/ecto/lib/mix/tasks/ecto.create.ex \
  "$tmp"/ecto/lib/mix/tasks/ecto.drop.ex \
  "$tmp"/ecto_sql/lib/ecto/migration.ex \
  "$tmp"/ecto_sql/lib/ecto/migrator.ex \
  "$tmp"/ecto_sql/lib/ecto/adapters/sql.ex \
  "$tmp"/ecto_sql/lib/ecto/adapters/sql/sandbox.ex \
  "$tmp"/ecto_sql/lib/mix/tasks/ecto.migrate.ex \
  "$tmp"/ecto_sql/lib/mix/tasks/ecto.rollback.ex \
  "$tmp"/ecto_sql/lib/mix/tasks/ecto.migrations.ex \
  "$tmp"/ecto_sql/lib/mix/tasks/ecto.gen.migration.ex \
  "$tmp"/ecto_sql/lib/mix/tasks/ecto.dump.ex \
  "$tmp"/ecto_sql/lib/mix/tasks/ecto.load.ex \
  "$tmp"/ecto_sql/lib/mix/tasks/ecto.query.ex \
  "$tmp"/ecto_sqlite3/lib/ecto/adapters/sqlite3.ex \
  "$tmp"/ecto_sqlite3/lib/ecto/adapters/sqlite3/type_extension.ex
