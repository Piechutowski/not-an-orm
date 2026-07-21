# DBML Vet Rules

This document specifies every `vet` analyzer: what it reports, why the
pattern is a concern even though it is **valid DBML**, and its known
limitations. It is the lint annex to the [language specification](../SPEC.md);
errors (invalid DBML) are the [check package's](../check/) business and are
not listed here.

**The examples are executable, not prose.** Each rule links to its test
file under [`testdata/`](testdata/). In those files, a line ending in
`//WANT <rule>` is a **bad** example — the analyzer MUST warn on exactly
that line — and every unmarked line is a **good** example — the analyzer
MUST stay silent. `go test ./vet/` verifies both directions, and
`TestRulesDocumentation` fails the build if any analyzer is missing from
this document, links to a nonexistent file, or has no bad example in its
linked tests. The docs cannot silently drift from the code.

Run a single rule with:

```sh
dbml vet --enable <rule> file.dbml
```

---

## Dead declarations

The analogue of Go's "imported and not used": a declaration nothing
references is usually a refactoring leftover, and it keeps readers
searching for uses that do not exist.

### unusedenum

**What it does.** Reports an `Enum` that no column in the file uses as its
type.

**Why.** An enum's only effect is through columns typed with it; an unused
enum is dead weight that still occupies a name in its schema.

**Examples.** [testdata/unused.dbml](testdata/unused.dbml) — `orphan_status`
is bad; `used_status` and the schema-qualified `core.kind` are good.

**Limitations.** Analysis is per-file: a file that exports the enum to
other files via the module system (§7) may see a false positive. Treat the
warning as "unused *here*".

### unusedpartial

**What it does.** Reports a `TablePartial` that no table injects.

**Why.** Same as `unusedenum`; partials exist only to be injected.

**Examples.** [testdata/unused.dbml](testdata/unused.dbml) —
`orphan_partial` is bad; `used_partial` is good.

**Limitations.** Per-file, as above.

### unusedalias

**What it does.** Reports a table alias (`Table users as U`) that no
relationship endpoint, TableGroup member, DiagramView member or records
declaration uses.

**Why.** The alias occupies a slot in the global table namespace (§8.2.1)
— it can collide with future table names — while providing nothing.

**Examples.** [testdata/unused.dbml](testdata/unused.dbml) — `P` is bad;
`U` (used by `Ref: U.id < posts.id`) is good.

**Limitations.** Per-file, as above.

---

## Redundancy

Two ways of saying the same thing eventually disagree. Each of these
patterns states something the language (or another line) already
guarantees.

### redundantnull

**What it does.** Reports an explicit `[null]` column setting.

**Why.** Columns are nullable by default (spec §6.3.2), so `[null]` adds no
information. Worse, it reads as if it were load-bearing, suggesting the
author believed the default was `not null`.

**Examples.** [testdata/redundantnull.dbml](testdata/redundantnull.dbml) —
`[null]` is bad; `[not null]` and no setting are good.

### redundantindex

**What it does.** Reports four index patterns:
1. `[pk, unique]` on one index — a primary key is already unique;
2. a single-column `[unique]` index on a column already marked `unique`;
3. a single-column `[unique]` or `[pk]` index on a column already marked `pk`;
4. two indexes with an identical key (column order significant).

**Why.** The duplicate creates a second physical index (cost on every
write) or a second spelling of the same constraint that can drift from the
first.

**Examples.** [testdata/redundantindex.dbml](testdata/redundantindex.dbml)
— note that `(email, country)` and `(country, id)` coexisting is good:
composite indexes with different column order are different indexes.

**Limitations.** Expression atoms are compared textually: `` `a+b` `` and
`` `a + b` `` count as different keys.

### emptybody

**What it does.** Reports empty `indexes {}` and `checks {}` blocks, empty
`TableGroup` bodies, empty (non-wildcard) DiagramView categories, and empty
`Project` bodies.

**Why.** An empty block does nothing but implies work in progress; an empty
DiagramView category in particular reads as "show everything" but actually
selects nothing (spec §6.13.2).

**Examples.** [testdata/emptybody.dbml](testdata/emptybody.dbml) —
`Notes { * }` is good: a wildcard is a real selection.

### legacyflag

**What it does.** Reports the legacy bare `pk` / `unique` flags between a
column's type and its settings list (`id int pk`).

**Why.** The form survives for backward compatibility (spec §6.3.7) but
splits column constraints across two syntaxes; the settings list is the
canonical spelling.

**Examples.** [testdata/legacyflag.dbml](testdata/legacyflag.dbml) —
`ok int [pk]` is the good spelling.

### shadowedcolumn

**What it does.** Reports a column definition that overrides another via
partial-injection conflict resolution (spec §6.9.4) — table-over-partial or
later-partial-over-earlier-partial.

**Why.** The override is **silent**: no error, last writer wins. A type or
constraint change in the losing partial then has no effect on this table,
which is invisible at the partial's definition site.

**Examples.** [testdata/shadowedcolumn.dbml](testdata/shadowedcolumn.dbml)
— both conflict directions are bad; non-overlapping columns are good.

### projectnotes

**What it does.** Reports a `Project` body holding more than one `Note`
definition.

**Why.** Multiple notes are accepted (spec §6.11.1 exempts Project bodies)
but downstream tools render a single project note, so all but one are
silently dropped. The good form is a single (possibly multi-line) note.

**Examples.** [testdata/projectnotes.dbml](testdata/projectnotes.dbml).

### unnecessaryquotes

**What it does.** Reports a quoted identifier (`"users"`) whose content is
already a valid plain identifier.

**Why.** Quoting suggests the name *needs* quoting (spaces, keywords in
some SQL dialect); when it does not, the quotes are visual noise and invite
inconsistent spellings of the same name.

**Examples.**
[testdata/unnecessaryquotes.dbml](testdata/unnecessaryquotes.dbml) —
`"full name"` and `"double precision"` are good: they contain spaces and
genuinely need quotes.

**Limitations.** "Plain" is judged by ASCII letters/digits/underscore; a
quoted identifier containing non-ASCII letters is not reported even though
it may be unquotable.

---

## Modeling traps

Valid DBML that describes a database likely to misbehave. This is the
family where a warning most often prevents a production incident.

### missingpk

**What it does.** Reports a table with no primary key: no `pk`/`primary
key` column setting (including via injected partials and legacy flags) and
no `[pk]` index.

**Why.** Tables without primary keys defeat replication, upserts, ORMs and
row identity in general. Almost always an oversight.

**Examples.** [testdata/missingpk.dbml](testdata/missingpk.dbml) — all
three ways of declaring a pk (column setting, composite index, legacy flag)
are good.

### reftype

**What it does.** Reports a relationship whose two columns have different
declared types, compared pairwise for composite endpoints.

**Why.** DBML does not require type agreement, but the exported database
either rejects the FK or performs implicit casts (`bigint` vs `integer`
overflows; `varchar` vs `int` breaks joins).

**Examples.** [testdata/reftype.dbml](testdata/reftype.dbml).

**Limitations.** Types are compared as written (case-insensitively):
`int` vs `integer` are reported as different even where a database treats
them as aliases. Prefer one spelling per schema.

### reftarget

**What it does.** Reports a foreign key whose referenced ("one"-side)
column set is not guaranteed unique: not a `pk`/`unique` column and not
covered by a matching composite `[pk]`/`[unique]` index. The FK side per
operator follows spec §6.7.3 (`>` left, `<` right, `-` second endpoint);
`<>` has no FK side.

**Why.** SQL requires FK targets to be unique; a schema that violates this
fails at export or, worse, models a "to-one" relationship that the data
cannot uphold.

**Examples.** [testdata/reftarget.dbml](testdata/reftarget.dbml) — the
composite case shows that only an index with **the same column set** makes
a composite target unique.

### selfref

**What it does.** Reports a relationship with the same column on both
sides (`employees.id - employees.id`).

**Why.** A column trivially equals itself; the relationship is either a
typo for a hierarchy edge (`manager_id > id`) or meaningless.

**Examples.** [testdata/selfref.dbml](testdata/selfref.dbml) —
`manager_id > id` (a real hierarchy) is good.

### refcycle

**What it does.** Reports cycles in the foreign-key graph between distinct
tables (`a → b → a`), each cycle once.

**Why.** Cyclic FKs mean no valid insertion order exists without deferred
constraints or nullable columns, and bulk loads/dumps need special
handling. Sometimes intentional — then the warning documents the decision.

**Examples.** [testdata/refcycle.dbml](testdata/refcycle.dbml) — the
acyclic `c → a` edge is good; self-references are excluded (that hierarchy
pattern is legitimate and covered by `selfref` only when degenerate).

### casecollision

**What it does.** Reports two names in one namespace differing only in
letter case: tables per schema, columns per table, enums per schema.

**Why.** DBML names are case-sensitive (spec §1.4) but many databases and
most humans are not; `users` next to `Users` is a bug generator.

**Examples.** [testdata/casecollision.dbml](testdata/casecollision.dbml).

---

## Code generation

Rules that guard the DBML → Go mapping (decisions
[D09–D13](../docs/decisions.md)) rather than the schema itself.

### modelname

**What it does.** Reports a table whose singular Go model name is a
*guess*: the [inflector](../inflect/) could not confidently singularize the
table name (endings like `-us`, `-is`, `-ss` where a trailing `s` is often
not a plural marker at all) and no `[model: '...']` setting pins the name.

**Why.** Models are singular by decision D10 (`users` → `User`), but
English pluralization is not decidable by rules: `menus` wants `Menu`,
`status` wants `Status`, and a deterministic inflector cannot know which is
which. Generation proceeds with the guess either way; this rule marks
exactly the places where the guess deserves a human eye. Writing
`[model: 'Menu']` both fixes the name and silences the warning — the
setting is the documentation.

**Examples.** [testdata/modelname.edbml](testdata/modelname.edbml) —
regular plurals (`users`, `order_items`), irregulars the inflector knows
(`people`, `account_statuses`), and a pinned `menus [model: 'Menu']` are
good; `order_status`, `bonus` and `axis` warn.

**Limitations.** Only the final underscore-separated segment is inflected,
and the confidence heuristic is suffix-based: a wrongly-guessed regular
plural (schemas full of domain jargon) stays silent. `[model:]` always
wins; use it whenever the generated name matters.

### dynname

**What it does.** Reports declarations whose *generated dynamic-query
names* collide in Go package scope. The dynamic layer (decisions D28-D30)
mints names by concatenation — `<Model>Cols` for the column-handle set,
`<Model>Limit`, `<Model>Offset`, `<Model>Distinct`, `<Model>OrderBy`,
`<Model>After`, `<Model>Set` for the option wrappers — alongside the model
structs, params structs, enum types and enum constants, so two distinct
DBML declarations can flatten to one Go identifier: `Table user_limits`
mints model `UserLimit`, exactly the option wrapper of `Table users`.

**Why.** `nao gen go` refuses such schemas outright (loud failure, never a
silent rename), but the generator can only name the first collision; this
rule reports every collision, at the later of the two declarations, with
both origins spelled out. The name derivation is the generator's own — the
rule calls into `gen/golang` — so rule and generator cannot drift apart.

**Examples.** [testdata/dynname.edbml](testdata/dynname.edbml) — plain
`users` and `orders` coexist fine; `user_limits` (model = users' `UserLimit`
wrapper), `Enum user_offset` (type = users' `UserOffset` wrapper) and a
table pinned to `[model: 'UserCols']` (= users' handle set) all warn.

**Limitations.** Per-file, like every rule here. A schema that fails
generation planning altogether (unusable names, field collisions) reports
nothing from this rule — generation raises those errors itself.

---

## Candidate rules — considered, not implemented

Reviewed and deliberately deferred; listed so a reader can see what was
*not* overlooked but judged out of scope for now. Promoting any of these is
one file in `vet/` plus one testdata file.

| Candidate | What it would report | Why deferred |
|---|---|---|
| `junctionhint` | `<>` many-to-many refs, reminding that SQL export materializes an implicit junction table (spec §6.7.7) | informational, not a defect; would fire on every legitimate `<>` |
| `orphantable` | tables with no relationships at all | too noisy: lookup/config tables are legitimately standalone |
| `nullablefk` | nullable FK columns, i.e. zero-or-many cardinality (spec §8.5) | that cardinality is a valid modeling choice, not a smell |
| `recordstypes` | record values inconsistent with column types (dates, enum membership, booleans) | needs the §6.10.4 value-typing engine; belongs in `check` when built |
| `recordsdupkey` | duplicate primary-key values across record rows | same engine as above |
| `sqlkeyword` | unquoted names that are reserved words in common SQL dialects (`order`, `user`) | needs per-dialect wordlists and a target-dialect setting |
| `namingstyle` | mixed naming conventions (snake_case vs camelCase) | pure style; needs configuration machinery first |
| `unusedstickynote` | sticky notes not shown by any DiagramView | views are optional; most files have none, so every note would warn |

---

## Adding a rule

1. Create the analyzer in a `vet/*.go` file and `register` it.
2. Add `testdata/<rule>.dbml` with `// analyzers: <rule>` on line 1, bad
   lines marked `//WANT <rule>`, and good lines unmarked.
3. Add a `### <rule>` section here linking that file.

`TestRulesDocumentation` and `TestAnalyzers` enforce that all three exist
and agree.
