# nao gen go — Go code generation

Generates three sibling files from a checked (E)DBML file:

| File | Contents |
|---|---|
| `nao_models.go` | one struct per table, one string-typed enum per DBML enum, `db`/`json` tags ([gen.go](gen.go)) |
| `nao_queries.go` | the fixed-shape CRUD surface: `UserGet`, `UserGetMany`, `UserGetBy*`, `UserList`, `UserCreate`, `UserUpdate`, `UserDelete` over a `Queries` handle ([queries.go](queries.go), D15-D17) |
| `nao_dyn.go` | the dynamic query layer: typed column handles (`UserEmail`), option wrappers (`UserLimit`, `UserOrderBy`, ...), predicate-driven verbs (`UserQuery`, `UserCount`, `UserExists`, `UserDeleteWhere`, `UserUpdateWhere`) ([dyn.go](dyn.go), D28-D34) |

```sh
nao gen go --out ./models --package models schema.edbml
```

The output is deterministic (source order), depends only on the standard
library plus the `rt` runtime package (D03), is gofmt-clean by
construction (everything passes through `go/format.Source`), and starts
with the machine-readable `// Code generated ... DO NOT EDIT.` marker.
`nao gen` refuses to overwrite a file that lacks that marker.

## Notes become doc comments

Documentation is written once, in the schema, and lands at the matching
Go level — godoc, IDE hovers and linters see it with zero duplication:

| DBML note on          | Generated position                        |
|-----------------------|-------------------------------------------|
| `Project`             | package comment                            |
| `Table` (body form wins over `[note: ...]`) | struct doc comment   |
| column                | field doc comment                          |
| enum value            | constant doc comment                       |
| injected partial column | travels with the column; a §6.9.4 override brings its own note |

## Type mapping

Lower-cased DBML type, parenthesized arguments ignored. An unmapped type
is a **generation error** naming the column — never a silent guess. Extend
the table in [`types.go`](types.go).

| DBML types | Go |
|---|---|
| `tinyint`, `int2`, `smallint`, `smallserial` | `int16` |
| `int`, `integer`, `int4`, `mediumint`, `serial` | `int32` |
| `bigint`, `int8`, `bigserial` | `int64` |
| `"tinyint unsigned"` / `"smallint unsigned"` / `"int unsigned"` / `"bigint unsigned"` | `uint8` / `uint16` / `uint32` / `uint64` |
| `real`, `float4` | `float32` |
| `float`, `float8`, `double`, `"double precision"` | `float64` |
| `decimal`, `numeric`, `money` | `string` (exact; money never rides a float) |
| `bool`, `boolean` | `bool` |
| `varchar`, `char`, `text` family, `citext`, `string`, `uuid` | `string` |
| `date`, `timestamp*`, `datetime` | `time.Time` |
| `time`, `timetz` (time of day) | `string` (SQLite has no time type; stored as TEXT) |
| `json`, `jsonb` | `json.RawMessage` |
| `bytea`, `blob` family, `binary`, `varbinary` | `[]byte` |
| enum type (optionally schema-qualified) | generated `type X string` + constants |

## Nullability

A column is required (`T`) when it has `not null`, `pk`/`primary key`
(setting or legacy flag), `increment`, or is covered by a `[pk]` index —
primary keys imply NOT NULL in SQL. Everything else is nullable and
generated as `rt.Null[T]` — a value plus a validity bit, never a pointer
(D13). Types where `nil` already expresses NULL (`[]byte`,
`json.RawMessage`) stay bare. In the dynamic layer, nullable columns get
`rt.NullColumn` handles: comparisons still take plain `T`, NULL is
explicit (`IsNull`, `SetNull`).

## Naming

`snake_case` → `PascalCase` with the Go initialisms convention
(`user_id` → `UserID`, `api_key` → `APIKey`). Model names are the
singular of the table name (`users` → `User`, D10), overridable with
`[model: '...']`. Non-`public` schemas prefix the type
(`core.users` → `CoreUser`) so equal base names cannot collide.
A leading digit is prefixed (`2fa_codes` → `X2faCode`). Two declarations
mapping to the same Go identifier is a generation error; the vet
`dynname` rule reports the dynamic layer's concatenation collisions
(`user_limits` → model `UserLimit` = users' `UserLimit` wrapper) ahead
of time with both origins named.

## Tests

`gen_test.go` and `queries_test.go` enforce five invariants over the
golden corpus in [`testdata/`](testdata/) (self-references, junction
table with composite pk, polymorphic association, one-to-one, partial
composition with overrides, nullable FKs, closure table, hostile names,
every note level):

1. **golden** — byte-exact output per schema and per file (`-update` to
   regenerate);
2. **gofmt-stable** — formatting a golden file is the identity;
3. **compiles** — every golden file is built with the real Go toolchain;
4. **header** — the first line matches `^// Code generated .* DO NOT EDIT\.$`;
5. **prepares** — every CRUD statement and a kitchen-sink render of every
   dynamic verb `EXPLAIN`s against the DDL `gen/sqlite` emits from the
   same schema, on a real SQLite (D02).
