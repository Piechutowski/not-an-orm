# Editor tooling: architecture and patterns

This document explains how EDBML editor support (the Zed extension, the
tree-sitter grammar and the `edbml-ls` language server — D40) is put
together: the three
components, why the system is shaped this way, and the specific patterns each
component uses — so that extending it for EDBML is a matter of following
existing patterns rather than reverse-engineering them.

## 1. The big picture

```
                        ┌────────────────────── Zed process ──────────────────────┐
                        │                                                          │
   tree-sitter-edbml/   │   ┌───────────────┐        ┌───────────────────────┐    │
   grammar.js ──generate──▶ │ parser.c→WASM │        │ extension WASM (glue) │    │
                        │   │  incremental  │        │  zed-extension/src/   │    │
                        │   │  syntax tree  │        │  "how do I launch     │    │
                        │   └──────┬────────┘        │   edbml-ls?"          │    │
                        │          │ tree                └────────┬──────────┘    │
                        │          ▼                              │ spawns        │
                        │   .scm queries                          │               │
                        │   highlights / outline /                │               │
                        │   indents / brackets /                  │               │
                        │   injections                            │               │
                        └─────────────────────────────────────────┼───────────────┘
                                                                  ▼
                                    ┌──────────────────── edbml-ls (Go) ─────────┐
                                    │  JSON-RPC over stdin/stdout (LSP 3.16)     │
                                    │                                            │
                                    │  scanner → parser → check → vet            │
                                    │     │        │        │      │             │
                                    │   tokens    AST     Info   lints           │
                                    │                       │                    │
                                    │              lsp.Index (symbol             │
                                    │              occurrences) powers           │
                                    │              def/refs/rename/hover         │
                                    └────────────────────────────────────────────┘
```

Three independently testable components:

1. **The grammar** (`tree-sitter-edbml/`) — a *syntactic*, error-tolerant,
   incremental parser compiled to WebAssembly and run inside Zed on every
   keystroke. It powers everything visual: colors, outline panel, indent,
   bracket matching, Markdown-in-notes.
2. **The extension** (`zed-extension/`) — a thin package. Declarative files
   (manifest, language config, queries) plus ~70 lines of Rust whose only
   job is telling Zed how to launch the language server.
3. **The language server** (`lsp/`, `cmd/edbml-ls`) — a *semantic*, spec-exact
   analyzer speaking LSP over stdio. It powers everything intelligent:
   diagnostics, completion, hover, navigation, rename.

### Why two parsers?

The grammar and the server both parse EDBML, and that is deliberate — they
have opposite requirements:

| | tree-sitter grammar | Go front end |
|---|---|---|
| Runs | inside the editor, every keystroke | in a separate process, per change notification |
| Must be | fast, incremental, **error-tolerant** (half-typed code still colors) | **correct** — the reference implementation of SPEC.md |
| Output | concrete syntax tree | AST + semantic model (`check.Info`) + diagnostics |
| Knows | shape only | meaning: name resolution, partial injection, ref validity |

Every serious language setup (Rust, Go, TypeScript in any modern editor)
works this way. The risk of the two disagreeing is handled by
cross-validation (§5).

The Go front end itself is layered like the Go toolchain it mirrors, and the
LSP reuses it wholesale rather than reimplementing anything:

```
token    lexical vocabulary (Kind, Token, Position with byte Offset)
scanner  text → tokens                 (lexical errors)
parser   tokens → ast.File            (syntax errors)
check    ast.File → check.Info        (semantic errors; symbol table)
vet      (File, Info) → warnings      (legal but suspicious)
lsp      all of the above → LSP       (this repo's addition)
```

One source of truth: an error squiggle in Zed is *by construction* the same
error `dbml vet` or codegen would report.

## 2. The grammar (`tree-sitter-edbml/`)

Tree-sitter compiles `grammar.js` (a JS DSL) into a table-driven GLR parser
in C (`src/parser.c`, committed, because Zed builds the WASM from it). The
patterns below are the load-bearing decisions; all cite SPEC.md sections.

### Pattern: explicit newlines, not `extras` (§3.2)

DBML is newline-sensitive — columns, enum values, record rows, index lines,
project properties and group members are all *terminated by line breaks*.
The classic mistake (made by the abandoned `tree-sitter-dbml` prototype) is
putting `\n` in `extras` (ignorable whitespace) while also using it as a
terminator: the parser then can't tell two columns on one line from two on
separate lines, and members silently merge.

