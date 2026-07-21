# Not an ORM — Feature Specification

The tool's feature list organized **by problem solved**, not by Rails
guide. This is the build plan: each feature has a stable ID (`CRUD-3`,
`SEL-2`), a slice (v0–v4), and a status; future sessions implement
feature by feature against this list.

How this document relates to the other three:

- [`the-model-layer.md`](the-model-layer.md) names the problems;
- [`orm-capability-matrix.md`](orm-capability-matrix.md) proves nothing
  Rails does is unaccounted for (the exhaustiveness audit);
- [`decisions.md`](decisions.md) records *why* each answer is what it is;
- **this file** says *what to build*, per problem.

Statuses: `DONE` (built and tested) · `vN` (scheduled slice) · `HAND`
(documented pattern, deliberately hand-written) · `LATER` (compatible,
unscheduled) · `NO` (non-goal, see [D27](decisions.md)).

---

## The ladder: where this tool sits

The Go data-access ecosystem is a chain of supersets. Each layer adds one
capability over the previous — and each maps to one piece of Not an ORM:

| Layer | What it adds | Ecosystem | Not an ORM's version |
|---|---|---|---|
| driver | connections, `Exec`/`Query`, `Rows` | `database/sql` + a SQLite driver | the same base; `rt.Open`, `rt.DBTX` (driver stays test-only, D03/D25) |
| scan ergonomics | rows → structs, named params | `sqlx` | generated per-query scan code — zero reflection, checked at compile time |
| SQL building | composing SQL in Go at runtime | `squirrel` | v2 typed option values — **only** for filter/order/limit (D28–D32) |
| typed queries from SQL | write SQL, generate typed Go | `sqlc` | v1 `Select`/`View` blocks in the schema file, gen-time prepare validation (D06) |
| full ORM | CRUD, associations, migrations, callbacks | Rails AR, GORM, Ent | v0 CRUD + v3 loaders + v4 migrations; callbacks/dirty tracking/lazy loading: never (D27) |

I am not inventing a new problem — I am answering each rung with
codegen instead of runtime machinery, from one authored DBML file.

### Why there is no "any query" builder

An ORM must let you build any query at runtime because runtime is its
only chance to construct SQL. I have a build step, so I split "any
query" by **what actually varies when the program runs** — and the split
is exhaustive (D32, the shape rule):

