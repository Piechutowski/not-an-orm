# dbml gen sqlite — SQLite DDL generation

Generates `dbml_schema.sql` from a checked DBML file: `CREATE TABLE` per
table, `CREATE [UNIQUE] INDEX` per non-pk index, and `INSERT` statements
from `records` sample data.

```sh
dbml gen sqlite --out ./db schema.dbml
```

Output is deterministic (source order), starts with the
`-- Code generated ... DO NOT EDIT.` marker, and opens with
`PRAGMA foreign_keys = ON;`. `dbml gen` refuses to overwrite a
`dbml_schema.sql` lacking the marker.

## Notes become SQL comments

Project note → file header comment; table note (body form wins over
`[note: ...]`) → comment above its `CREATE TABLE`; column note → comment
above its column line. One source of documentation, same as `gen go`.

## Dialect decisions

| DBML | SQLite |
|---|---|
| enum-typed column | `TEXT` + `CHECK (col IN ('a', 'b'))` — SQLite has no enum type |
| `core.users` (schema) | `core_users` — SQLite has no schemas; collisions are generation errors |
| `pk` column | explicit `NOT NULL` added: SQLite's `PRIMARY KEY` does **not** imply NOT NULL |
| integer `[pk, increment]` | exactly `INTEGER PRIMARY KEY` (rowid alias); `AUTOINCREMENT` deliberately avoided |
| composite pk (`[pk]` index) | `PRIMARY KEY (a, b)` table constraint; members get `NOT NULL` |
| refs (`>`, `<`, `-`) | `FOREIGN KEY ... REFERENCES` with `ON DELETE` / `ON UPDATE`; FK side per spec §6.7.3 |
| one-to-one (`-`) | adds `UNIQUE` on the FK column unless something already guarantees it |
| many-to-many (`<>`) | nothing — model the junction table explicitly |
| ints / bools | `INTEGER` (booleans stored 0/1; `true`/`false` in records and defaults become 1/0) |
| floats | `REAL` |
| decimal / numeric / money | `TEXT` (exact, consistent with the Go backend's `string`) |
| strings / uuid / dates / json | `TEXT` (dates as ISO-8601, SQLite's convention) |
| bytea / blob family | `BLOB` |
| backtick expressions | verbatim; parenthesized in `DEFAULT` as SQLite requires. Dialect-owned: `now()` is yours to keep valid |
| `records` | multi-row `INSERT` statements after all tables, in source order; empty fields become `NULL` |
| unknown type | generation **error** naming the column |

Identifiers are quoted only when needed (not a plain lower-case name, or a
SQLite keyword): `order` → `"order"`, `full name` → `"full name"`,
`2fa_codes` → `"2fa_codes"`.

## Tests

The suite runs over the shared corpus in [`../testdata/`](../testdata/)
(same schemas as the Go backend):

1. **golden** — byte-exact `.sql.golden` per schema (`-update` regenerates);
2. **executes** — every golden runs against a real SQLite engine
   (`:memory:`, `PRAGMA foreign_keys = ON`, `foreign_key_check` after),
   including the seed INSERTs — the analogue of the Go compile test;
3. **header** — first line matches `^-- Code generated .* DO NOT EDIT\.$`;
4. error paths (unknown type, flattening collision) and quoting units.