Here `extras` is only `[ \t\r]` + comments, and every body uses one helper:

```js
function newlineSep1(rule, $) {
  return seq(rule, repeat(seq(repeat1($._newline), rule)), repeat($._newline));
}
// body: '{' repeat(_newline) optional(newlineSep1(item, $)) '}'
```

Consequences that fall out for free:
- `Table t { id int name varchar }` is a syntax error (spec-correct);
- `Table t { id int }` one-liners still parse (the `}` delimits);
- settings lists are same-line only (§4.2) without any extra rule, because
  a newline simply cannot appear inside them;
- a line comment does not consume its newline (the comment token stops
  before `\n`), so `id int // hi` still terminates the column.

### Pattern: case-insensitive contextual keywords (§3.5)

DBML keywords are case-insensitive (`TABLE` ≡ `table`) and **not reserved**
(a column may be named `table`). Each keyword is a generated regex aliased
to its canonical spelling:

```js
function kw(word, precedence = 1) {
  // "Table" -> /[tT][aA][bB][lL][eE]/, displayed as "Table"
  return alias(token(prec(precedence, new RegExp(pattern))), word);
}
```

The alias makes queries stable (`"Table" @keyword` matches every casing),
and tree-sitter's *context-aware lexing* provides the "contextual" part: the
lexer only attempts tokens that are valid in the current parse state, so
`table` in column-name position lexes as an identifier. The one place this
breaks down: at the start of a table-body line, both a column name and a
block keyword are valid, so an unquoted column literally named `indexes`,
`checks`, `note` or `records` lexes as the keyword. Documented trade-off
(see Known limitations below); quoting (`"indexes"`) opts out, because quoted identifiers
never act as keywords (§1.4).

### Pattern: split string tokens for language injection (§3.6–3.7)

Strings are not single tokens. The delimiters and the content are separate:

```js
seq("'''", optional(alias($._triple_string_content, $.string_content)), "'''")
```

`token.immediate` keeps whitespace/comments from sneaking between them. The
payoff is that `(string_content)` is a *node with its own range*, which is
exactly what `injections.scm` needs to hand note bodies to Zed's Markdown
grammar (DBML notes are conventionally Markdown, §6.11) and backtick
expressions to SQL. A single-token string would leave nothing to inject
into. Same trick for `expression_content`.

### Pattern: GLR conflict for reference chains (§6.7)

`a.b.c` in a ref endpoint is `table.column` under schema `a`… or is `a.b`
the schema-qualified table? LR(1) cannot decide when it sees the first dot,
so `ref_target` is declared in `conflicts` and the GLR engine keeps both
stacks alive; the token *after* the chain kills the wrong one. This is the
precise spot where the old prototype grammar broke (it committed early via
precedence and could no longer parse two-part refs at all).

### Pattern: generic settings, open vocabulary (§4.2)

`settings_list` parses *any* `[name, multi word name: value]` — it does not
know that `pk` is a column setting and `headercolor` a table setting. Two
reasons:

1. **Spec fidelity**: setting names are multi-word identifiers (`not null`,
   `primary key`), values include colors, expressions, inline refs, enum
   constants and multi-word words (`no action`) — the *shape* is uniform;
   which keys are legal where is semantics, and the checker (`check/settings.go`)
   already enforces it with positions the LSP surfaces.
2. **EDBML headroom**: new settings (like the reserved `model:`) parse
   without touching the grammar. Grammar releases stay decoupled from
   language-feature releases.

The same philosophy applies to types (`column_type` accepts any name — the
open type vocabulary of §6.3) and project properties (arbitrary keys, §6.1).

### Pattern: corpus tests + differential validation

`test/corpus/*.txt` pins 37 input → expected-tree cases (including two
`:error` cases asserting *invalidity*). Expectations were generated with
`tree-sitter test --update` and then reviewed — the tool pins whatever the
parser does, so the review step is where correctness lives.

Beyond the corpus, the grammar is validated *differentially* against the Go
reference parser: `examples/kitchen_sink.dbml` (every construct in the spec)
must produce zero tree-sitter ERROR nodes **and** zero front-end
diagnostics, and every known-valid file in `vet/testdata/` must parse clean.
When the two parsers disagree, the front end is right by definition.

## 3. The Zed extension (`zed-extension/`)

