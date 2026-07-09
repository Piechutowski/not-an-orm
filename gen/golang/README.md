# dbml gen — Go model generation

Generates `dbml_models.go` from a checked DBML file: one struct per table,
one string-typed enum per DBML enum, `db` and `json` struct tags.

```sh
dbml gen --out ./models [--package models] schema.dbml
```

The output is deterministic (source order), depends only on the standard
library, is gofmt-clean by construction (everything passes through
`go/format.Source`), and starts with the machine-readable
`// Code generated ... DO NOT EDIT.` marker. `dbml gen` refuses to
overwrite a `dbml_models.go` that lacks that marker.

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
| `date`, `time*`, `timestamp*`, `datetime` | `time.Time` |
| `json`, `jsonb` | `json.RawMessage` |
| `bytea`, `blob` family, `binary`, `varbinary` | `[]byte` |
| enum type (optionally schema-qualified) | generated `type X string` + constants |

## Nullability

A column is required (`T`) when it has `not null`, `pk`/`primary key`
(setting or legacy flag), `increment`, or is covered by a `[pk]` index —
primary keys imply NOT NULL in SQL. Everything else is nullable and
generated as a pointer (`*T`) with `,omitempty` in its json tag. Types
where `nil` already expresses NULL (`[]byte`, `json.RawMessage`) are never
pointered.

## Naming

`snake_case` → `PascalCase` with the Go initialisms convention
(`user_id` → `UserID`, `api_key` → `APIKey`). Non-`public` schemas prefix
the type (`core.users` → `CoreUsers`) so equal base names cannot collide.
A leading digit is prefixed (`2fa_codes` → `X2faCodes`). Two names mapping
to the same Go identifier is a generation error.

## Tests

`gen_test.go` enforces four invariants over the golden corpus in
[`testdata/`](testdata/) (self-references, junction table with composite
pk, polymorphic association, one-to-one, partial composition with
overrides, nullable FKs, closure table, hostile names, every note level):

1. **golden** — byte-exact output per schema (`-update` to regenerate);
2. **gofmt-stable** — formatting a golden file is the identity;
3. **compiles** — every golden file is built with the real Go toolchain;
4. **header** — the first line matches `^// Code generated .* DO NOT EDIT\.$`.
