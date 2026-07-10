# Not an ORM

**An ORM-grade developer experience for Go + SQLite, without the ORM.**
One authored file — your [DBML](SPEC.md) schema — and the boring 90% of
database code is *generated*: structs, CRUD, typed queries, DDL, and
(coming) migrations. No reflection, no runtime magic, no `Save()` methods
that lie. Plain Go you can read, grep, and step through in a debugger.

(Yes, the name works like *Not a Flamethrower*.)

```text
        schema.dbml  ──  the single source of truth (and your ER diagram)
             │
   dbml gen ─┼──► dbml_models.go    structs, enums, notes as doc comments
             ├──► dbml_queries.go   typed CRUD on a Queries handle
             └──► dbml_schema.sql   DDL + seed data, FK/CHECK/UNIQUE real
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
runtime machinery; we remove it by construction — one canonical file,
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
dbml check  schema.dbml                 # syntax + semantics (spec §4–§8)
dbml vet    schema.dbml                 # legal-but-suspicious DBML (vet/RULES.md)

# From the directory that holds schema.dbml, gen needs no arguments:
dbml gen go                             # ./schema.dbml -> ./dbml_{models,queries}.go
dbml gen sqlite                         # ./schema.dbml -> ./dbml_schema.sql

# Defaults: input ./schema.dbml, output '.', Go package 'main'. Override with
# -i/--input, -o/--out, -p/--package:
dbml gen go -i db/schema.dbml -o ./models -p models
```

Everything the CLI does is importable as a library
([D04](docs/decisions.md)): `parser`, `check`, `vet`, `gen/golang`,
`gen/sqlite`.

### Install

```sh
go install github.com/Piechutowski/not-an-orm/cmd/dbml@latest
```

This builds the `dbml` binary into your Go bin directory (`$GOBIN`, else
`$GOPATH/bin`); put that directory on your `PATH`. From a clone,
`go install ./cmd/dbml` does the same.

### Shell completion

The CLI ships completion for bash, zsh, fish and PowerShell. Source the
script for your shell (add the line to your shell rc file to make it
permanent):

```sh
source <(dbml completion bash)   # ~/.bashrc
source <(dbml completion zsh)    # ~/.zshrc
dbml completion fish | source    # fish
dbml completion pwsh             # PowerShell: pipe into your $PROFILE
```

## The DBML spec lives here too

This repository also contains the **normative DBML language
specification** — [`SPEC.md`](SPEC.md), a complete EBNF grammar with
constraints — plus the [conformance corpus](conformance/) cross-checked
against the upstream `@dbml/parse` compiler (0 disagreements). Our
extensions (`[model:]` today; `Select`, `View`, `[was:]`, `[repr:]` to
come) are a strict superset: core schemas stay valid for
[dbdiagram.io](https://dbdiagram.io) diagramming.

## Documentation

| Doc | What it is |
|---|---|
| [`docs/features.md`](docs/features.md) | the feature spec, by problem — **start here** |
| [`docs/decisions.md`](docs/decisions.md) | locked design decisions D01–D38, the law |
| [`docs/orm-capability-matrix.md`](docs/orm-capability-matrix.md) | every Rails AR capability vs. our verdict |
| [`docs/not-an-orm.md`](docs/not-an-orm.md) | the vision note |
| [`docs/the-model-layer.md`](docs/the-model-layer.md) | why M is the hard layer |
| [`SPEC.md`](SPEC.md) | the DBML language specification |
| [`vet/RULES.md`](vet/RULES.md) | every lint rule, with executable examples |

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
