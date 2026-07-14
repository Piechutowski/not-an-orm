# Not an ORM

**An ORM-grade developer experience for Go + SQLite, without the ORM.**
One authored file — your [DBML](SPEC.md) schema — and the boring 90% of
database code is *generated*: structs, CRUD, typed queries, DDL, and
(coming) migrations. No reflection, no runtime magic, no `Save()` methods
that lie. Plain Go you can read, grep, and step through in a debugger.

> [!CAUTION]
> **This project is an alpha version, and everything may change in the future.**

```text
        schema.edbml  ──  the single source of truth (and your ER diagram)
             │
  edbml gen ─┼──► edbml_models.go    structs, enums, notes as doc comments
             ├──► edbml_queries.go   typed CRUD on a Queries handle
             └──► edbml_schema.sql   DDL + seed data, FK/CHECK/UNIQUE real
```

## Ten seconds of it

```dbml
Table users {
  id         integer   [pk, increment]
  email      varchar   [not null, unique]
  bio        text      [note: 'NULL until the user writes one']
  created_at timestamp [not null, default: `CURRENT_TIMESTAMP`]
}
```

```go
db, _ := rt.Open("sqlite3", "app.db")      // WAL, busy_timeout, foreign_keys=ON
q := models.New(db)

u, err := q.UserCreate(ctx, models.UserCreateParams{Email: "ann@example.com"})
u, err = q.UserGetByEmail(ctx, "ann@example.com")   // per unique column
err = q.Tx(ctx, func(q *models.Queries) error {     // same methods, in a tx
    _, err := q.UserUpdate(ctx, u.ID, params)
    return err
})
```

Nullable columns are `rt.Null[T]` values (JSON shows the value or
`null`), misses are `rt.ErrNotFound`, every name is subject-first
(`UserGet`, `UserList`, `OrderStatusPending`) so autocomplete groups by
model. Generated code imports the standard library and the tiny
[`rt`](rt/) runtime — nothing else, and no driver is chosen for you.

## The idea

The model layer's real job is keeping four representations of the same
truth coherent: the diagram, the DDL, the code, and the live database
([the analysis](docs/the-model-layer.md)). ORMs fight the drift with
runtime machinery; I remove it by construction — one canonical file,
everything else derived. And because SQLite is the only target (all-in,
[D02](docs/decisions.md)), SQLite itself is the gen-time type checker:
every generated statement is proven by preparing it against the generated
schema before your code ever runs.

Where this sits on the ecosystem ladder, and how "almost any query" works
without a runtime query builder, is spelled out in
[**docs/features.md**](docs/features.md) — the feature specification,
organized by problem solved, with per-feature status. The short version:

| Problem | Answer | Status |
|---|---|---|
| models + default CRUD | generated (`UserGet` … `UserDelete`) | **done (v0)** |
| custom static queries | `Select`/`View` blocks: real SQL in the schema, typed functions out, prepare-validated | v1 |
| runtime filter/order/limit | typed predicate values (`UserEmail.Eq(x)`) | v2 |
| associations | explicit per-ref loaders, batched `IN` | v3 |
| migrations | declarative diff, hash-pinned ledger, owned 12-step rebuild | v4 |
| callbacks, lazy loading, dirty tracking | never — [D27](docs/decisions.md) | — |

## The CLI

```sh
edbml check  schema.edbml               # syntax + semantics (spec §4–§8)
edbml vet    schema.edbml               # legal-but-suspicious EDBML (edbml/vet/RULES.md)

# From the directory that holds schema.edbml, gen needs no arguments:
edbml gen go                            # ./schema.edbml -> ./edbml_{models,queries}.go
edbml gen sqlite                        # ./schema.edbml -> ./edbml_schema.sql

# Defaults: input ./schema.edbml, output '.', Go package 'main'. Override with
# -i/--input, -o/--out, -p/--package:
edbml gen go -i db/schema.edbml -o ./models -p models

# -m/--models-only emits just edbml_models.go (structs/enums, no CRUD) — for
# sharing the row types across processes (e.g. gob between a server and a GUI):
edbml gen go --models-only -o ./shared -p shared

# The language server (used by the Zed extension) is a subcommand too:
edbml lsp                               # LSP over stdin/stdout
```