### Anatomy

```
extension.toml               manifest: ids, [grammars.edbml], [language_servers.edbml-ls]
Cargo.toml, src/lib.rs       Rust glue, compiled BY ZED to WASM at install time
languages/edbml/
  config.toml                language registration: name, suffixes, comments, brackets
  highlights.scm             syntax tree → theme captures
  outline.scm                syntax tree → outline panel / breadcrumbs
  indents.scm                auto-indent regions
  brackets.scm               bracket pair matching
  injections.scm             Markdown into notes, SQL into backticks
```

Only language-server extensions need Rust at all; a pure highlighting
extension is entirely declarative. The glue implements one essential trait
method:

```rust
fn language_server_command(...) -> Result<zed::Command>
```

with a two-step resolution chain — user setting `lsp.edbml-ls.binary.path`
first, then `worktree.which("edbml-ls")` — and two optional passthrough
methods that forward `initialization_options` / workspace `settings` from
Zed's `settings.json` to the server. There is deliberately no
download-from-releases step (the Odin extension shows that pattern): this
extension is local-first, and `scripts/install-ls.sh` is the installer.

### Pattern: the local grammar mirror (`scripts/sync-grammar.sh`)

Zed loads grammars from a **git repository** (`repository` + `commit` in
`extension.toml`) — even for dev extensions; the only concession is that
`file://` URLs are allowed. Vendoring a nested git repo inside this repo
would create gitlink/submodule traps, so the script:

