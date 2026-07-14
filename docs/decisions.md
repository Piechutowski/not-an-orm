# Decisions

The locked design decisions of Not an ORM, numbered for reference. Each was
argued out in working sessions; this file is the cross-session memory. A
decision is changed by editing this file, not by drifting away from it.

## Scope and identity

- **D01 — This repository is the project, named `not-an-orm`.** DBML spec +
  front end + linter + generators + (coming) query/migration layers, one
  module (`github.com/Piechutowski/not-an-orm`). The front page (README.md)
  presents the tool; the DBML spec lives in SPEC.md. (Renamed from the
  pre-pivot "dbml-docs" on 2026-07-09.)
- **D02 — SQLite-first, all-in.** Every simplification bought by rejecting
  other DBMSes is a good trade. SQLite is also *tooling*: at generation time
  an in-memory SQLite is my SQL parser and type checker (prepare-validate
  everything I emit or accept).
- **D03 — One coherent tool, zero runtime dependencies.** Generated code
  imports stdlib and my own small runtime package only. No sqlc, squirrel,
  sqlx, Atlas. Proven *shapes* may be borrowed (e.g. Queries/DBTX); code is
  mine.
- **D04 — Library-first, CLI-thin.** Everything the CLI does is importable;
  user binaries embed subcommands (`app migrate up`) and migrations
  (`embed.FS`). One self-contained deployable binary.
- **D39 — `gen` defaults to the working directory.** The common case is
  authoring `schema.dbml` in a package directory and generating alongside
  it, so `dbml gen go` / `dbml gen sqlite` need no arguments: input
  defaults to `./schema.dbml`, output to `.`, Go package to `main`. Short
  flags `-i/--input`, `-o/--out`, `-p/--package` override. Consistent with
  D16's "never guess": the input default is the fixed `schema.dbml`
  convention, not a scan-and-pick among the directory's `.dbml` files, and
  supplying both `-i` and a positional path is an error, not a silent
  choice. Shell completion is enabled on the CLI (`dbml completion <shell>`).
  `gen go -m/--models-only` emits only `dbml_models.go` (the struct/enum
  row types), so those types can be shared across processes — e.g. gob
  between a server and a GUI client — without dragging in the CRUD layer;
  the models file only ever depends on the stdlib and `rt`.
- **D40 — Editor tooling lives in this repository** (2026-07-12, extends
  D01): the tree-sitter grammar (`tree-sitter-edbml/`), the Zed extension
  (`zed-extension/`) and the language server (`lsp/`, `cmd/edbml-ls`) are
  part of the project, not a sibling repo. Rationale: the LSP is a second
  consumer of the same front end the CLI wraps (D04 in action), and every
  language extension (Select, View, `[was:]`, `[repr:]`) must move the
  parser, checker, vet, conformance corpus, grammar and editor features in
  lockstep — one repository makes that a single commit. The interim
  `edbml` repo (formerly `edbml-zed`), which vendored a copy of the front
  end, is superseded and archived; its development history remains there.
  Editor architecture and patterns: `docs/editor-architecture.md`.

## Language and front end

- **D05 — The spec is normative; the conformance corpus + upstream
  cross-check pin it.** Extensions (Select, View, Trigger, `[was:]`,
  `[model:]`, `[repr:]`) are a superset; core stays projectable to vanilla
  DBML for diagramming.
- **D06 — Structure mine, expressions SQLite's.** Extended-DBML queries parse
  select lists, tables, joins and declared params with my front end
  (resolved against `check.Info`); WHERE/HAVING bodies and other expressions
  stay opaque and are validated by gen-time prepare. I never build an SQL
  expression grammar.
- **D07 — Query params are declared, not inferred** — `Select foo (rok int)`.
  Explicit signatures beat inference.
- **D08 — Joins are inferred from declared refs** where a unique ref exists
  between the joined tables; explicit `on:` overrides.

## Generated Go

- **D09 — Naming is subject-first, verb-last**: `UserGet`, `UserList`,
  `UserCreateParams`, `PostCommentsLoad`, `OrderStatusPending`. Autocomplete
  and search group by model, then filter by verb.
- **D10 — Models are singular** (`User` from `users`) via a small
  deterministic inflector; `[model: 'Person']` overrides; vet warns when the
  inflector guessed. Go names use the initialisms convention (`UserID`).
- **D11 — Enums are string-backed by default** (`type OrderStatus string`,
  constants `OrderStatusPending`), stored as TEXT. Int-backed
  (`[repr: int]`) is opt-in and generates the full conversion suite
  (String, Valuer, Scanner, JSON) — the int must never escape the process.
