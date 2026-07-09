# The Model Layer

*Design note. This is the problem analysis behind Not an ORM
([`not-an-orm.md`](not-an-orm.md)); the per-capability plan derived from it
is [`orm-capability-matrix.md`](orm-capability-matrix.md).*

## Why M is the hard one

The single fact that makes M hard: the same truth exists in four
representations at once, and the real job of M is keeping them coherent.

1. **Design / visual** — the entity-relationship picture in your head, on a diagram, or in a declarative schema language (DBML, an ERD).
2. **Schema** — the DDL: CREATE TABLE, constraints, indexes, and the migrations that produce them.
3. **Code** — the Go structs and the data-access functions over them.
4. **Runtime** — the live database as it actually exists right now on disk.

"Good luck" is the sound of these four drifting apart. The diagram says one thing,
the migrations another, the structs a third, and production a fourth. Every hard
problem below is a special case of this drift.

M decomposes into five sub-concerns. Frameworks differ mostly in how they tangle
or separate them.

### Domain modeling

The structs and the invariants. What a Post is, independent of any database:
its fields, its value objects (an Email, a Money, a Slug that cannot be
constructed invalid), and its validation rules. This is Rails' Active Model — model
behavior with no table behind it.

Key questions this sub-layer answers:

- Where does validation live — on the struct, in the controller, at the DB? (The honest answer is usually "all three, at different granularities": the type system enforces shape, the struct enforces business invariants, the DB enforces the last-resort constraints.)
- Do you model with anemic structs (data only, logic elsewhere) or rich types (data plus the methods that protect its invariants)? Go's grain leans toward small types with a few guarding methods, not deep behavior.

### Persistence and the impedance mismatch

The object–relational impedance mismatch is the root problem: objects are graphs
with identity, references, and (sometimes) inheritance; relations are flat sets of
rows joined by keys. Moving between them is never free, and every persistence tool
is a different bet on how to pay.

The spectrum, from most magic to least:

| Approach          | What it is                                              | Examples            |
|-------------------|---------------------------------------------------------|---------------------|
| Active Record ORM | The object persists itself: post.Save()                 | Rails AR, Eloquent  |
| Data Mapper       | A separate mapper moves data; the object is DB-ignorant | Hibernate, Doctrine |
| Query builder     | Programmatic SQL; you map the rows yourself             | squirrel (Go), jOOQ |
| Raw SQL + scan    | Hand-written SQL, manual struct scanning                | database/sql        |
| Codegen from SQL  | You write SQL; a tool generates typed Go                | sqlc                |

Sub-problems every approach must answer:

- **Identity** — when two queries load "the same" row, are they the same object? (The identity-map pattern. Most Go data layers deliberately do not solve this; each scan is a fresh struct, and that simplicity is a feature.)
- **Associations** — how a Post reaches its Comments. Eager (join up front) vs lazy (load on access). Lazy loading is where the N+1 problem is born: one query for the list, then one more per item.
- **Where the SQL physically lives** — inline in handlers (scatters it), in a per-model data-access file (the maintainable default), or generated from .sql files (codegen). The handmade lean: SQL in per-model functions, or sqlc reading hand-written .sql, never reflection guessing your queries.

### Schema evolution (migrations) — "good luck"

This is the hardest sub-layer, and the difficulty is structural, not incidental.

**Imperative vs declarative.** Two opposite philosophies:

- Imperative (up/down scripts): you write the steps — "add this column," "drop that index." The current schema is the sum of all migrations ever run. Rails, most Go migration tools.
- Declarative (target schema): you describe the schema you want, and a tool computes the diff against the live database and generates the steps. Atlas, sqldef, Prisma migrate.

**The source-of-truth taxonomy.** A schema can have exactly one canonical
representation; the others should be derived. Who is canonical is the whole
design decision:

| Canonical truth            | Migrations are…                     | Code is…                   | Examples                     |
|----------------------------|-------------------------------------|----------------------------|------------------------------|
| Code-first                 | generated from struct diffs         | hand-written (the truth)   | Django, GORM AutoMigrate, EF |
| Migration-first            | hand-written (the truth)            | reads a schema snapshot    | Rails                        |
| Schema-first / declarative | computed as a diff to a schema file | generated from the schema  | Atlas, Prisma, DBML-driven   |
| Database-first             | applied out of band                 | generated from the live DB | sqlc, jOOQ                   |

