# Bob documentation (reference copies)

The Bob documentation used to build
[`../../orm-capability-matrix.md`](../../orm-capability-matrix.md). Bob
(https://github.com/stephenafamo/bob) is the closest Go relative of this
project: database-first code generation producing typed models, query
mods, relationship loaders and factories, written by a maintainer of
SQLBoiler as its successor. Where Rails and Ecto show what a model layer
*does*, Bob shows what the same surface looks like when a Go code
generator answers it — which makes it the sharpest comparison point for
API shape (generated handles, `R` relationship structs, Preload/ThenLoad
loaders, view models, hooks). Refresh with `./fetch.sh`.

The files are a verbatim copy of `website/docs/` from the Bob repository
(the source of https://bob.stephenafamo.com/docs), directory structure
preserved; the tree's own landing page is kept as `docs_intro.md` so this
README can exist:

- `query-builder/` — the dialect-aware fluent SQL builder (starters,
  mods, operators), including per-dialect example corpora for
  psql/mysql/sqlite.
- `sql-executor/` — scanning and execution (`One`/`All`/`Cursor`,
  prepared statements).
- `models/` — the generated-code runtime surface: table/view models,
  column lists, hooks.
- `code-generation/` — the generator itself: configuration, per-driver
  setup, relationships (loading, modification, `R` structs), enums,
  factories, and `queries.md` (their sqlc-equivalent: typed code from
  hand-written SQL).
- `plugins/` — generator extension points.

Bob is © Stephen Afam-Osemene, MIT-licensed.
