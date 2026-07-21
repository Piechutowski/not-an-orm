# ORM Capability Matrix

Every capability of a full-grown ORM — inventoried from the Rails Active
Record guides (reference copies in [`reference/rails/`](reference/rails/)) —
with Not an ORM's verdict for each. This is the completeness check for the
roadmap: a feature is in scope, out of scope, or consciously deferred, and
nothing is unaccounted for.

**Verdicts**

| Tag | Meaning |
|-----|---------|
| `GEN(vN)` | generated Go code, scheduled for roadmap slice vN |
| `DBML(vN)` | expressed in (extended) DBML, code generated from it |
| `RT(vN)` | lives in the small hand-written runtime package |
| `DDL` | already handled by the schema generator (`gen sqlite`) |
| `DONE` | already built |
| `HAND` | deliberately hand-written by the user, with a documented pattern |
| `LATER` | plausible and compatible, not scheduled |
| `NO` | non-goal, with the reason |

Roadmap slices: **v0** CRUD + runtime *(shipped 2026-07-08)* · **v1** declared queries/views ·
**v2** dynamic typed queries · **v3** association loaders · **v4** migrations.

---

## 1. Basics & conventions (`active_record_basics.md`)

| Capability | Rails | Verdict | How / why |
|---|---|---|---|
| Model ↔ table naming convention | `User` ↔ `users` via inflection | `GEN(v0)` `DONE` | singular model via deterministic inflector, `[model:]` override (D10) |
| Schema conventions (`id`, FK naming) | convention | `DBML` | explicit in the schema; conventions are *suggested* by vet, never assumed |
| Magic timestamp columns (`created_at`/`updated_at` auto-managed) | automatic | `NO` | hidden writes; declare the columns and a DB default / trigger instead. A TablePartial replaces the convention |
| `new` / `create` | `User.create(...)` | `GEN(v0)` `DONE` | `UserCreate(ctx, UserCreateParams)` → row via `RETURNING` (D16) |
| Read: find all / by pk | `User.all`, `User.find(1)` | `GEN(v0)` `DONE` | `UserList(ctx)`, `UserGet(ctx, id)` |
| Update | `user.update(...)` | `GEN(v0)` `DONE` | `UserUpdate(ctx, id, UserUpdateParams)` — full-row by pk; partial updates are v2 |
| Delete / destroy | `user.destroy` | `GEN(v0)` `DONE` | `UserDelete(ctx, id)`; `destroy_all` → v2 `UserDeleteWhere` |
| Namespaced models | Ruby modules | `GEN` | DBML schemas prefix the type (`core.users` → `CoreUser`) |
| `save` on the object | active record pattern | `NO` | objects don't persist themselves (D14) |

## 2. Querying (`active_record_querying.md`)

### Retrieving

| Capability | Rails | Verdict | How / why |
|---|---|---|---|
| `find` (single pk) | `find(1)` | `GEN(v0)` `DONE` | `UserGet` |
| `find` (multiple pks) | `find([1,2])` | `GEN(v0)` `DONE` | `UserGetMany(ctx, ids) ([]User, error)` — one `IN` query |
| `find_by` unique column | `find_by(email:)` | `GEN(v0)` `DONE` | `UserGetByEmail(ctx, email)` generated **per unique column** — typed, zero magic |
| `take`/`first`/`last` | relation methods | `RT(v2)` `DONE` | `UserQuery(... OrderBy, Limit(1))`; convenience `UserFirst` `LATER` |
| Dynamic finders (`find_by_email` metaprogrammed) | method_missing | `GEN(v0)` `DONE` | same as `find_by` row above — generated statically instead of conjured |
| Method chaining / lazy relations | relation composes lazily | `RT(v2)` `DONE` | options-based values, assembled explicitly at the call site; nothing lazy |
| `find_or_create_by` / `create_or_find_by` | upsert-ish | `GEN(v1)` | `UserUpsert` on SQLite's native `INSERT … ON CONFLICT` — race-free, unlike Rails' select-then-insert |
| `find_or_initialize_by` | in-memory | `HAND` | trivial two-liner over `UserGetByX` + struct literal |
| `exists?` / `any?` / `many?` | relation predicates | `RT(v2)` `DONE` | terminals `UserExists`, `UserCount` |
| Batches (`find_each`, `in_batches`) | cursor batching | `RT(v2)` `DONE` | keyset pagination (`UserAfter(last-seen key)`), first-class: OFFSET degrades linearly and SQLite is not a small-apps database (D34) |

### Conditions