When you pick none of these and hand-maintain all four, they drift. That is the
entire problem in one sentence.

**The reversibility lie.** "Every migration has a down." In practice many do
not. DROP COLUMN cannot restore the data it deleted; a data backfill cannot be
un-backfilled meaningfully. Down-migrations are honest only for pure structural,
data-preserving changes. Treat irreversible migrations as irreversible and stop
pretending.

**Schema vs data migrations.** Changing the shape of the database (a column) is
a different risk class from changing the contents (backfilling a value into a
million rows). Tangling them in one migration is a frequent source of pain;
many teams forbid data changes inside schema migrations entirely.

**Ordering, versioning, and team drift.** Migrations are an ordered log. Two
developers branching off the same point both write "migration 0042," and now there
are two. The live database has run one set; the repo describes another. This is
the two-truths problem in miniature, and it is why migration tooling obsesses
over versioning and a recorded schema_migrations table.

**Zero-downtime: expand/contract.** You cannot rename a live column in one shot
without breaking either old or new code. The pattern is: expand (add the new
column, write to both), deploy, backfill, switch reads, then contract (drop the
old column) in a later migration. A single logical change becomes three deploys.
This is the cost of never taking the app down.

**SQLite's specific pain.** SQLite's ALTER TABLE is deliberately minimal: it can
ADD COLUMN, RENAME TABLE, RENAME COLUMN, and (in recent versions) DROP COLUMN,
and almost nothing else. Changing a column's type, adding a constraint,
or reordering forces the documented twelve-step dance: create a new table with the
desired shape, copy the data across, drop the old table, rename the new one — with
PRAGMA foreign_keys turned off for the duration and turned back on after. Your
migration runner has to know this dance, because SQLite will not do it for you.

### Schema design and visualization (the DBML connection)

This is the part most frameworks ignore and the part that matters most as a schema
grows. The claim:

> Design, schema, and code are not three artifacts to maintain separately. They
> are one pipeline with one source of truth and the rest derived.

**Why visualization is not a nicety.** A six-table schema lives in your head. A
sixty-table schema does not. At that size the diagram is the mental model — the
only representation a human can actually hold. A declarative schema language like
DBML gives you that diagram and a precise, version-controllable text source at the
same time, which is why it belongs in the loop, not in a wiki nobody updates.

**The coherent pipeline (one truth, everything derived).** A handmade,
SQLite-based, DBML-driven stack wants the declarative schema to be canonical:

```
   DBML (design + diagram, the canonical truth)
     │  export / maintain
     ▼
   SQL schema (DDL)
     │  diff against the live DB  ──►  generated migration  ──►  apply to SQLite
     ▼
   structs + typed queries  ◄── codegen reads the schema (e.g. sqlc)
```

In this arrangement DBML is where you think and see, the migration is a computed
mechanism to move the database toward the design, and the Go structs are
generated from the schema rather than hand-kept. Three of the four
representations become derived; only one is authored. Drift has nowhere to live.

How much of this chain you hand-roll versus adopt is a separate decision — you can
own the migration runner and the diff while letting a codegen tool produce the
structs — but the shape is the point: pick one canonical representation, derive
the others, compute migrations as a diff toward the target.

### Querying (the read side)

Persistence is mostly about writes and identity; querying is its own concern.

- **Typed vs stringly-typed.** Raw SQL strings are flexible and unchecked; a typed builder or codegen (sqlc) catches column and type mistakes at compile time at the cost of a generation step. The handmade lean is codegen: the output is plain Go you can read, with none of a query builder's runtime indirection.
- **Projection vs full hydration.** A list view rarely needs every column. Loading the whole struct when you need three fields is the read-side equivalent of N+1 waste. The query layer should make narrow projections easy, not force full-row hydration everywhere.
- **N+1, again.** It reappears on the read side as soon as a template touches an association the query did not load. The fix is loading what the view needs in one query, which couples the query shape to the view's needs — a tension to manage, not eliminate.
