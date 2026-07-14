# CLAUDE.md

This repository is **Not an ORM**: a DBML-first code generator for Go +
SQLite. One authored file (the DBML schema) — structs, CRUD, typed queries,
DDL and migrations derived from it. The repo also contains the normative
DBML language specification and its reference front end.

> [!CAUTION]
> **This project is an alpha version, and everything may change in the future.**

The project is deliberately fluid: names, layout, CLI surface and APIs are
still seeking their best shape. There are no users yet, so there is no
backward compatibility to protect — when a better shape appears, propose
it and rename/restructure boldly instead of preserving the status quo.
Locked decisions (docs/decisions.md) still bind until edited.

## Read these first, in this order

1. [`docs/decisions.md`](docs/decisions.md) — **the law.** Numbered locked
   decisions (D01–D40). Never contradict one silently; changing a decision
   means editing that file in the same commit.
2. [`docs/features.md`](docs/features.md) — **the build plan.** Every
   feature by problem solved, with stable IDs (`CRUD-3`, `SEL-2`, `MIG-1`),
   slice and status. Implementation sessions work off this list; landing a
   feature updates its status in the same commit.
3. [`docs/orm-capability-matrix.md`](docs/orm-capability-matrix.md) — every
   ORM capability with my verdict and roadmap slice (v0–v4); the
   exhaustiveness audit behind features.md. Scope changes update the matrix.
4. [`docs/not-an-orm.md`](docs/not-an-orm.md) — the vision;
   [`docs/the-model-layer.md`](docs/the-model-layer.md) — the problem
   analysis behind it.
5. [`SPEC.md`](SPEC.md) — the DBML language spec (normative, EBNF).
   Extensions I add (Select, View, `[was:]`, `[model:]`, `[repr:]`) are a
   superset; the core stays conformant to upstream DBML.

## Architecture (Go-toolchain layering; keep stages decoupled)

| Package | Role |
|---|---|
| `edbml/token`, `edbml/scanner` | lexer (Pike state functions; newline-as-ASI flag) |
| `edbml/ast`, `edbml/parser` | syntax only — no semantic judgment, multi-error recovery |
| `edbml/check` | semantics + `check.Info` symbol table (partials expanded, refs resolved) — **all generators consume Info, never re-derive from the AST** |
| `edbml/vet` | analyzer framework + rules; docs in `edbml/vet/RULES.md` |
| `gen/golang`, `gen/sqlite` | generators (structs + CRUD queries, DDL+seeds); shared corpus `gen/testdata/*.dbml` |
| `rt` | hand-written runtime for generated code: `Null[T]`, `DBTX`, `Tx`, `Open` (pragmas), `StmtCache` — stdlib-only, registers no driver |
| `inflect` | deterministic singularizer behind model naming (D10); `edbml/vet/modelname` flags its guesses |
| `itest` | integration fixture: checked-in generated files (drift-tested, `go generate ./itest` refreshes) + real-SQLite CRUD round trips (mattn, **test-only** cgo dep, D25) |
| `cmd/edbml` | the one binary (D41): thin urfave/cli wrapper over everything above, `edbml lsp` serves the language server — all of it stays library-callable (D04) |
| `edbml/conformance/` | spec snippet corpus + upstream `@dbml/parse` cross-check (`refcheck/`, needs bun) |

## Invariants the tests enforce (don't break, extend)

- `go test ./...` runs everything: front-end unit corpus, the 80-snippet
  conformance corpus (valid MUST pass, invalid MUST fail), vet's
  `//WANT`-annotation tests **plus** the docs⇄registry⇄testdata consistency
  test (`vet/docs_test.go`), and generator goldens.
- Generated Go goldens must be gofmt-stable and **compile** (real toolchain);
  generated SQL goldens must **execute** on real SQLite (via python3's
  sqlite3, `PRAGMA foreign_keys=ON` + `foreign_key_check`); every generated
  CRUD statement must **prepare** against the generated DDL
  (`gen/golang/queries_test.go`), and `itest/` runs the whole surface
  against a real driver.
- Golden refresh: `go test ./gen/... -update` (and inspect the diff — goldens
  exist to surface unintended output changes in review).
- Every new vet rule = analyzer + `testdata/<rule>.dbml` (`//WANT` markers)
  + `### <rule>` section in `edbml/vet/RULES.md`, or the build fails.
- Generation failures are loud: unknown type, name collision → error naming
  the column. Never guess (D16-adjacent principle).

## Conventions

- Naming is subject-first, verb-last everywhere: `UserGet`, `UserCreateParams`,
  `PostCommentsLoad`, `OrderStatusPending` (D09).
- Zero runtime dependencies in generated code; the ecosystem (sqlc, squirrel,
  sqlx, Ent) is reference material, never a dependency (D03).
- SQLite-first, all-in; SQLite itself is the gen-time SQL parser/type checker
  via in-memory prepare (D02, D06).
- Error messages and comments cite spec sections (`§6.7`) or decision numbers.
- Generated files start with the `Code generated ... DO NOT EDIT.` marker;
  the CLI refuses to overwrite files lacking it.

## Environment notes (restricted sandboxes)

- Module fetches may need `GOPROXY=direct GOSUMDB=off` (proxy.golang.org and
  sum.golang.org can be blocked; github.com direct works).
- No sqlite3 CLI; use python3's sqlite3 module (3.45) for executing SQL.
- The upstream cross-check (`conformance/refcheck/setup.sh`) rebuilds the
  reference compiler from git history and runs under bun.
- Rails guide reference copies: `docs/reference/rails/` (`fetch.sh` refreshes).

## Working style expected by the maintainer

Plan before building (design docs first for big features), integration
tests over mocks for the data layer, exhaustive unit corpus for language
code, golden files for generator output, and every non-obvious behavior
either specified in `SPEC.md`, decided in `docs/decisions.md`, or ruled
in `vet/RULES.md`. When a session produces new decisions, append them to
`docs/decisions.md` before committing.