Everything the CLI does is importable as a library
([D04](docs/decisions.md)): `edbml/parser`, `edbml/check`, `edbml/vet`,
`gen/golang`, `gen/sqlite`.

### Install

```sh
go install github.com/Piechutowski/not-an-orm/cmd/edbml@latest
```

This builds the `edbml` binary into your Go bin directory (`$GOBIN`, else
`$GOPATH/bin`); put that directory on your `PATH`. From a clone,
`go install ./cmd/edbml` does the same.

### Shell completion

The CLI ships completion for bash, zsh, fish and PowerShell. Source the
script for your shell (add the line to your shell rc file to make it
permanent):

```sh
source <(edbml completion bash)  # ~/.bashrc
source <(edbml completion zsh)   # ~/.zshrc
edbml completion fish | source   # fish
edbml completion pwsh            # PowerShell: pipe into your $PROFILE
```

## Editor support

The schema deserves the same editor experience as the code generated from
it, so the repository also ships the editor tooling (D40):

- [`edbml/grammar/`](edbml/grammar/) — a tree-sitter grammar
  covering the full spec, for syntax highlighting;
- [`zed-extension/`](zed-extension/) — a [Zed](https://zed.dev) extension:
  highlighting, outline, auto-indent, Markdown rendered inside notes;
- [`edbml/lsp/`](edbml/lsp/) — the language server, served by `edbml lsp`,
  wrapping the same front end as the CLI, so squiggles, `edbml check` and codegen can never
  disagree: live diagnostics (check errors + vet lints), completion,
  hover, go-to-definition, find references, rename. Editor-agnostic LSP
  over stdio — works in Zed, Neovim, Helix, VS Code.

Install locally: `go install ./cmd/edbml`, then `./scripts/sync-grammar.sh`,
then Zed's `Install Dev Extension` pointed at `zed-extension/`. The design
and every pattern used: [`docs/editor-architecture.md`](docs/editor-architecture.md).

## The DBML spec lives here too

This repository also contains the **normative DBML language
specification** — [`SPEC.md`](SPEC.md), a complete EBNF grammar with
constraints — plus the [conformance corpus](edbml/conformance/) cross-checked
against the upstream `@dbml/parse` compiler (0 disagreements). My
extensions (`[model:]` today; `Select`, `View`, `[was:]`, `[repr:]` to
come) are a strict superset: core schemas stay valid for
[dbdiagram.io](https://dbdiagram.io) diagramming.

## Documentation

| Doc | What it is |
|---|---|
| [`docs/features.md`](docs/features.md) | the feature spec, by problem — **start here** |
| [`docs/decisions.md`](docs/decisions.md) | locked design decisions D01–D40, the law |
| [`docs/orm-capability-matrix.md`](docs/orm-capability-matrix.md) | every Rails AR capability vs. my verdict |
| [`docs/not-an-orm.md`](docs/not-an-orm.md) | the vision note |
| [`docs/the-model-layer.md`](docs/the-model-layer.md) | why M is the hard layer |
| [`SPEC.md`](SPEC.md) | the DBML language specification |
| [`edbml/vet/RULES.md`](edbml/vet/RULES.md) | every lint rule, with executable examples |
| [`docs/editor-architecture.md`](docs/editor-architecture.md) | the editor tooling: grammar, Zed extension, language server |

## Development

`go test ./...` runs the whole proof chain: the conformance corpus,
vet's annotation tests and docs-consistency check, generator goldens
(gofmt-stable, compiled by the real toolchain), every generated statement
prepared against the generated DDL on a real SQLite, and full CRUD round
trips through a real driver in [`itest/`](itest/) (the only cgo
dependency, test-only). Refresh goldens with `go test ./gen/... -update`;
refresh the itest fixtures with `go generate ./itest`.

## License

[Apache-2.0](LICENSE). The DBML language originates from
[holistics/dbml](https://github.com/holistics/dbml); the specification
here was written against its reference implementation.
