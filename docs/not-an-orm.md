# Not an ORM

**An ORM-grade developer experience for Go, without the ORM.** One source of
truth — your DBML schema — and the boring 90% of database code is *generated*:
structs, CRUD, typed queries. No reflection, no inheritance, no monkey-patching,
no runtime magic. Just plain Go you can read, open in your editor, and step
through in a debugger.

> **Status: design phase, with the foundation built.** The DBML front end
> (spec, scanner, parser, checker, vet), the Go struct generator and the
> SQLite DDL generator live in this repository. The method/query/migration
> layers below are the plan. The long-form analysis behind it lives in
> [`the-model-layer.md`](the-model-layer.md); the per-capability plan in
> [`orm-capability-matrix.md`](orm-capability-matrix.md); the locked design
> decisions in [`decisions.md`](decisions.md).

---

## The pain

This is the function every Go CRUD app makes a developer write, by hand, a
thousand times:

```go
func GetUserByID(db *sql.DB, id int64) (User, error) {
	const q = `SELECT id, name, email FROM users WHERE id = ?`

	var u User
	err := db.QueryRow(q, id).Scan(&u.ID, &u.Name, &u.Email)
	if err != nil {
		return User{}, err
	}
	return u, nil
}
```

There is nothing to *think* about here. The column list is the struct. The scan
targets are the struct fields, in order. The `WHERE id = ?` is mechanical. And yet
Go gives you no way to not write it — so you write it again for `GetUserByEmail`,
and again for every table in the schema, and again the day someone adds a column
and the scan order silently rots.

Rails and Django make this disappear. They do it with metaprogramming,
inheritance, and monkey-patching — techniques that are powerful in Ruby and Python
and deeply *against the grain* in Go. GORM brings that style to Go and is widely
disliked for exactly that reason: reflection everywhere, magic you can't read, and
errors that surface at runtime instead of compile time.

So Go developers are stuck between two bad options: hand-write the boilerplate
forever, or adopt a reflective ORM that fights the language.

## The bet

There is a third way, and it is the idiomatic-Go way: **code generation.**

The code above *shouldn't be written by hand* — but it also shouldn't be replaced
by runtime reflection. It should be **generated** from something more abstract,
once, into plain Go that looks exactly like what you'd have written yourself.
`sqlc` proved Go developers love this: you get the ergonomics of an ORM and the
transparency of hand-written code, with the costs paid at build time instead of
request time.

Metaprogramming and inheritance were invented to kill code redundancy — but
before all the magic that languages like Ruby or Python give you, old-school
programmers were just generating code. It is a forgotten skill that Go's own
toolchain kept alive (`go generate`, stringer, protoc).

Not an ORM takes that bet and pushes it one step earlier in the pipeline. `sqlc`
starts from SQL. I start from **DBML** — the place most developers already think
about their schema.

### Why DBML is the source of truth

The hardest thing about the model layer is that the same truth lives in four
places at once — the diagram in your head, the schema DDL, the Go structs, and the
live database — and they constantly drift apart (the full argument is in
[`the-model-layer.md`](the-model-layer.md)). The only durable fix is to
make **one** representation canonical and *derive* the rest.