| Capability | Rails | Verdict | How / why |
|---|---|---|---|
| Hash conditions (`where(name:)`) | typed-ish | `RT(v2)` `DONE` | `UserCols.Name.Eq(v)` — generated typed handles over generic `Column[M, T]` (D29) |
| Array/placeholder conditions | `where("x > ?", v)` | `RT(v2)` `DONE` | `Raw("x > ?", v)` escape hatch — explicit, marked, last resort |
| Pure string conditions | discouraged | `NO` | injection surface with no type info; use `Raw` or a `Select` block |
| NOT / OR / AND | `where.not`, `.or` | `RT(v2)` `DONE` | `Not(...)`, `Or(...)`, `And(...)` combinators |
| Range / IN / subset ops | hash sugar | `RT(v2)` `DONE` | `In/NotIn`, `Gt/Ge/Lt/Le`, `Like/NotLike`, `IsNull/IsNotNull`, column-to-column `EqCol...` per typed handle (`Between` = `And(Ge, Le)`) |
| `where.associated` / `where.missing` | EXISTS sugar | `LATER` | `EXISTS` sugar over declared refs; nice-to-have |

### Shaping

| Capability | Rails | Verdict | How / why |
|---|---|---|---|
| Ordering | `order(:created_at)` | `RT(v2)` `DONE` | `UserOrderBy(UserCols.CreatedAt.Desc())` |
| Limit / offset | `limit/offset` | `RT(v2)` `DONE` | generated `UserLimit(n)`/`UserOffset(n)` (D30); keyset `UserAfter` shipped with them (D34) |
| Select specific fields (projection) | `select(:id, :name)` | `DBML(v1)` | typed projection needs a result type → a named `Select` element generates one. Dynamic projection is `NO` — a half-filled `User` lies about its zero values |
| Distinct | `distinct` | `RT(v2)` `DONE` | `UserDistinct()` option |
| Group / Having / aggregates of groups | `group/having` | `DBML(v1)` | aggregate shapes belong in declared queries where the result struct is generated |
| Calculations (`count/sum/min/max/average`) | relation terminals | `GEN(v0)` `DONE` count (predicate-aware since v2); `RT(v2)` rest | `UserCount(preds...)` now; typed `sum/min/max/avg` terminals later |
| `explain` | debugging | `LATER` | runtime debug hook logging SQL + `EXPLAIN QUERY PLAN` |
| Overriding (`unscope/except/rewhere/...`) | un-doing scopes | `NO` | exists only because relations mutate implicitly; my builder composes explicitly, there is nothing to un-do |
| `none` (null relation) | polymorphic no-op | `NO` | return an empty slice |
| `readonly` | runtime guard | `NO` | rows are value copies; there is nothing to protect |

### Scopes

| Capability | Rails | Verdict | How / why |
|---|---|---|---|
| Named scopes | `scope :active, ->{...}` | `DBML(v1)` + `HAND` | reusable queries = named `Select` elements; reusable *predicates* = plain Go values (`var activeUsers = UserCols.Status.Eq(UserStatusActive)`) composed by hand (D28) |
| Scopes with arguments | lambdas | `DBML(v1)` | declared params: `Select active_users (since timestamp)` |
| Default scope | implicit filter on every query | `NO` | hidden query mutation — the haunted house. Explicit or nothing (D27) |
| Merging of scopes | `merge` | `RT(v2)` `DONE` | predicate values compose with `And`/`Or` |

### Locking & concurrency

| Capability | Rails | Verdict | How / why |
|---|---|---|---|
| Optimistic locking | `lock_version` column | `LATER` | opt-in `[lock_version]` setting: `UserUpdate` gains a version check + bump; clean fit, not v0 |
| Pessimistic locking | `SELECT … FOR UPDATE` | `NO` | SQLite has no row locks; the analogue is `BEGIN IMMEDIATE` — a runtime `Tx` option, `LATER` |
| Transactions | `ActiveRecord::Base.transaction` | `GEN(v0)` `DONE`/`RT(v0)` `DONE` | `Queries.WithTx` + a `Tx(ctx, db, func(q *Queries) error)` helper |

### Joins & eager loading

| Capability | Rails | Verdict | How / why |
|---|---|---|---|
| `joins` / `left_outer_joins` | symbol-based | `DBML(v1)` | `Select` elements with ref-inferred joins (D08) |
| Association loading (`includes/preload/eager_load`) | 3 strategies | `GEN(v3)` | per-ref batched loaders (D19). Note: in embedded SQLite N+1 is a function-call loop, not a network disaster — the loaders' real wins are atomicity of one query and ergonomics (D35) |
| `strict_loading` | raises on lazy access | `NO` | there is no lazy access to guard against — N+1-by-accident is impossible by construction |

