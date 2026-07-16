# Ecto documentation (reference copies)

The Ecto documentation used to build
[`../../orm-capability-matrix.md`](../../orm-capability-matrix.md) — the
feature-by-feature inventory of what an ORM does, with Not an ORM's verdict
per capability. Of all the surveyed toolkits, Ecto sits closest to this
project's shape: explicit data mapping over a hidden model layer, queries
as composable data, migrations as code. Refresh with `./fetch.sh`.

Two kinds of files, from https://github.com/elixir-ecto/ecto,
https://github.com/elixir-ecto/ecto_sql and
https://github.com/elixir-sqlite/ecto_sqlite3 (the community SQLite3
adapter — another project's answer to mapping the full Ecto surface onto
SQLite, so its type-mapping choices and documented limitations are prior
art for D02):

- **Guides** (`getting_started.md`, `dynamic_queries.md`,
  `safe_migrations.md`, …) — verbatim copies of `guides/` from both repos,
  filenames snake_cased; `*_cheatsheet.md` are the ExDoc cheatsheets.
- **Module reference** (`ecto.schema.md`, `ecto.changeset.md`,
  `ecto.migration.md`, …) — unlike Rails, most of Ecto's manual lives in
  `@moduledoc`/`@doc` heredocs inside the Elixir sources (what hexdocs.pm
  renders). `moduledocs_extract.py` copies those out verbatim, adding only
  provenance/section headings; `@doc false` internals are skipped, as on
  hexdocs. The curated module list in `fetch.sh` covers the
  capability-comparison surface: schema (+ metadata), changeset, query
  (+ query API), repo, multi, types, and from ecto_sql migrations, the
  SQL adapter, the test sandbox and the Mix tasks documenting the
  migration/database workflow (`mix.tasks.ecto.migrate.md`, …) —
  adapter-implementer behaviours are left out. From ecto_sqlite3: the
  adapter moduledoc (`ecto.adapters.sqlite3.md` — configuration, type
  mapping, SQLite limitations), the TypeExtension behaviour, and its
  README (`ecto_sqlite3_readme.md`), which is that package's hexdocs
  front page.

These documents are © Plataformatec and the Ecto contributors
([Apache-2.0](https://github.com/elixir-ecto/ecto/blob/master/LICENSE)),
except the ecto_sqlite3 files, © Matthew A. Johnston and contributors
([MIT](https://github.com/elixir-sqlite/ecto_sqlite3/blob/main/LICENSE));
all kept here for offline reference across working sessions.