I pick the design as canonical, expressed in [DBML](https://dbml.dbdiagram.io/):

- It's how people **already** model — most developers visualize the database
  before they write a line of code or a single migration.
- It's **both** a precise, version-controllable text source **and** a diagram. A
  six-table schema lives in your head; a sixty-table schema does not — at that
  size the diagram *is* the mental model, and DBML keeps it in the repo instead of
  a wiki nobody updates.
- It's declarative, so migrations become a *computed diff* toward the target
  rather than a hand-maintained pile of scripts.

```
   DBML  ── the one thing you author ────────────────┐
     │                                               │
     ├─►  Go structs (with tags)                     │  everything
     ├─►  conventional CRUD  (UserGet, UserList…)    │  below the line
     ├─►  typed dynamic queries (filter/sort/page)   │  is GENERATED,
     ├─►  association loading (posts → comments)     │  never hand-kept
     └─►  SQL schema ─► migration diff ─► SQLite     │
                                                     ┘
```

## What I generate

### 1. Structs

```dbml
Table users {
  id         integer  [pk, increment]
  name       text     [not null]
  email      text     [not null, unique]
  created_at timestamp [default: `datetime('now')`]
}
```

becomes

```go
type User struct {
	ID        int64     `db:"id"`
	Name      string    `db:"name"`
	Email     string    `db:"email"`
	CreatedAt time.Time `db:"created_at"`
}
```

### 2. Conventional CRUD — the boilerplate killer

Every table gets the set you'd otherwise hand-write, generated with explicit scans
(no reflection). Names are subject-first so autocomplete groups by model:

```go
func (q *Queries) UserGet(ctx context.Context, id int64) (User, error)
func (q *Queries) UserList(ctx context.Context) ([]User, error)
func (q *Queries) UserCreate(ctx context.Context, arg UserCreateParams) (User, error)
func (q *Queries) UserUpdate(ctx context.Context, arg UserUpdateParams) (User, error)
func (q *Queries) UserDelete(ctx context.Context, id int64) error
func (q *Queries) UserCount(ctx context.Context) (int64, error)
```

`GetUserByID` is now something you *describe*, not something you *type*.

### 3. Custom queries — when convention isn't enough

The 20% that's specific to your app you declare in the same DBML file, as a
`Select` element: the *structure* (columns, tables, joins, declared params) is
resolved and type-checked by my own front end, while the *expressions* stay
SQLite's business — at generation time each query is prepared against an
in-memory database built from the generated DDL, so **SQLite itself is the SQL
parser and type checker**. Joins are inferred from declared `ref:`s where
possible. No SQL grammar of my own, no sqlc dependency.

### 4. Typed dynamic queries — the hard one, on purpose

The classic case: a list view where the user can filter and sort by **any**
column. You can't pre-generate every combination, so the query is built at
runtime. But "runtime" doesn't have to mean "stringly-typed." I generate typed
column handles so the dynamic builder is column- and type-safe at the API
boundary:

```go
users, err := q.UserQuery(ctx,
	UserEmail.Eq("a@b.com"),
	UserName.Like("%fo%"),
	UserCreatedAt.Desc(),
	UserLimit(20),
)
```

`UserEmail` is a generated, model-tagged handle (`Column[User, string]`), not a
string. Rename the column in DBML, regenerate, and this stops compiling. Pass a
`Post` column into a `User` query and it *never* compiled. Options are inert
values — a tiny expression tree walked by one interpreter that renders SQL and
named args in lockstep — so predicates store in variables, compose with
`And`/`Or`/`Not`, and plug into `UserCount` and `UserDeleteWhere` unchanged.

### 5. Associations — the *other* hard one

A `Post` reaching its `Comments` is the object-relational impedance mismatch in
miniature. (Honest note: in embedded SQLite, N+1 is not the disaster it is over
a network — a query is a function call, and SQLite's own docs bless the
loop-of-queries pattern. My batched loaders are the default for *atomicity* —
one query sees one snapshot, while a loop in autocommit can interleave with
writes — and for ergonomics and large-N speed, not out of network-era fear.)
Because relationships are declared in DBML (`ref:`), I generate the loading
code — as explicit per-ref loaders, never as struct fields (a `Comments` field
on `Post` can't distinguish "not loaded" from "empty", and that ambiguity is
the seed of lazy loading):

```go
comments, err := q.PostCommentsLoad(ctx, posts)
// comments: map[int64][]Comment, keyed by post ID — ONE batched query
```

Default strategy: load parents, then `SELECT … WHERE fk IN (…)` for children and
group in memory — predictable, and it avoids the cartesian row-explosion you get
when you `JOIN` two has-many relations at once.

### 6. Migrations — the genuinely hard sub-layer

The extended-DBML file is **declarative**: it always describes the schema you
*want right now*. So migrations are a computed diff toward that target — you don't
hand-write `ALTER` scripts, you edit the schema and regenerate.

The subtlety (and it's a real one): a declarative file can't carry an imperative
"drop column X" annotation, because after the drop, column X isn't in the file to
annotate. So I keep the rule clean:

- **Add** a field → it appears in the file → differ emits `ADD COLUMN`.
- **Drop** a field → you delete it from the file → differ emits the drop. No
  annotation, no contradiction.
- **Rename** is the one case a structural diff *can't* infer (it looks like
  drop-old + add-new, which would lose the data), so the renamed field carries a
  provenance hint — `email_address [was: 'email']` — which annotates the thing that
  *still exists*. You can prune the hint once the migration is cut.

Versioning lives in the **generated** migrations (an ordered `0001_…`, `0002_…`
ledger plus a `schema_migrations` table), not as per-operation version numbers
inside the declarative file. Each generated migration embeds the content hash of
the DBML it targets, so drift ("schema changed, no migration cut") is detectable.
One authored truth (the schema), one derived ordered log (the migrations) — which
is exactly how you avoid the two-truths drift. Migrations are applied by the CLI
(`dbml migrate up`) or by the user's own binary embedding the runner — never
implicitly on app startup.

Views and triggers — which I add to the language — are the *cheap* part of
migrations: they're stateless, so drop-and-recreate is always safe. The expensive
part stays the **table** structural change, because SQLite's `ALTER TABLE` forces
the documented twelve-step rebuild dance. I own that dance too — I already
generate full `CREATE TABLE` statements, so a rebuild is a new table, an
`INSERT … SELECT` over the intersecting columns, a drop and a rename inside the
pragma bracket — and the test harness executes it against real SQLite.

## Principles

- **No reflection, no runtime magic.** Generated code uses explicit scans and
  explicit SQL. You can read it, grep it, and debug it. This is the line that
  separates me from GORM — and, notably, from `sqlx`'s reflective `StructScan`:
  since I generate the code anyway, explicit scans are free, so I take them.
- **One source of truth, everything derived.** You author DBML. Structs, CRUD, and
  schema are outputs. Drift has nowhere to live.
- **One file for everything.** Tables, relationships, enums, indexes — *and* the
  things plain DBML lacks: views, queries, triggers, and migration intent — live
  in a single *extended-DBML* source. No scattered DBML + `.sql` + Go to keep in
  sync.
- **SQL is not the enemy.** I generate SQL and let you write SQL fragments. I
  don't bury it under an abstraction that you'll eventually have to fight.
- **SQLite-first, all-in.** The target is SQLite — the database that's perfect for
  the solo dev and small-team apps this is built for. Every design simplification
  bought by rejecting other DBMSes is a good trade. SQLite is also a *tool I
  lean on*: at generation time it is my SQL parser and type checker.
- **One coherent tool, not glued libraries.** I own the parser, the checker, the
  emitters and the small runtime end to end — one consistent style across structs,
  CRUD, dynamic queries, and associations. Modularity means one well-built,
  extensible API — not guerrilla-gluing a combine harvester in the trenches.
- **Built for speed of building.** The audience is the solo developer who wants to
  ship a CRUD app *super sonic fast* without rebuilding the data layer from
  scratch every time.

## How I relate to what exists

I'm not replacing the ecosystem — I'm subsuming the good ideas and filling
the gap they leave.

| Tool             | Approach                | Auto CRUD | Dynamic queries   | Schema source | Reflection / magic     |
| ---------------- | ----------------------- | --------- | ----------------- | ------------- | ---------------------- |
| `database/sql`   | raw SQL + manual scan   | ✗         | manual            | —             | none                   |
| `sqlx`           | raw SQL + helpers       | ✗         | manual            | —             | yes (`StructScan`)     |
| `squirrel`       | runtime query builder   | ✗         | yes (string cols) | —             | none                   |
| `sqlc`           | codegen from SQL        | partial¹  | ✗ (static only)   | SQL / live DB | none                   |
| GORM             | active-record ORM       | ✓         | ✓                 | structs       | heavy reflection       |
| **Not an ORM**   | **codegen from DBML**   | **✓**     | **✓ (typed)**     | **DBML**      | **none**               |

¹ Only via the third-party `sqlc-gen-crud` plugin, which generates the CRUD `.sql`
for you to then feed back into `sqlc`.

What this table says plainly: `sqlc` is the closest predecessor — and the bar my
generated code should clear — but it is **static**: it cannot express the
filter-by-any-column list view, and it doesn't start from your design. `squirrel`
*can* do dynamic, but with stringly-typed columns and no codegen. GORM does
everything and pays for it with reflection. **The unclaimed corner is:
DBML-first, auto-CRUD, *typed* dynamic queries, zero reflection.** That's me.

## Non-goals

Saying no is how this stays small and fast:

- **No identity map / unit of work.** Each scan is a fresh struct — a value, not
  a shared object. Go data layers deliberately skip this, and the simplicity is a
  feature.
- **No lazy loading.** Associations load when you ask for them, in a batched query
  — never implicitly on field access. In Go this is impossible by construction
  (struct field access is a memory read), and I keep it that way.
- **No active-record `post.Save()`.** Objects don't persist themselves; a generated
  function does. Data stays DB-ignorant.
- **No reflection-based mapping**, ever, in generated paths.
- **No callbacks, no dirty tracking, no default scopes.** Hidden control flow and
  hidden query mutation are where ORMs turn into haunted houses.
- **Not a general-purpose query DSL.** For anything exotic, you write SQL — inside
  the schema file's `Select` blocks, where regeneration re-validates it.

## Roadmap (lean vertical slice first)

The front end (spec, parser, checker, vet) and the first two generators
(structs, SQLite DDL + seeds) are **done**. The slices ahead:

- **v0 — kill the boilerplate.** Conventional CRUD
  (`UserGet/UserList/UserCreate/UserUpdate/UserDelete/UserCount`, plus
  `UserGetByEmail`-style finders for unique columns) for SQLite, with the small
  runtime package (`Null[T]`, DBTX, pragmas helper).
- **v1 — declared queries.** `Select`/`View` elements in extended DBML → typed Go,
  validated by gen-time SQLite prepare.
- **v2 — typed dynamic queries.** Generated typed column handles + the
  filter/sort/paginate builder. *The differentiator.*
- **v3 — associations.** Batched per-ref loaders from declared `ref:`s.
- **v4 — migrations.** Declarative diff against the last-migrated snapshot,
  `[was:]` rename hints, ordered ledger, owned twelve-step SQLite rebuild.