## 3. Associations (`association_basics.md`)

| Capability | Rails | Verdict | How / why |
|---|---|---|---|
| `belongs_to` / `has_one` | macro + methods | `GEN(v3)` | from each ref: `PostAuthorGet(ctx, post)`, batched `PostAuthorLoad(ctx, posts)` |
| `has_many` | macro | `GEN(v3)` | `UserPostsList(ctx, user)`, batched `UserPostsLoad(ctx, users) map[...]` |
| `has_many :through` | join through | `GEN(v3)` | junction detected (2 FKs forming the pk) → `UserRolesList` through it, one joined query |
| `has_one :through` | join through | `GEN(v3)` | same mechanism, single row |
| `has_and_belongs_to_many` | headless junction | `NO` | the junction table is always explicit in DBML; through-loaders cover it |
| Polymorphic associations | `commentable` | `LATER` | needs an extended-DBML declaration (`type`+`id` pair); loaders per target follow mechanically |
| Self joins | `manager_id` | `GEN(v3)` | falls out of ref loaders naturally |
| Composite-pk associations | query_constraints | `GEN(v3)` | row-value `IN ((a,b),…)` — SQLite supports it |
| `dependent:` destroy/delete | app-level cascade | `DDL` | `ref [delete: cascade]` → real `ON DELETE CASCADE`, already generated. App-level cascade callbacks: `NO` |
| Counter cache | cached child count | `NO` | `PostCommentsCount` is one indexed query; a DB trigger if it ever matters |
| Inverse-of / association caching | object-graph identity | `NO` | no object graph (D14) |
| Association callbacks / extensions | hooks on collections | `NO` | callbacks are a non-goal (D27) |
| Single-table inheritance | `type` column magic | `NO` | Go has no inheritance; model explicitly |
| Delegated types | STI successor | `NO` | same; the polymorphic pattern (`LATER`) covers the storage shape |

## 4. Validations (`active_record_validations.md`)

The stance (from [`the-model-layer.md`](the-model-layer.md)): shape → type
system, invariants → hand-written methods, last resort → DB constraints.
I generate the first and third; the middle is yours.

| Capability | Rails | Verdict | How / why |
|---|---|---|---|
| `presence` | validator | `DDL` `DONE` | `not null` |
| `uniqueness` | SELECT-then-INSERT (racy!) | `DDL` `DONE` | real `UNIQUE` constraint — strictly stronger than Rails' |
| `inclusion` | validator | `GEN` `DONE` | enum types; DB `CHECK` opt-in (D12) |
| `length` / `format` / `numericality` / `comparison` | validators | `LATER` | candidates for extended-DBML settings compiling to `CHECK`s and/or a generated `Validate()`; not scheduled |
| `acceptance` / `confirmation` | form-layer | `NO` | HTTP-form concerns, not model concerns |
| Custom validators / contexts / conditional validation | framework | `HAND` | `func (u User) Validate() error` in a sibling file (D18) |
| `errors` object & rich error API | framework | `HAND`/`LATER` | plain Go errors first; a tiny multi-error type in the runtime if demand appears |
| Skipping validations | `save(validate: false)` | `NO` | nothing implicit to skip |

## 5. Callbacks (`active_record_callbacks.md`)

| Capability | Rails | Verdict | How / why |
|---|---|---|---|
| The entire lifecycle-callback system (`before_save`, `after_create`, `after_commit`, …) | core feature | `NO` | hidden control flow is the single biggest source of ORM haunting. Replacements, explicitly: wrap the generated call in your own function (before/after logic in plain sight); DB triggers in the schema file (`LATER`) for data-adjacent reactions; "after_commit" = the line after a successful `Tx` block |

## 6. Migrations (`active_record_migrations.md`)

| Capability | Rails | Verdict | How / why |
|---|---|---|---|
| Create/drop table, add/remove/change column, indexes, FKs | hand-written `change` | `GEN(v4)` | computed diff: current DBML vs last-migrated snapshot (D20) |
| Rename column/table without data loss | `rename_column` | `DBML(v4)` | `[was: 'old']` provenance hint — the one thing a structural diff can't infer |
| Reversibility (`change`/`up`/`down`) | assumed | `GEN(v4)` | down generated only when mechanically safe; otherwise marked irreversible — the reversibility lie, refused |
| Versioning + `schema_migrations` | ordered log | `GEN(v4)` | ledger `0001_…` + table + schema content hash (D21) |
| Schema dump (`schema.rb`) | derived snapshot | `NO` | unnecessary by construction — the DBML file *is* the schema statement |
| Data migrations | discouraged mixing | `GEN(v4)` | separate ledger entry kind (plain SQL), never mixed into structural steps |
| Seeds | `db/seed.rb` | `DONE` | `records` → generated `INSERT`s |
| Running/rollback/status CLI | `rails db:*` | `GEN(v4)` | `dbml migrate up/down/status`, embeddable in the user's binary (D04/D23) |
| SQLite structural change (the twelve-step rebuild) | n/a (Rails hides per-adapter) | `GEN(v4)` | owned: create-new + `INSERT…SELECT` + drop + rename in the pragma bracket (D22) |
| Multiple databases / sharding | Rails 6+ | `NO` | SQLite-first; "multiple databases" in SQLite is `ATTACH`, revisit if ever needed |