| What varies at runtime | Answer | Tier | Slice |
|---|---|---|---|
| nothing — fixed shapes | generated CRUD (`UserGet`, …) | [CRUD](#p2--default-crud) | v0 `DONE` |
| values only | real SQL in a declared `Select` block, `:named` params, typed function generated, prepare-validated | [SEL](#p3--custom-queries-static-shapes-select--view) | v1 |
| filter / order / limit | typed predicate **values** (`UserCols.Email.Eq(x)`) over generated column handles | [DYN](#p4--dynamic-queries-runtime-composition) | v2 `DONE` |
| the shape itself | quarantined report engine over the exported schema catalog | [RPT](#p6--ad-hoc-user-defined-queries) | LATER |

The long tail an ORM covers with its query DSL lands in tier two, where
the query language is SQL itself — already the best language for it —
and SQLite proves every statement at generation time (D02). The only
builder-shaped surface that survives is tier three, and it is inert data,
not string assembly.

---

## P1 — One schema, everything derived

**Problem.** The same truth lives in four places (diagram, DDL, code,
live DB) and drifts ([the-model-layer.md](the-model-layer.md)). **Answer.**
DBML is canonical; everything else is generated from `check.Info`.

| ID | Feature | Slice | Status |
|---|---|---|---|
| SRC-1 | Normative DBML spec (EBNF) + conformance corpus + upstream cross-check | — | `DONE` ([SPEC](../SPEC.md), D05) |
| SRC-2 | Full front end: scanner, parser, `check.Info`, multi-error recovery | — | `DONE` |
| SRC-3 | `vet`: lint for legal-but-suspicious DBML, docs-enforced rules | — | `DONE` ([RULES.md](../vet/RULES.md), D26) |
| SRC-4 | `dbml gen go` → models (`dbml_models.go`): structs, enums, notes → doc comments, `Null[T]`, singular names + `[model:]` | v0 | `DONE` (D09–D13) |
| SRC-5 | `dbml gen sqlite` → DDL + seed `INSERT`s from `records` | v0 | `DONE` (D12, D38) |
| SRC-6 | Extensions stay a projectable superset (core diagrams still work) | all | ongoing (D05) |

## P2 — Default CRUD

**Problem.** 90% of data access is the same eight statements per table;
hand-writing them is boilerplate, an ORM hides them behind reflection.
**Answer.** Generate them as plain, readable Go (v0, shipped).

| ID | Feature | Slice | Status |
|---|---|---|---|
| CRUD-1 | `UserGet` by pk; composite pks → one positional arg per key column | v0 | `DONE` (D15, D17) |
| CRUD-2 | `UserGetMany(ids)` — one `IN` query; missing keys are absence, not error | v0 | `DONE` (D36) |
| CRUD-3 | `UserGetByEmail` per unique column | v0 | `DONE` |
| CRUD-4 | `UserList`, `UserCount` | v0 | `DONE` |
| CRUD-5 | `UserCreate(params)` — excludes auto-increment + defaulted columns, `RETURNING` (D16) | v0 | `DONE` |
| CRUD-6 | `UserUpdate(id, params)` full-row, `RETURNING`; misses → `rt.ErrNotFound` | v0 | `DONE` (D36) |
| CRUD-7 | `UserDelete(id)` — `RowsAffected`-checked miss | v0 | `DONE` (D36) |
| CRUD-8 | No-pk tables: `List`/`Create`/`Count` only | v0 | `DONE` (D17) |
| CRUD-9 | Named `:param` placeholders end to end; identity positional, data in params structs | v0 | `DONE` (D15) |

## P3 — Custom queries: static shapes (`Select` / `View`)

**Problem.** Real applications need joins, aggregates, projections, CTEs,
window functions — "almost any query". ORMs answer with a runtime DSL;
sqlc answers with `.sql` files divorced from the schema. **Answer.**
Declared query blocks *inside the schema file*: structure (select list,
tables, joins, declared params) parsed by my front end and resolved
against `check.Info`; expression bodies opaque and proven by gen-time
prepare against the generated DDL (D06). Each block mints its own result
struct (the shape rule, D32) and a typed function.

Requirements (syntax to be specified in SPEC.md when built; sketches
are illustrative):

| ID | Feature | Slice | Status |
|---|---|---|---|
| SEL-1 | `Select name (param type, …) { … }` block: declared params (D07), select list, from/joins, opaque `where`/`having`/`group by`/`order by` bodies | v1 | planned |
| SEL-2 | Result struct minted per block (`OrderReport`), fields typed from `check.Info` + SQLite's column metadata | v1 | planned |
| SEL-3 | Generated function `OrderReportQuery(ctx, params) ([]OrderReport, error)`; single-row variants | v1 | planned |
| SEL-4 | Joins inferred from declared refs where unique; explicit `on:` override (D08) | v1 | planned |
| SEL-5 | Gen-time prepare validation of every block against the generated DDL; column-list agreement checked (D02, D24) | v1 | planned |
| SEL-6 | `View` blocks: DDL `CREATE VIEW` + read-only struct + accessors; declared columns verified against the SQL (D24) | v1 | planned |
| SEL-7 | Narrow projections (list views without full hydration) — a `Select` block per shape is *the* mechanism | v1 | planned |
| SEL-8 | Raw SQL strings in `.go` files: outside the safety net, escape hatch documented as "promote it to a Select block" | — | `HAND` (D18) |

## P4 — Dynamic queries: runtime composition

**Problem.** Filters chosen by user input at runtime (search screens,
admin lists) can't be enumerated at build time. **Answer.** Functional
options as inert data (D28): predicates are values over generated typed
column handles, one interpreter renders SQL + positional args in
lockstep (D42), statements cached by text (D31).

| ID | Feature | Slice | Status |
|---|---|---|---|
| DYN-1 | Runtime core: `Pred[M]`, `And/Or/Not`, `Raw` escape hatch, deterministic interpreter, positional `?` binding | v2 | `DONE` (D28, D42) |
| DYN-2 | Generated handles, one set per model: `UserCols.Email` (`rt.Column[User, string]`; nullable columns `rt.NullColumn` with `IsNull`/`SetNull`); operators `Eq/Ne/In/Gt/Lt/Like/IsNull/Desc/EqCol…` once in the runtime | v2 | `DONE` (D29) |
| DYN-3 | Generated wrappers for value-less options: `UserLimit(n)`, `UserOffset(n)`, `UserOrderBy(…)`, `UserAfter(…)`, `UserDistinct()`, `UserSet(…)` | v2 | `DONE` (D30) |
| DYN-4 | Verbs sharing predicates: `UserQuery`, `UserCount`, `UserExists`, `UserDeleteWhere`, `UserUpdateWhere` + typed `Assign[M]` setters (partial updates land here) | v2 | `DONE` (D32) |
| DYN-5 | Keyset pagination first-class (`UserAfter(key)`, lexicographic expansion, mixed directions); OFFSET degrades linearly | v2 | `DONE` (D34) |
| DYN-6 | Prepared-statement cache keyed by rendered SQL (LRU), wired via `Queries.WithCache` | v2 | `DONE` (D31) |
| DYN-7 | vet rule `dynname`: generated package-scope name collisions (handle sets, option wrappers vs models/enums) | v2 | `DONE` (D29) |

## P5 — Associations

**Problem.** A `Post` needs its `Comments`; lazy loading and struct
fields lie about what's loaded. **Answer.** Explicit per-ref loaders,
generated from declared refs (D19); loops of queries are officially fine
in embedded SQLite, batching is for atomicity and large N (D35).

| ID | Feature | Slice | Status |
|---|---|---|---|
| ASSOC-1 | Single loaders: `PostAuthorGet(ctx, post)` (FK side), `UserPostsList(ctx, user)` (one-to-many) | v3 | planned |
| ASSOC-2 | Batched loaders: `PostCommentsLoad(ctx, posts) (map[int64][]Comment, error)` — one `IN`, grouped in memory | v3 | planned (D19, D35) |
| ASSOC-3 | Many-to-many through junction tables: `UserRolesLoad` walks both refs | v3 | planned |
| ASSOC-4 | No association fields on row structs, ever — not-loaded vs empty must stay distinguishable | — | `NO` by construction (D14, D19) |
| ASSOC-5 | Polymorphic / STI patterns | — | `HAND`/`NO` (D27; matrix §3) |

## P6 — Ad-hoc (user-defined) queries

**Problem.** End users composing their own reports at runtime — the one
case where the shape is unknowable at build time. **Answer.** Quarantine
it (D33): one package, validated against a generated catalog, values only
ever behind placeholders, output `[]map[string]any` whose sole consumer
is serialization.

| ID | Feature | Slice | Status |
|---|---|---|---|
| RPT-1 | Schema catalog export: tables, columns, types, refs as runtime data | LATER | planned (D33) |
| RPT-2 | Report engine: request validation, joins only along declared refs, placeholder-only values | LATER | planned (D33) |

## P7 — Schema evolution (migrations)

**Problem.** "Good luck" — the hardest sub-layer
([the-model-layer.md](the-model-layer.md)). **Answer.** Declarative:
diff current DBML against the last-migrated snapshot; the ledger is
generated, ordered, hash-pinned; I own SQLite's twelve-step rebuild.

| ID | Feature | Slice | Status |
|---|---|---|---|
| MIG-1 | Semantic diff: DBML vs snapshot (never SQL parsing); adds/drops/type changes | v4 | planned (D20) |
| MIG-2 | Renames via `[was: 'old']` provenance hints (extension) | v4 | planned (D20) |
| MIG-3 | Generated migration ledger + `schema_migrations` table; each migration embeds the schema content hash → drift is machine-detectable | v4 | planned (D21) |
| MIG-4 | Owned twelve-step rebuild for changes ALTER TABLE can't do (pragma bracket, `foreign_key_check` after) | v4 | planned (D22) |
| MIG-5 | Apply via CLI or embedded runner (`app migrate up`, `embed.FS`); **never** on app start | v4 | planned (D04, D23) |
| MIG-6 | Irreversibility honesty: no fake `down` for destructive changes | v4 | planned |
| MIG-7 | Data migrations (backfills) | — | `HAND`, documented pattern; kept out of schema migrations |

## P8 — Data integrity (constraints & validation)

**Problem.** Where validation lives (struct vs controller vs DB) and how
much the database enforces. **Answer.** Structure is the database's job
(generated DDL), business invariants are typed Go on the models (D18).

| ID | Feature | Slice | Status |
|---|---|---|---|
| INT-1 | `NOT NULL`, `UNIQUE`, FKs with `ON DELETE/UPDATE`, composite PKs, 1:1 uniqueness, `CHECK` expressions | v0 | `DONE` (DDL) |
| INT-2 | Enums: string-backed types + constants; DB `CHECK` opt-in only (twelve-step-dance avoidance) | v0 | `DONE` (D11, D12) |
| INT-3 | Int-backed enums via `[repr: int]` (extension) — full String/Valuer/Scanner/JSON suite, int never escapes the process | LATER | planned (D11) |
| INT-4 | Business validation: hand-written methods in sibling files of the generated package | — | `HAND` (D18); document the pattern |
| INT-5 | Optimistic locking via `[lock_version]` (extension): `Update` gains version check + bump | LATER | planned (matrix §2) |

## P9 — Transactions & concurrency

| ID | Feature | Slice | Status |
|---|---|---|---|
| TX-1 | `Queries` over `DBTX`: same generated methods inside and outside a tx; `WithTx`, `q.Tx(ctx, fn)` | v0 | `DONE` |
| TX-2 | `rt.Tx`: begin/commit, rollback on error or panic | v0 | `DONE` |
| TX-3 | Single-writer discipline: one-connection pool, WAL, `busy_timeout` via `rt.Open` | v0 | `DONE` (D37) |
| TX-4 | Batched loaders as the *atomicity* answer to loops of reads | v3 | planned (D35) |
| TX-5 | Nested transactions / savepoints | — | `LATER`; error today, honest about it |

## P10 — Runtime & deployment

| ID | Feature | Slice | Status |
|---|---|---|---|
| RUN-1 | `rt` package: `Null[T]` (value-or-null JSON), `DBTX`, `ErrNotFound` (= `sql.ErrNoRows`), `Open`, `StmtCache` — stdlib-only, registers no driver | v0 | `DONE` (D03, D13, D36–D37) |
| RUN-2 | Everything library-callable; CLI is a thin wrapper | v0 | `DONE` (D04) |
| RUN-3 | User binaries embed subcommands + migrations: one self-contained deployable | v4 | planned (D04) |
| RUN-4 | Generated code header + clobber protection in the CLI | v0 | `DONE` |

## P11 — Guarantees (the correctness harness)

The features above are only as good as their proofs. Standing invariants,
all enforced by `go test ./...`:

| ID | Guarantee | Status |
|---|---|---|
| PROOF-1 | Spec ⇄ conformance corpus ⇄ upstream `@dbml/parse` (0 disagreements) | `DONE` |
| PROOF-2 | Generated Go: gofmt-stable goldens + compiled by the real toolchain | `DONE` |
| PROOF-3 | Generated SQL: executes on real SQLite (`foreign_key_check` clean) | `DONE` |
| PROOF-4 | Every generated statement prepares against the generated DDL (cross-generator coherence) | `DONE` |
| PROOF-5 | `itest`: full CRUD round trips through a real driver; generated files drift-tested | `DONE` (D25) |
| PROOF-6 | vet rules ⇄ RULES.md ⇄ testdata consistency test | `DONE` (D26) |
| PROOF-7 | v1 onward: every `Select`/`View` block prepare-validated at gen time; declared columns verified | with v1 |

## Non-goals

Permanent, with reasons — see **D27**: identity map / unit of work, lazy
loading, active-record `Save()`, callbacks, dirty tracking, default
scopes, single-table inheritance, reflection in generated paths, a
Turing-complete query DSL in Go. Every one is either a lie waiting to
happen (lazy loading, callbacks) or a worse language for a job SQL
already does.

---

## Build order

v1 `SEL-*` → v2 `DYN-*` (+ INT-3/INT-5 opportunistically) → v3 `ASSOC-*`
→ v4 `MIG-*` + RUN-3 → RPT-* when a consumer exists. Each feature lands
with its PROOF-row test before it's called done.