- **D12 — No enum CHECK constraints by default** (adding a value must not
  force SQLite's twelve-step rebuild); opt-in for belt-and-suspenders.
  Structural constraints (NOT NULL, FK, UNIQUE) always emitted.
- **D13 — Nullable columns are `Null[T]`** — my runtime's generic value type
  (`struct { V T; Valid bool }`) with Scan/Value and JSON that marshals as
  the value or `null`. Value semantics, no heap, no nil-deref. (Supersedes
  the pointer representation in early `gen go` output.)
- **D14 — Values, not objects.** Each scan is a fresh struct; no identity
  map, no lazy loading (impossible by construction), no `Save()` methods,
  no association fields on row structs.
- **D15 — Named placeholders; identity args positional, data args in a
  struct.** Generated SQL uses SQLite's named parameters (`:email`), bound
  via `sql.Named` inside the generated code — the placeholder *is* the
  column name, so ordering cannot exist as a concept, and the generated SQL
  is self-documenting. At the Go boundary: **identity** arguments (the
  pk columns of Get/Delete) are positional — `UserGet(ctx, id)`,
  `UserRoleGet(ctx, userID, roleID)`; **data** arguments (Create/Update,
  declared query params) arrive in a generated params struct whose fields
  map 1:1 to the named placeholders. Explicit end to end: DBML column →
  struct field → `:param`.
- **D16 — CreateParams exclude auto-increment PKs and defaulted columns**;
  the INSERT omits them and `RETURNING` (SQLite ≥ 3.35) brings back the row
  the database actually wrote. Overriding a default = dynamic/declared query.
- **D17 — Tables without a primary key** get only `List/Create/Count`.
  Composite PKs get multi-argument `Get/Update/Delete`.
- **D18 — Hand-written code extends generated types in sibling files** of the
  same package (methods, validation, invariants). Generated business logic
  is a non-goal. Raw SQL strings in `.go` files are outside the safety net —
  the supported escape hatch is a `Select` block in the schema file, which
  regeneration re-validates.

## Query layer (from the query-builder design session, 2026-07-08)

- **D28 — The dynamic query core is functional options as inert data.**
  Options and predicates are plain values (a small expression tree), never
  closures and never a mutable builder: one terminal call
  (`UserQuery(ctx, opts...)`) walks the tree once and renders SQL + named
  args in lockstep. Values compose (`And`/`Or`/`Not`), store in variables
  (the scope replacement), append conditionally, and can be inspected,
  cached and tested — a closure can only be executed. A fluent facade could
  be layered on later; the value core is the irreversible part.
- **D29 — Typed handles via a generic runtime column type.** The runtime
  defines `Column[M, T]` once (phantom model type `M`, value type `T`);
  the generator emits one-line vars: `UserEmail = rt.Column[User, string]{...}`.
  Operators (`Eq/Ne/In/Gt/Like/Desc/EqCol/...`) live once in the runtime,
  specialized per column kind. Phantom `M` makes cross-model mixups a
  compile error (`Pred[Post]` cannot enter a `User` query). Vet must flag
  flat-name collisions (two table-column pairs producing one Go name).
- **D30 — The generics wall is papered by codegen.** Go cannot infer `M`
  for value-less options (`Limit`), so the generator emits per-model
  wrappers: `UserLimit(n)`, `UserOffset(n)`. Subject-first everywhere.
- **D31 — Prepared-statement cache keyed by rendered SQL text.** The
  interpreter is deterministic, so identical tree shapes render identical
  SQL; the runtime caches `*sql.Stmt` per text (LRU). Static SQL (CRUD,
  declared Selects) is prepare-validated at generation time; dynamic shapes
  are validated by construction plus first prepare.
- **D32 — The shape rule.** Anything that *changes what a row is* (joins,
  computed columns, GROUP BY/HAVING) happens at generation time, where
  structs can be minted (declared `Select`/`View` blocks). Anything that
  only *filters/orders/limits an existing shape* is runtime-builder
  material (including same-model `EqCol` column comparisons). Predicates
  are shared across verbs: `UserQuery`, `UserCount`, `UserDeleteWhere`,
  `UserUpdateWhere` (+ typed `Assign[M]` setters).
- **D33 — Truly user-defined queries live in a quarantined report engine.**
  The generator also exports the schema as a runtime *catalog* (tables,
  columns, types, refs); the engine validates request-data against it,
  emits SQL with values only ever behind placeholders, joins only along
  declared refs, and returns `[]map[string]any` whose sole consumer is
  serialization. The untyped region is one package.
- **D34 — Keyset pagination is a first-class runtime feature** (scheduled
  with v2, not "later"): `OFFSET` degrades linearly with depth. SQLite is
  not a small-apps database — the solo dev with 100M rows is the thesis —
  so scale features are in scope by default.
- **D35 — N+1 is reframed, batched loaders stay.** In embedded SQLite a
  query is a function call, not a network round trip — the loop-of-queries
  pattern is officially acceptable (SQLite's own docs, Fossil's practice),
  so N+1 is not the marketing headline. Batched loaders remain the default
  because one `IN` query is *atomic* (a loop of queries in autocommit can
  interleave with writes — the single-writer property makes consistency,
  not speed, the argument) and still wins at large N. Statement overhead is
  microseconds, not milliseconds; I say so honestly.

## Runtime and CRUD surface (v0 build, 2026-07-08)

- **D36 — CRUD miss and return semantics.** `Get`/`GetBy*` return
  `rt.ErrNotFound`, which *is* `sql.ErrNoRows` (existing `errors.Is`
  checks keep working). `Update` rewrites every non-key column and
  returns the resulting row via `RETURNING` (`ErrNotFound` on a miss);
  partial updates are v2's `UserUpdateWhere` with typed setters. `Delete`
  checks `RowsAffected` and reports a miss as `ErrNotFound`. `GetMany`
  treats missing keys as absence, not error, and exists only for
  single-column keys.
- **D37 — `rt.Open` pins the pool to one connection.** SQLite pragmas are
  per-connection, so `Open(driver, dsn)` sets `MaxOpenConns(1)` and then
  applies WAL, `busy_timeout=5000` and `foreign_keys=ON` — the pragmas
  provably hold for every statement, and in-process busy contention
  disappears. SQLite has a single writer anyway; multi-connection read
  scaling can be revisited without breaking the API. The runtime registers
  no driver (D03): the application imports one and passes its name.
- **D38 — Temporal declared types are the driver contract.** `gen sqlite`
  declares `TIMESTAMP`/`DATETIME`/`DATE` (NUMERIC affinity, stored as
  ISO-8601 text) because SQLite drivers key their `time.Time` parsing on
  the declared type. Time-of-day types (`time`, `timetz`) map to Go
  `string`: SQLite has no time type and drivers no scan rule for one.

## Associations

- **D19 — Per-ref loaders, not struct fields**: single
  (`PostAuthorGet(ctx, post)`) and batched
  (`PostCommentsLoad(ctx, posts) map[int64][]Comment`) — one `IN (…)` query,
  grouped in memory. A `Comments` field on `Post` cannot distinguish
  not-loaded from empty; that ambiguity is banned.

## Migrations

- **D20 — Declarative, diffed against the last-migrated DBML snapshot**
  (semantic-model diff, never SQL parsing). Drops = deletion from the file;
  renames = `[was: 'old']` provenance hints; versioning lives only in the
  generated ordered ledger + `schema_migrations` table.
- **D21 — Each migration embeds the schema content hash** it targets, so
  "schema changed but no migration cut" is machine-detectable drift.
- **D22 — I own the SQLite twelve-step rebuild** (I already generate full
  CREATE TABLE; a rebuild is create-new + INSERT…SELECT + drop + rename in
  the pragma bracket). No Atlas.
- **D23 — Migrations never run on app startup.** Applied by CLI or an
  embedded runner invocation, explicitly.
- **D24 — Views declare their columns and carry their SQL**; the prepare
  check verifies the two agree. Views generate read-only structs +
  accessors. Triggers: deferred (stateless drop-and-recreate when added).

## Testing

- **D25 — Integration over mocks for the data layer.** Generated code is
  exercised against a real SQLite (test-only driver dependency); the
  language front end keeps its exhaustive unit corpus. Golden files remain
  for a different job: surfacing unintended generator-output changes in
  review. Every generated SQL artifact must execute against a real engine
  in tests.
- **D26 — Docs are enforced where possible** (vet RULES ↔ registry ↔
  testdata consistency test); design docs live in `docs/` as the
  cross-session memory: `the-model-layer.md`, `not-an-orm.md`,
  `orm-capability-matrix.md`, `features.md` (the per-problem feature spec
  and build plan — feature statuses update in the commit that lands them),
  this file.

## Non-goals (permanent unless revisited here)

- **D27 — Never:** identity map / unit of work, lazy loading, active-record
  `Save()`, reflection in generated paths, callbacks, dirty tracking,
  default scopes, single-table inheritance / delegated types, a
  Turing-complete query DSL.
