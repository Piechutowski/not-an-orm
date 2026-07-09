# DBML Conformance Test Suite

Executable companion to the specification in [`../SPEC.md`](../SPEC.md).

## Layout

- `snippets/valid/*.dbml` — programs a conforming implementation MUST accept.
- `snippets/invalid/*.dbml` — programs it MUST reject.
  Every snippet's first line is `// spec: §… — what it exercises`, so a
  failure points at the spec clause being violated.
- `refcheck/` — cross-check harness against the upstream `@dbml/parse`
  compiler.

The reference implementation lives at the repository root as an importable
Go module (`scanner`, `parser`, `check`, `vet`, `cmd/dbml` — see the
top-level README). The corpus is executed by that library's test suite:

## Running the suite

```sh
go test ./check/ -run TestConformanceCorpus -v   # from the repository root
```

or ad hoc against any file with the CLI:

```sh
go run ./cmd/dbml check conformance/snippets/valid/35_kitchen_sink.dbml
```

Every file under `valid/` must produce zero errors; every file under
`invalid/` at least one.

## Cross-checking against the upstream implementation

The corpus verdicts were verified against the upstream compiler
(`@dbml/parse`, extracted from this repository's git history at the fork
point). Requires [bun](https://bun.sh):

```sh
cd refcheck
./setup.sh
(cd work && bun refcheck.ts ../../snippets)
```

Expected output: `0 disagreements` (module-system snippets are skipped —
they need a multi-file project layout).

`shims/` contains minimal stand-ins for the upstream package's runtime
dependencies (`lodash-es`, `luxon`, `pathe`, `monaco-editor-core`) so the
compiler runs from source without npm access.

## Scope

The Go front end validates the grammar, all local (single-element)
constraints, and single-file cross-element semantics (§8: name resolution,
duplicate relationships, partial injection). Multi-file resolution across
use/reuse imports is exercised only by the upstream compiler in the
cross-check.