1. regenerates `src/parser.c` if a tree-sitter CLI is present (the committed
   one keeps working when it isn't);
2. mirrors `tree-sitter-edbml/` into `~/.cache/edbml/tree-sitter-edbml-git`
   (a plain `tar` copy — no rsync dependency) and commits there;
3. rewrites `[grammars.edbml]` with the mirror's `file://` URL and fresh SHA.

The rewritten lines are machine-local by design — the committed
`extension.toml` carries a placeholder, and the script is the source of
truth. This is the price of Zed's "grammar = git repo" contract; everything
else about the dev loop is plain **Rebuild** in the extensions page.

### Pattern: capture-name discipline in queries

Zed themes style a fixed capture vocabulary (`@keyword`, `@type`,
`@property`, `@enum`, `@variant`, `@attribute`, …) — Helix/Neovim names like
`@constant.numeric` silently do nothing (the community DBML extension made
exactly this mistake). The queries here map *roles*, not node names:

- table names/aliases/references → `@type`; schema qualifiers → `@namespace`
  with `@type` fallback (multiple captures resolve right-to-left)
- columns → `@property`, enum values → `@variant`, enums → `@enum`
- setting names → `@attribute`, keyword-ish setting values → `@constant`
- partials → `@constructor`, named refs/groups/notes → `@label`

Later patterns win in a query file, so generic rules go first, specific
overrides after — the file is ordered that way on purpose.

## 4. The language server (`lsp/`)

### Protocol shape

`edbml-ls` speaks LSP 3.16 over stdio via `tliron/glsp`, which supplies the
JSON-RPC plumbing and typed protocol structs; the entire wiring is one
`protocol.Handler` literal in `lsp/server.go`. Document sync is **full-text**
(`TextDocumentSyncKindFull`): schema files are small, and full sync makes
the server stateless per edit — no incremental-patch bookkeeping to get
wrong. (The `didChange` handler still applies ranged patches defensively if
a client sends them.)

### The per-edit pipeline (`Document.Update`)

Every open/change runs the whole front end — it is fast enough that there
is no cache to invalidate:

```
text
 └─ parser.ParseFile ──▶ ast.File     + syntax diagnostics
     └─ check.File   ──▶ check.Info   + semantic diagnostics
         └─ vet.Run  ──▶               style warnings   (only if error-free)
             └─ BuildIndex ──▶ lsp.Index (symbol occurrences)
                 └─ publishDiagnostics (push, not pull)
```

Two deliberate choices:

- **vet runs only on error-free files.** Style advice stacked on top of
  hard errors is noise while typing; it reappears the moment the file
  checks clean. The `modelname` analyzer is additionally excluded outright
  because `[model:]` is EDBML-only (`activeAnalyzers`).
- **`check.File` returns a usable `Info` even for broken files**, so
  completion and hover keep working from the last coherent model while the
  user is mid-keystroke.

### Pattern: the occurrence index (`lsp/index.go`)

Definition, references, rename and hover are all the same question —
"which symbol is under the cursor, and where else does it appear?" — so
they share one data structure built once per edit:

```go
type SymbolID struct{ Kind SymKind; Container, Name string }
type Occurrence struct{ ID SymbolID; Ident *ast.Ident; IsDecl bool }
```

`BuildIndex` walks the AST *with the semantic model in hand*, resolving
every name the way the checker does (canonical `schema.name` keys, `public`
default schema, aliases in their own namespace) and recording an occurrence
per identifier token. Features then reduce to list operations:

- **definition** = the `IsDecl` occurrence of the ID under the cursor
- **references** = all occurrences of that ID
- **hover** = render the `check.Info` entry the ID points at (notes emitted
  as raw Markdown — they *are* Markdown)
- **rename** = text edits over occurrences (with a twist, below)

Two design decisions worth knowing:

1. **Container-scoped columns.** A column's ID is
   `{SymColumn, "table:public.users", "id"}` — *unless* the column comes
   from a TablePartial, in which case every table that injects it shares
   `{SymColumn, "partial:base", "id"}`. Go-to-definition from `users.id`
   therefore correctly lands inside the partial, and renaming it updates
   the partial once plus every reference through every injecting table —
   which is what the schema author actually means. This mirrors the
   checker's own conflict-resolution model (§6.9.4), reachable through
   `check.ColumnDef.Partial`.

2. **Spelling-aware rename.** A table is reachable by name *and* alias
   (`users` and `U` in the same file). All those occurrences share one
   SymbolID — references finds them all — but rename only rewrites
   occurrences whose identifier text matches the one under the cursor.
   Renaming `users` leaves `U.id` alone; renaming `U` leaves `users` alone.
   New names are validated against the identifier grammar (§3.4) before any
   edit is produced.

### Pattern: position encoding at the boundary

The front end counts 1-based lines and 1-based *rune* columns (plus byte
offsets); LSP counts 0-based lines and 0-based *UTF-16 code units* — three
different coordinate systems. All conversion lives in two functions on
`Document` (`ToLSP`, `FromLSP`) that re-derive the mapping from the line's
actual text, so multi-byte and surrogate-pair characters (`𝔘`) stay correct
(pinned by `TestUTF16Conversion`). Nothing else in the server ever does
position arithmetic.

Diagnostics carry only a start position in the front end; `diagnosticRange`
widens each to the identifier-ish token at that offset so editors have
something visible to underline.

### Pattern: textual context + semantic fill for completion

Completion cannot rely on the AST — the current line usually doesn't parse
*while it is being typed*. So `lsp/completion.go` splits the problem:

- **Where am I?** is answered textually: regexes over the line prefix
  (`~…`, inside `[…]`, after `ref: >`, after `users.`) plus `blockContext`,
  a tiny upward scanner that walks lines toward the file start tracking
  `{`/`}` balance until it finds the unclosed opener and classifies it by
  its leading keyword (`table`, `enum`, `indexes`, …). Cheap, tolerant of
  arbitrary breakage below/above, no parse required.
- **What are the candidates?** is answered semantically, from the last
  `check.Info`: real table/alias names, real columns of the resolved table,
  real enum values, real partials — plus static tables (per-construct
  settings straight from §4.2, built-in SQL types, ref actions).

The result behaves like it understands the file even when the file is
momentarily nonsense.

### What the server deliberately does not do (yet)

Single-file analysis only: `use`/`reuse` imports parse, and the checker
already relaxes unresolved-name errors for importing files (§7), but
definitions do not resolve across files. The extension point is clear —
`Server.docs` already holds every open document; a workspace-level index
keyed by import paths is the natural next layer (see Known limitations).

## 5. Testing strategy

Layered, one suite per component boundary:

| Layer | What | Where |
|---|---|---|
| Grammar unit | 37 corpus cases incl. `:error` cases | `tree-sitter-edbml/test/corpus/` |
| Grammar ↔ front end | kitchen-sink + vet testdata parse clean under **both** parsers | `examples/`, cross-checked manually/CI |
| Front end unit | the scanner/parser/check/vet suites (incl. `//WANT` marker corpus, RULES.md ↔ registry ↔ testdata pinning) | `*/_test.go` |
| LSP unit | index, rename semantics, hover content, completion contexts, UTF-16 | `lsp/lsp_test.go` |
| LSP integration | real JSON-RPC session against the built binary: initialize → didOpen → diagnostics → hover/def/refs/rename/symbols/completion | scripted (see git history: `lsp_smoke.py`) |

`check/conformance_test.go` runs the spec's snippet corpus
(`conformance/snippets/`) through the same front end the LSP wraps. The
grammar is differentially validated against the same corpus: every valid
snippet must produce zero tree-sitter ERROR nodes — currently 39/40, the
exception being `34_no_reserved_words.dbml`, which is precisely the
keyword-column limitation documented below.

## 6. Extending for EDBML — the recipes

EDBML will be a significant superset of DBML. The system was shaped so each
kind of extension has a beaten path:

**New setting on an existing construct** (e.g. enabling `model:`):
grammar — nothing (generic settings). Checker — add a row to the setting
whitelist in `check/settings.go`. LSP — add the key to `settingsByContext`
in `lsp/completion.go`; re-enable the `modelname` analyzer in
`lsp/document.go` if it's the `model:` setting itself.

**New element type** (a new top-level `Thing name { … }`):
1. Grammar: add a `thing_definition` rule following an existing element's
   shape (`kw('Thing')`, `newlineSep1` body), add to `_element`, regenerate,
   add corpus cases.
2. Queries: a `@keyword` line in `highlights.scm`, an `@item` block in
   `outline.scm`.
3. Front end: AST node (+ `walk.go` case), parser production, checker rules
   with a `spec/§` code, optional vet analyzers (one file + `register`).
4. LSP: if it declares names — declarations/references in `BuildIndex`,
   a hover renderer, a `DocumentSymbols` case, completion keywords. Each
   is an additive switch case; nothing central changes.

**New lint**: one file in `vet/` with an `*Analyzer` + `register()`, a
`### name` section in `vet/RULES.md`, a `testdata/*.dbml` with `//WANT`
markers — `docs_test.go` fails the build until all three agree. The LSP
picks it up automatically.

**Grammar change of any kind**: edit `grammar.js` → `tree-sitter generate`
→ `tree-sitter test` (update corpus deliberately) → verify
`kitchen_sink.dbml` still parses clean under both parsers →
`./scripts/sync-grammar.sh` → Rebuild in Zed.

## 7. Decision log

- **Fresh grammar over extending `dynamotn/tree-sitter-dbml`** — the
  prototype couldn't parse two-part refs or inline refs at all, had no
  partials, no fields, no per-construct nodes, a 5/5-failing corpus; fixing
  it meant redesigning tokenization anyway.
- **Go for the server** — the reference front end is Go; wrapping it gives
  spec-exact diagnostics for free and one implementation to maintain.
- **`tliron/glsp` over `go.lsp.dev`** — maintained, complete 3.16 types,
  handler-struct wiring with no codegen; the alternative is semi-dormant.
- **Full-text sync over incremental** — schema files are small; statelessness
  beats patch bookkeeping.
- **Local-first distribution** — dev extension + PATH-resolved binary; no
  marketplace, no binary downloads. Publishing later only adds steps
  (public grammar repo, `zed-industries/extensions` PR), it changes nothing
  structural.
- **`.dbml` and `.edbml` both registered** — EDBML is a superset; plain DBML
  files are valid EDBML.

## Known limitations (documented trade-offs, not bugs)

- **`[model:]` is parsed but not assisted.** The grammar's generic settings
  rule parses it, but the language server does not run the `vet/modelname`
  analyzer (`lsp/document.go: activeAnalyzers`) — the editor should not
  push authors toward an extension setting the editor tooling itself does
  not yet understand end to end. Re-enable alongside full EDBML support.
- **Grammar: unquoted columns named like block keywords.** DBML has no
  reserved words (§3.5), but the grammar gives `indexes`, `checks`, `note`
  and `records` keyword precedence inside table bodies, so a column
  literally named `indexes` must be written `"indexes"`. The front end
  (spec-exact) accepts both.
- **LSP: single-file analysis.** `use`/`reuse` imports parse and the
  checker relaxes unresolved-name errors accordingly (§7), but names do
  not yet resolve across files.
- **Grammar: exotic string edges.** Triple-quoted strings whose content
  ends in a quote immediately before the closing `'''` may tokenize
  differently from the spec's maximal-munch rule; the front end handles
  them correctly.