## 7. Active Model (`active_model_basics.md`)

| Capability | Rails | Verdict | How / why |
|---|---|---|---|
| Table-less models, attribute API | modules | `NO` | that's just a Go struct; the language starts where Rails needs a framework |
| Dirty tracking | changed-attribute API | `NO` | value semantics make "changed since load" meaningless; compare two values if you care |
| Serialization | `as_json` etc. | `DONE` | `json` tags on generated structs; `Null[T]` marshals value-or-null (D13) |
| SecurePassword | bcrypt sugar | `HAND` | a few lines with `golang.org/x/crypto/argon2` (argon2id — bcrypt is dated) in your own file |
| Naming / translation / lint tests | framework glue | `NO` | framework-internal machinery with no Go counterpart needed |

## 8. Encryption (`active_record_encryption.md`)

| Capability | Rails | Verdict | How / why |
|---|---|---|---|
| Encrypted attributes (incl. deterministic-for-query) | `encrypts :field` | `LATER` | generatable as a wrapper type with Valuer/Scanner if demand appears; note the SQLite-native alternative is whole-file encryption (SQLCipher). Not scheduled |

## 9. Composite primary keys (`active_record_composite_primary_keys.md`)

| Capability | Rails | Verdict | How / why |
|---|---|---|---|
| CPK in schema | `primary_key: [...]` | `DONE` | `[pk]` composite index → `PRIMARY KEY (a, b)` |
| CPK find/where | array args | `GEN(v0)` `DONE` | `UserRoleGet(ctx, userID, roleID)` — one typed argument per pk column |
| CPK associations | `query_constraints` | `GEN(v3)` | row-value `IN` loaders |

## 10. Cross-cutting (no Rails guide, still table stakes)

| Capability | Verdict | How / why |
|---|---|---|
| `context.Context` on every call | `GEN(v0)` `DONE` | first parameter, always |
| Works inside and outside transactions | `GEN(v0)` `DONE` | `Queries` over `DBTX` (satisfied by `*sql.DB` and `*sql.Tx`), `WithTx` |
| Connection setup & SQLite pragmas | `RT(v0)` `DONE` | `Open(driver, dsn)` helper: WAL, `busy_timeout`, `foreign_keys=ON` — the three everyone forgets |
| SQL injection safety | `GEN` all slices | named placeholders (`:email`) everywhere, bound via `sql.Named` (D15); string interpolation never emitted |
| Query logging / instrumentation | `LATER` | optional runtime hook (`func(sql string, args []any, d time.Duration)`) |
| `ErrNotFound` semantics | `GEN(v0)` `DONE` | `rt.ErrNotFound` *is* `sql.ErrNoRows` — one sentinel, `errors.Is`-compatible both ways (D36) |
| Prepared-statement caching | `RT(v2)` `DONE` | deterministic rendering makes SQL text the cache key (D31); wired via `Queries.WithCache` |
| End-user report engine (arbitrary runtime queries) | `LATER` | generated schema *catalog* + quarantined interpreter returning `[]map[string]any` (D33) |

---

## Tallies and the shape of the tool

Of ~75 inventoried capabilities: roughly **40 are generated or DBML-declared**,
**10 live in the small runtime**, **8 are consciously hand-written patterns**,
**7 deferred**, and **~15 are refused** — and every refusal is a *cause* of ORM
haunting (callbacks, dirty tracking, default scopes, lazy loading, identity,
inheritance) rather than a capability users lose. That is the claim of the
project in one sentence: **the boring 90% is derived from the schema; the
dangerous 10% is replaced by explicit Go; nothing is magic.**

Rails users write raw SQL for only a few percent of queries. We match that
bar differently: the few percent has a *home* — a `Select` block inside the
schema file — where regeneration re-validates it against every future schema
change, instead of rotting silently in a `.go` string.
