# DBML — Database Markup Language: Language Specification

**Version:** 1.0 (based on `@dbml/parse` reference implementation, holistics/dbml)
**Status:** Normative
**Part of:** [Not an ORM](README.md) — this repository implements the spec
(front end, vet, conformance suite) and generates code from it.

DBML (Database Markup Language) is a declarative, database-agnostic domain-specific
language for defining database schemas: tables, columns, indexes, constraints,
relationships, enumerations, sample data, and documentation metadata.

This document is a complete, formal specification of DBML intended for
implementers of parsers, compilers, and tooling. Every construct is specified
by (1) a grammar production in EBNF, (2) an enumerated list of constraints, and
(3) a minimal example. The collected grammar appears in [Appendix A](#appendix-a-collected-grammar).

---

## Table of Contents

1. [Notation](#1-notation)
   - 1.1 [EBNF](#11-ebnf)
   - 1.2 [Character Notation (U+XXXX)](#12-character-notation-uxxxx)
   - 1.3 [Unicode General Categories](#13-unicode-general-categories)
   - 1.4 [Case Sensitivity and Terminology](#14-case-sensitivity-and-terminology)
2. [Source Text](#2-source-text)
3. [Lexical Structure](#3-lexical-structure)
   - 3.1 [Tokenization](#31-tokenization)
   - 3.2 [Line Structure and Whitespace](#32-line-structure-and-whitespace)
   - 3.3 [Comments](#33-comments)
   - 3.4 [Identifiers](#34-identifiers)
   - 3.5 [Keywords](#35-keywords)
   - 3.6 [String Literals](#36-string-literals)
   - 3.7 [Multi-line String Literals](#37-multi-line-string-literals)
   - 3.8 [Escape Sequences](#38-escape-sequences)
   - 3.9 [Numeric Literals](#39-numeric-literals)
   - 3.10 [Boolean and Null Literals](#310-boolean-and-null-literals)
   - 3.11 [Color Literals](#311-color-literals)
   - 3.12 [Expression Literals](#312-expression-literals)
   - 3.13 [Operators and Punctuation](#313-operators-and-punctuation)
4. [Common Syntactic Forms](#4-common-syntactic-forms)
   - 4.1 [Names and Qualified Names](#41-names-and-qualified-names)
   - 4.2 [Settings Lists](#42-settings-lists)
5. [Program Structure](#5-program-structure)
6. [Element Definitions](#6-element-definitions)
   - 6.1 [Project](#61-project)
   - 6.2 [Table](#62-table)
   - 6.3 [Columns](#63-columns)
   - 6.4 [Default Values](#64-default-values)
   - 6.5 [Indexes](#65-indexes)
   - 6.6 [Checks](#66-checks)
   - 6.7 [Relationships (Ref)](#67-relationships-ref)
   - 6.8 [Enum](#68-enum)
   - 6.9 [TablePartial](#69-tablepartial)
   - 6.10 [Records (Sample Data)](#610-records-sample-data)
   - 6.11 [Notes](#611-notes)
   - 6.12 [TableGroup](#612-tablegroup)
   - 6.13 [DiagramView](#613-diagramview)
7. [Module System](#7-module-system)
8. [Static Semantics](#8-static-semantics)
- [Appendix A: Collected Grammar](#appendix-a-collected-grammar)

---

## 1. Notation

### 1.1 EBNF

The grammar is written in **EBNF** (Extended Backus–Naur Form) following
ISO/IEC 14977. Nonterminal names are lowercase words separated by spaces
(e.g. `table name`); the space is part of the name, not concatenation.

| Notation      | Meaning                                                        |
|---------------|----------------------------------------------------------------|
| `name = … ;`  | Rule: the nonterminal `name` is defined as `…`                 |
| `"text"`, `'text'` | Terminal: the literal character sequence `text`           |
| `,`           | Concatenation: `a, b` means `a` followed by `b`                |
| `\|`          | Alternation: `a \| b` means `a` or `b`                         |
| `[ x ]`       | Option: zero or one occurrence of `x`                          |
| `{ x }`       | Repetition: zero or more occurrences of `x`                    |
| `x, { x }`    | Idiom for one or more occurrences of `x`                       |
| `n * x`       | Exactly `n` occurrences of `x` (e.g. `4 * hex digit`)          |
| `( … )`       | Grouping                                                       |
| `x - y`       | Exception: anything matching `x` that does not match `y`       |
| `? … ?`       | Special sequence: prose description of a match                 |
| `(* … *)`     | Comment inside the grammar                                     |

Note that the square brackets, braces, and parentheses of the *EBNF
meta-language* are distinct from the DBML tokens `[ ]`, `{ }`, `( )`, which
always appear quoted (`"["`, `"{"`, `"("`) when they are part of the language
being defined.

Whitespace between symbols in a production is insignificant unless a
production explicitly references the `newline` or `sp` nonterminals.

### 1.2 Character Notation (U+XXXX)

`U+XXXX` is the standard Unicode notation for a single character (code
point), where `XXXX` is its number in hexadecimal. The characters referenced
by this specification:

| Notation | Character                                            |
|----------|------------------------------------------------------|
| `U+000A` | LINE FEED — the newline character, `\n`              |
| `U+000D` | CARRIAGE RETURN — `\r` (first half of Windows CRLF)  |
| `U+0009` | CHARACTER TABULATION — the tab character, `\t`       |
| `U+0020` | SPACE — the ordinary space character                 |
| `U+0000` | NUL                                                  |
| `U+0008` | BACKSPACE                                            |
| `U+000B` | LINE TABULATION (vertical tab)                       |
| `U+000C` | FORM FEED                                            |

### 1.3 Unicode General Categories

Unicode assigns every character a *General Category*. This specification
uses two of them to define identifiers:

- **Category L (Letter)** — letters of any script: `a`–`z`, `A`–`Z`, but
  also `ż`, `é`, `ß`, `я`, `漢`, `ا`, etc. (It is the union of the
  subcategories Lu uppercase, Ll lowercase, Lt titlecase, Lm modifier,
  Lo other.)
- **Category M (Mark)** — combining marks: characters that attach to the
  preceding character, such as a combining acute accent (U+0301). These are
  included so that accented text in *decomposed* form — where `é` is stored
  as `e` followed by U+0301 — is still a valid identifier.

In regular-expression terms these are `\p{L}` and `\p{M}`. Any Unicode-aware
implementation language provides them; an implementation MUST NOT
approximate category L with ASCII `[a-zA-Z]`.

### 1.4 Case Sensitivity and Terminology

**Case sensitivity.** All DBML *keywords* (terminals spelled with letters in
this grammar, e.g. `"Table"`, `"pk"`, `"not null"`) are matched
**case-insensitively**: `Table`, `table`, and `TABLE` are equivalent.
User-defined names (identifiers) preserve case; implementations MUST treat
them case-sensitively for lookup unless stated otherwise.

**Terminology.** The key words MUST, MUST NOT, SHOULD, and MAY are to be
interpreted as described in RFC 2119.

---

## 2. Source Text

1. A DBML source file is a sequence of Unicode characters, conventionally
   encoded as UTF-8, conventionally using the file extension `.dbml`.
2. A *program* (§5) is the content of one source file. Multiple files are
   related only through the module system (§7).

---

## 3. Lexical Structure

### 3.1 Tokenization

Before parsing, the input is split into *tokens* (identifiers, literals,
operators, punctuation) by a scanner governed by two rules:

1. **Left to right, single pass.** The scanner starts at the first character
   and repeatedly cuts the next token off the front of the remaining input.
   It never backs up to re-tokenize text it has already consumed.
2. **Longest match** (also called *greedy* or *maximal munch*). If, at the
   current position, the upcoming characters could form more than one valid
   token, the scanner always chooses the **longest** one.

Consequences of the longest-match rule:

- `<>` is a single many-to-many operator, never the two tokens `<` `>`.
- `'''` opens a multi-line string, never the empty string `''` followed by `'`.
- `//` begins a comment, never two `/` operators; `>=` is one token, not `>` `=`.
- `user_id2` is one identifier, not the identifier `user_id` followed by the
  number `2` — the scanner keeps consuming while characters can extend the
  current token.

To write two adjacent tokens that would otherwise fuse, separate them with
whitespace.

### 3.2 Line Structure and Whitespace

```ebnf
newline = ? U+000A LINE FEED ? ;
sp      = ? U+0020 SPACE ? | ? U+0009 TAB ? ;
```

1. Carriage return (U+000D) is discarded wherever it appears and produces no
   token; files with Windows (CRLF) line endings are therefore handled
   transparently.
2. DBML is **newline-sensitive**: a line break terminates statements such as
   column definitions, enum values, record rows, and settings-free field
   lines. Productions in this specification reference `newline` explicitly
   wherever it is syntactically significant.
3. Space and tab characters separate tokens and are otherwise insignificant.
4. Indentation is never significant (except inside multi-line strings, §3.7).

### 3.3 Comments

There are **exactly two** comment forms:

```ebnf
comment            = line comment | block comment ;
line comment       = "//", { any char - newline } ;
block comment      = "/*", block comment body, "*/" ;
block comment body = { any char } - ( { any char }, "*/", { any char } ) ;

any char           = ? any Unicode character ? ;
```

1. A line comment extends to, but does not include, the next `newline` (or
   end of file).
2. A block comment may span any number of lines and MUST be terminated by
   `*/` before end of file; an unterminated block comment is a lexical error.
3. Block comments do not nest.
4. Comments are trivia: they may appear between any two tokens and have no
   semantic effect. A line comment does **not** consume the terminating
   newline; the newline retains its statement-terminating role.

```dbml
// single-line comment
/* block
   comment */
```

### 3.4 Identifiers

```ebnf
identifier        = plain identifier | quoted identifier ;

letter            = ? any character of Unicode category L (Letter) ?
                  | ? any character of Unicode category M (Mark) ?
                  | "_" ;
digit             = "0" | "1" | "2" | "3" | "4"
                  | "5" | "6" | "7" | "8" | "9" ;
ident char        = letter | digit ;

plain identifier  = letter, { ident char }
                  | digit, { ident char } ;    (* see constraint 2 *)

quoted identifier = '"', { qi char | escape sequence }, '"' ;
qi char           = any char - ( '"' | "\" | newline ) ;
```

1. A plain identifier is a maximal run of letters, digits, combining marks,
   and underscores.
2. A plain identifier MAY begin with digits, but a token consisting only of
   digits (and at most one `.`) is a numeric literal (§3.9), never an
   identifier. E.g. `2fa_codes` is an identifier; `255` is a number.
3. A quoted identifier (`"double quoted"`) permits any characters except an
   unescaped `"` and a line break, and supports the escape sequences of §3.8.
   Use it for names containing spaces or other special characters, including
   column types with spaces (e.g. `"double precision"`).
4. Plain and quoted identifiers are interchangeable everywhere an
   `identifier` is expected; `users` and `"users"` denote the same name.

### 3.5 Keywords

DBML has **no reserved words**. All keywords are contextual: a keyword such
as `Table` acts as a keyword only in keyword position and remains usable as an
ordinary identifier elsewhere (e.g. a column may be named `table`).

Element-type keywords: `Project`, `Table`, `TablePartial`, `TableGroup`,
`Enum`, `Ref`, `Note`, `Records`, `DiagramView`, `indexes`, `checks`,
`Tables`, `Notes`, `TableGroups`, `Schemas`.
Clause keywords: `as`, `use`, `reuse`, `from`.
Value keywords: `true`, `false`, `null`.
All are case-insensitive (§1.4).

### 3.6 String Literals

```ebnf
string             = single line string | multi line string ;

single line string = "'", { sls char | escape sequence }, "'" ;
sls char           = any char - ( "'" | "\" | newline ) ;
```

1. A single-line string is delimited by single quotes `'` and MUST NOT
   contain an unescaped line break.
2. Escape sequences (§3.8) are interpreted.

### 3.7 Multi-line String Literals

```ebnf
multi line string = "'''", mls body, "'''" ;
mls body          = { ( any char - "\" ) | escape sequence }
                    - ( { any char }, "'''", { any char } ) ;
```

1. Delimited by triple single quotes `'''`; may span any number of lines.
2. Escape sequences (§3.8) are interpreted; escape a literal `'` as `\'` and
   a literal `\` as `\\`.
3. A backslash immediately before a line break is a **line continuation**:
   both the backslash and the line break are removed from the value.
4. **Indentation stripping:** after escape processing, compute the minimum
   number of leading spaces over all non-empty lines; remove exactly that
   many leading spaces from every line. A first line that is empty (the
   common case, where content starts on the line after the opening `'''`)
   and a trailing newline before the closing `'''` are removed.

```dbml
Note: '''
  This is a block string.
  It spans multiple lines.
'''
```

The value of the above is `This is a block string.\nIt spans multiple lines.`

### 3.8 Escape Sequences

Escape sequences apply inside single-line strings, multi-line strings, and
quoted identifiers. They do **not** apply inside expression literals (§3.12)
or comments.

```ebnf
escape sequence = "\", escaped item ;
escaped item    = "t" | "n" | "r" | "0" | "b" | "v" | "f"
                | "\" | "'" | '"' | "`"
                | newline
                | "u", 4 * hex digit
                | any char ;
hex digit       = digit
                | "a" | "b" | "c" | "d" | "e" | "f"
                | "A" | "B" | "C" | "D" | "E" | "F" ;
```

| Sequence | Value                                   |
|----------|-----------------------------------------|
| `\t`     | horizontal tab (U+0009)                 |
| `\n`     | line feed (U+000A)                      |
| `\r`     | carriage return (U+000D)                |
| `\0`     | NUL (U+0000)                            |
| `\b`     | backspace (U+0008)                      |
| `\v`     | vertical tab (U+000B)                   |
| `\f`     | form feed (U+000C)                      |
| `\\`     | backslash                               |
| `\'`     | single quote                            |
| `\"`     | double quote                            |
| `` \` `` | backtick                                |
| `\` + newline | nothing (line continuation)        |
| `\uHHHH` | the code unit U+HHHH (exactly 4 hex digits; fewer is an error) |
| `\c` (any other `c`) | the character `c` itself     |

### 3.9 Numeric Literals

```ebnf
number   = digit, { digit }, [ ".", digit, { digit } ], [ exponent ] ;
exponent = ( "e" | "E" ), [ "+" | "-" ], digit, { digit } ;
```

1. Examples: `42`, `3.14`, `1e2`, `1.5e10`, `3.14e-5`.
2. A leading sign is not part of the literal; a negative value such as
   `-100` in record rows is parsed as prefix operator `-` applied to a number.
3. At most one decimal point is permitted. A digit run followed by further
   letters (e.g. `2fa`) lexes as an identifier (§3.4); a digit run containing
   a dot followed by letters (e.g. `12.3abc`) is a lexical error.

### 3.10 Boolean and Null Literals

```ebnf
boolean = "true" | "false" ;
null    = "null" ;
```

Case-insensitive, as all keywords.

### 3.11 Color Literals

```ebnf
color = "#", ( 3 * hex digit | 6 * hex digit ) ;
```

1. Shorthand `#rgb` or full `#rrggbb` hexadecimal color, e.g. `#3498DB`.
2. Used as the value of `headercolor` and `color` settings.

### 3.12 Expression Literals

```ebnf
expression literal = "`", { any char - "`" }, "`" ;
```

1. Delimited by backticks. The content is an opaque, raw SQL expression:
   backslash is **not** an escape character and there is no way to embed a
   literal backtick.
2. May span multiple lines.
3. Used for computed defaults (`` default: `now()` ``), expression indexes,
   check expressions, and expression values in records.

### 3.13 Operators and Punctuation

```ebnf
rel op = "<>" | "<" | ">" | "-" ;
punct  = "{" | "}" | "[" | "]" | "(" | ")"
       | "," | ":" | ";" | "." | "~" | "*" ;
```

Punctuation roles (uniform across the language):

| Token   | Role                                                        |
|---------|-------------------------------------------------------------|
| `{ }`   | element bodies (block form)                                 |
| `[ ]`   | settings lists                                              |
| `( )`   | composite column lists, type arguments, records column list |
| `:`     | introduces an inline (single-expression) body or a setting value |
| `,`     | separator inside `[ ]`, `( )`, and record rows              |
| `.`     | name qualification (`schema.table.column`)                  |
| `~`     | TablePartial injection prefix                               |
| `*`     | wildcard (module system, DiagramView)                       |
| `< > - <>` | relationship cardinality operators                       |

---

## 4. Common Syntactic Forms

### 4.1 Names and Qualified Names

```ebnf
name          = identifier ;

schema name   = name ;
table name    = [ schema name, "." ], name ;
column path   = [ schema name, "." ], name, ".", name ;
enum constant = name, ".", name ;        (* EnumName.value *)
```

1. `table name` optionally qualifies a table (or enum) with a schema.
2. If the schema qualifier is omitted, the name belongs to the default
   schema `public` (§8.1).
3. `enum constant` references one value of an enum, e.g. `status.active`;
   it is valid as a default value (§6.4) and as a record value (§6.10).

### 4.2 Settings Lists

Settings attach metadata to the construct they follow. A settings list is
always delimited by square brackets and comma-separated:

```ebnf
settings      = "[", setting, { ",", setting }, "]" ;
setting       = setting name, [ ":", setting value ] ;
setting name  = identifier, { sp, { sp }, identifier } ;
setting value = string | number | boolean | null
              | color | expression literal | identifier
              | inline ref value ;
```

1. A setting is either a flag (`pk`, `unique`, `not null`) or a key–value
   pair (`note: 'text'`, `default: 123`).
2. Setting names are case-insensitive and may consist of multiple words
   separated by spaces (`not null`, `primary key`, `no action`).
3. Each element section below enumerates *which* settings are valid there;
   a setting not listed for a construct is invalid in that position.
4. Within one settings list, each setting MUST appear at most once. The
   only exceptions are the column settings `check` and `ref` (§6.3), which
   MAY be repeated. Settings that are synonyms (`pk` / `primary key`) count
   as one setting for this rule.
5. A settings list MUST appear on the same line as (the end of) the
   construct it modifies.

---

## 5. Program Structure

```ebnf
program = { import statement | element } ;

element = project
        | table
        | table partial
        | enum
        | ref element
        | sticky note
        | table group
        | records element
        | diagram view ;
```

1. A program is a sequence of top-level elements and import statements
   (§7), in any order, separated by line breaks.
2. Forward references are permitted: an element may reference another
   element defined later in the file (or imported). DBML is fully
   declarative; declaration order carries no semantics.

---

## 6. Element Definitions

### 6.1 Project

Declares project-level metadata. There MUST be at most one `Project` element
per compiled schema.

```ebnf
project          = "Project", [ name ], "{", project body, "}" ;
project body     = { project property | note def } ;
project property = identifier, ":", string, newline ;
```

1. `name` names the project; it MAY be omitted.
2. Each property is a free-form key with a string value, one per line.
   The conventional well-known key is `database_type` (e.g. `'PostgreSQL'`,
   `'MySQL'`). Implementations MUST accept arbitrary property keys.
3. A `note def` (§6.11) documents the project.

```dbml
Project ecommerce {
  database_type: 'PostgreSQL'
  Note: 'E-commerce database schema'
}
```

### 6.2 Table

```ebnf
table          = "Table", table name, [ table alias ], [ table settings ],
                 "{", table body, "}" ;
table alias    = "as", name ;

table body     = { column
                 | indexes block
                 | checks block
                 | note def
                 | partial injection
                 | records block
                 } ;

table settings = "[", table setting, { ",", table setting }, "]" ;
table setting  = "headercolor", ":", color
               | "note", ":", string ;
```

1. A table MUST contain at least one column, either directly or via an
   injected TablePartial (§6.9).
2. **Alias.** `as name` declares an alternative name usable anywhere the
   table name is (e.g. in `Ref`s). The alias shares the namespace of
   top-level table names and MUST be unique. The alias is not
   schema-qualified.
3. **Settings.** `headercolor` (visualization) and `note` are the only table
   settings.
4. Column names MUST be unique within a table after partial injection (§8.4).

> **Extension (this implementation).** The table setting `model: string`
> pins the Go model name used by `dbml gen go` (by default the
> singularized table name — [decision D10](docs/decisions.md)). The
> setting is accepted by this front end but is not upstream DBML; the
> `vet` rule [`modelname`](vet/RULES.md#modelname) tells you when it is
> needed. The core grammar above is unchanged.

```dbml
Table core.users as U [headercolor: #3498DB] {
  id integer [pk]
  email varchar(255) [not null, unique]
}
```

### 6.3 Columns

```ebnf
column          = name, column type, { legacy flag },
                  [ column settings ], newline ;

column type     = type name, [ "(", type arg, { ",", type arg }, ")" ] ;
type name       = [ schema name, "." ], identifier ;
type arg        = number | identifier ;

legacy flag     = "pk" | "unique" ;

column settings = "[", column setting, { ",", column setting }, "]" ;
column setting  = "primary key" | "pk"
                | "null" | "not null"
                | "unique"
                | "increment"
                | "default", ":", default value
                | "check", ":", expression literal
                | "note", ":", string
                | "ref", ":", inline ref value ;
```

1. **Type.** Any type name is accepted; DBML does not restrict the type
   vocabulary. The type name MUST NOT contain spaces; a type containing
   spaces MUST be written as a quoted identifier (`"double precision"`,
   `"bigint unsigned"`). Parenthesized type arguments (`varchar(255)`,
   `decimal(10,2)`) are preserved verbatim. A type name may be
   schema-qualified to reference an enum (`v2.job_status`).
2. **Nullability.** Absent `not null`, a column is nullable. `null` and
   `not null` are mutually exclusive.
3. **`pk` / `primary key`** are synonyms and mutually exclusive; they mark a
   single-column primary key. A composite primary key MUST be expressed as
   an index with the `pk` setting (§6.5).
4. **`increment`** marks the column auto-increment.
5. **`check`** attaches a single-column check constraint; the setting MAY be
   repeated to attach multiple checks.
6. **`ref`** declares an inline relationship (§6.7). The setting MAY be
   repeated. Inline refs cannot carry names or settings.
7. **Legacy flags.** For backward compatibility, the bare words `pk` and
   `unique` MAY appear between the type and the settings list
   (`id int pk`). New documents SHOULD use the settings list instead.
8. Each column definition is terminated by a line break.

### 6.4 Default Values

```ebnf
default value = [ "-" ], number
              | string
              | boolean
              | null
              | expression literal
              | enum constant ;
```

| Kind          | Example                                    |
|---------------|--------------------------------------------|
| number        | `default: 123`, `default: -100`            |
| string        | `default: 'direct'`                        |
| boolean       | `default: false`, `default: null`          |
| expression    | `` default: `now() - interval '5 days'` `` |
| enum constant | `default: status.active`                   |

A bare identifier that is not `true`/`false`/`null` and not a dotted enum
constant is **not** a valid default value.

### 6.5 Indexes

An `indexes` block may appear inside a `Table` or `TablePartial` body. A
body MAY contain more than one `indexes` block; their contents accumulate.

```ebnf
indexes block  = "indexes", "{", { index }, "}" ;
index          = index key, [ index settings ], newline ;

index key      = index atom
               | "(", index atom, { ",", index atom }, ")" ;
index atom     = name | expression literal ;

index settings = "[", index setting, { ",", index setting }, "]" ;
index setting  = "type", ":", identifier
               | "name", ":", string
               | "unique"
               | "pk"
               | "note", ":", string ;
```

1. An index key is a single column, a single expression, or a parenthesized
   composite of columns and/or expressions.
2. Every `name` in an index key MUST refer to a column of the enclosing
   table (after partial injection).
3. `pk` declares the index a (composite) primary key; `unique` a unique
   index. Combining `pk` and `unique` on one index is redundant but
   permitted.
4. `type` selects the index method. Any identifier is accepted; the
   conventional, portable values are `btree` and `hash`.

```dbml
Table bookings {
  id integer
  country varchar
  booking_date date

  indexes {
    (id, country) [pk]
    booking_date [name: 'idx_booking_date', type: hash]
    (country, `lower(country)`) [unique]
  }
}
```

### 6.6 Checks

A `checks` block declares table-level check constraints (constraints over one
or many columns). It may appear inside a `Table` or `TablePartial` body. A
body MAY contain more than one `checks` block; their contents accumulate.

```ebnf
checks block   = "checks", "{", { check }, "}" ;
check          = expression literal, [ check settings ], newline ;

check settings = "[", check setting, { ",", check setting }, "]" ;
check setting  = "name", ":", string ;
```

1. The expression is opaque SQL (§3.12); DBML does not parse or validate it.
2. `name` names the generated constraint.
3. Single-column checks MAY alternatively be written as a `check:` column
   setting (§6.3).

```dbml
Table users {
  wealth integer
  debt integer

  checks {
    `debt + wealth >= 0` [name: 'chk_positive_money']
  }
}
```

### 6.7 Relationships (Ref)

Relationships define foreign-key constraints. There are three syntactic
forms: **long**, **short**, and **inline**.

```ebnf
ref element      = ref long | ref short ;

ref long         = "Ref", [ name ], "{", ref body, "}" ;
ref short        = "Ref", [ name ], ":", ref body ;

ref body         = ref endpoint, rel op, ref endpoint, [ ref settings ] ;

ref endpoint     = table name, ".", column group ;
column group     = name
                 | "(", name, { ",", name }, ")" ;

rel op           = "<>" | "<" | ">" | "-" ;

inline ref value = rel op, ref endpoint ;
                   (* value of a column's ref: setting *)

ref settings     = "[", ref setting, { ",", ref setting }, "]" ;
ref setting      = "delete", ":", ref action
                 | "update", ":", ref action
                 | "color", ":", color
                 | "inactive" ;
ref action       = "cascade" | "restrict" | "set null"
                 | "set default" | "no action" ;
```

1. **Cardinality operators** (left endpoint *op* right endpoint):
   - `<` one-to-many: `users.id < posts.user_id`
   - `>` many-to-one: `posts.user_id > users.id`
   - `-` one-to-one: `users.id - user_infos.user_id`
   - `<>` many-to-many: `authors.id <> books.id`
2. **Endpoints.** Each endpoint is a (optionally schema-qualified) table
   name — or table alias — followed by one column or a parenthesized column
   group. In a composite relationship both endpoints MUST list the same
   number of columns: `Ref: a.(x, y) > b.(x, y)`.
3. **Foreign-key side.** For `>`, the left endpoint is the foreign key. For
   `<`, the right endpoint is the foreign key. For `-`, the *second* (right)
   endpoint is the foreign key. For an inline ref, the declaring column is
   the foreign key.
4. **Inline form.** `ref: <op> <endpoint>` appears as a column setting
   (§6.3); the declaring column is the implicit left endpoint. Inline refs
   MUST NOT carry a relationship name or settings.
5. **Referential actions.** `delete:` and `update:` correspond to SQL
   `ON DELETE` / `ON UPDATE`.
6. `inactive` and `color` are visualization-only settings (rendered as a
   dotted line / line color); they have no SQL semantics.
7. A many-to-many (`<>`) relationship denotes an implicit junction table on
   SQL export; it MAY equivalently be modeled explicitly with two
   many-to-one relationships.
8. Zero-or-one / zero-or-many cardinality is not written explicitly; it is
   derived from the nullability of the foreign-key column (§6.3.2).
9. Duplicate relationships between the same column sets are invalid,
   regardless of direction: `a.x > b.y` and `b.y < a.x` are the same
   relationship.
10. A `Ref` element declares exactly **one** relationship. The long form is
    a stylistic variant of the short form, not a container for several
    relationships; to declare several, write several `Ref` elements.

```dbml
// short form, with settings
Ref fk_posts_user: posts.user_id > core.users.id [delete: cascade, update: no action]

// long form
Ref {
  merchant_periods.(merchant_id, country_code) > merchants.(id, country_code)
}

// inline form
Table posts {
  user_id integer [ref: > core.users.id]
}
```

### 6.8 Enum

```ebnf
enum          = "Enum", table name, "{", { enum value }, "}" ;
enum value    = name, [ enum settings ], newline ;

enum settings = "[", enum setting, { ",", enum setting }, "]" ;
enum setting  = "note", ":", string ;
```

1. The enum name MAY be schema-qualified; unqualified enums belong to
   `public` (§8.1).
2. An enum MUST contain at least one value. Values MUST be unique within
   the enum.
3. A value containing spaces or special characters MUST be written as a
   quoted identifier: `"Not Yet Set"`.
4. A column references an enum by using the (optionally schema-qualified)
   enum name as its type: `status v2.job_status`.

```dbml
Enum job_status {
  created [note: 'Waiting to be processed']
  running
  done
  failure
}
```

### 6.9 TablePartial

A `TablePartial` declares a reusable fragment of a table body. Tables inject
partials by name.

```ebnf
table partial     = "TablePartial", name, [ table settings ],
                    "{", partial body, "}" ;
partial body      = { column
                    | indexes block
                    | checks block
                    | note def
                    } ;

partial injection = "~", name, newline ;
```

1. Partial names live in their own global (schema-less) namespace and MUST
   be unique.
2. A partial body is a table body without `records` and without nested
   partial injections.
3. An injection `~p` inside a table body replaces itself with the entire
   content of partial `p` at that position. Injection order is source
   order.
4. **Conflict resolution** when the same column, setting, or index is
   defined more than once:
   1. A definition written directly in the table overrides any partial.
   2. Otherwise the **last-injected** partial (in source order) wins.

```dbml
TablePartial base_template [headercolor: #ff0000] {
  id int [pk, not null]
  created_at timestamp [default: `now()`]
}

Table users {
  ~base_template
  name varchar
}
```

### 6.10 Records (Sample Data)

`Records` declares sample rows for a table, in CSV-like syntax. Records may
be declared at top level (naming the table) or inside a table body.

```ebnf
records element = "Records", table name, records columns,
                  "{", { record row }, "}" ;

records block   = "Records", [ records columns ],
                  "{", { record row }, "}" ;

records columns = "(", name, { ",", name }, ")" ;

record row      = record value, { ",", record value }, newline ;
record value    = string
                | [ "-" ], number
                | boolean
                | null
                | expression literal
                | enum constant          (* EnumName.value, §4.1 *)
                | empty ;
empty           = ;                      (* nothing between separators *)
```

1. Top-level records MUST name the target table and MUST list columns.
   In-table records MAY omit the column list; the columns then default to
   all table columns in definition order (after partial injection, §8.4).
2. A table MUST have at most one records block.
3. Each row MUST supply exactly as many values as listed columns.
4. **Value typing.** Each value is checked against the target column's type:
   - Strings are single-quoted. Timestamps/dates are strings in ISO 8601 or
     other unambiguous formats (`'2024-01-15 10:30:00'`, `'2024-01-15'`).
   - Booleans accept, case-insensitively: `true`, `false`, `'true'`,
     `'false'`, `'Y'`, `'N'`, `'T'`, `'F'`, `1`, `0`, `'1'`, `'0'`.
   - Null is `null`, an empty field (nothing between commas), or the empty
     string `''` for non-string columns.
   - Enum values are written as `EnumName.value` or as a string literal of
     the value. A bare identifier (other than `true`/`false`/`null`) is
     **not** a valid record value.
   - Expression literals (backticks) pass through unchecked.

```dbml
Table users {
  id int [pk]
  name varchar
  status job_status

  Records (id, name, status) {
    1, 'Alice', job_status.created
    2, 'Bob', 'running'
    3, , null
  }
}

Records users(id, name, status) {
  4, 'Carol', `default_status()`
}
```

### 6.11 Notes

Notes attach human-readable documentation. There are two positions: a
**note definition** inside an element body, and a top-level **sticky note**
element.

```ebnf
note def      = "Note", ":", string, newline
              | "Note", "{", string, "}" ;

sticky note   = "Note", name, [ note settings ], "{", string, "}" ;

note settings = "[", "color", ":", ( color | "none" ), "]" ;
```

1. A `note def` may appear in the body of `Project`, `Table`,
   `TablePartial`, and `TableGroup`. `Table`, `TablePartial`, and
   `TableGroup` bodies MUST contain at most one note def; uniqueness is
   not enforced in `Project` bodies.
2. Notes on columns, indexes, and enum values use the `note:` setting
   instead (§6.3, §6.5, §6.8).
3. The value is a string; multi-line strings (§3.7) are permitted and
   conventionally contain Markdown.
4. A sticky note is a named, free-standing note (visualization only). Its
   `color` setting accepts a color literal or `none` (no background).

```dbml
Note deployment_reminder [color: #F4D03F] {
  'Remember to run migrations after deploy'
}
```

### 6.12 TableGroup

Groups related tables (documentation/visualization only; no SQL semantics).

```ebnf
table group          = "TableGroup", name, [ table group settings ],
                       "{", table group body, "}" ;
table group body     = { ( table name, newline ) | note def } ;

table group settings = "[", table group setting,
                       { ",", table group setting }, "]" ;
table group setting  = "note", ":", string
                     | "color", ":", color ;
```

1. Each body line names one table (optionally schema-qualified, or an
   alias). Every named table MUST exist.
2. A table MUST NOT belong to more than one TableGroup.

```dbml
TableGroup e_commerce [color: #3498DB, note: 'Core commerce tables'] {
  merchants
  countries
}
```

### 6.13 DiagramView

Declares a named view of the diagram, selecting which items are shown
(visualization only).

```ebnf
diagram view  = "DiagramView", name, "{", { view category }, "}" ;

view category = category kind, "{", category body, "}" ;
category kind = "Tables" | "Notes" | "TableGroups" | "Schemas" ;
category body = "*"
              | { table name, newline } ;
```

1. Each category may appear at most once per view.
2. `*` selects all items of that category; otherwise items are listed one
   per line. An empty body (or omitted category) selects nothing.
3. Listed names MUST refer to existing elements of the corresponding kind.

```dbml
DiagramView sales_view {
  Tables { users
           orders }
  Schemas { core }
  Notes { * }
}
```

---

## 7. Module System

A schema may be split across files. `use` imports elements from another
file; `reuse` additionally re-exports them.

```ebnf
import statement = import kw, import spec, "from", import path ;

import kw        = "use" | "reuse" ;

import spec      = "*"
                 | "{", import item, newline,
                   { import item, newline }, "}" ;

import item      = element kind, table name, [ "as", name ] ;
element kind     = "table" | "enum" | "tablepartial" | "note"
                 | "schema" | "tablegroup" ;

import path      = string ;
```

1. `import path` is a relative path to the source file. The `.dbml`
   extension is optional: `'./base'` and `'./base.dbml'` are equivalent.
2. `use * from <path>` imports every element the target file exports.
3. Selective import names elements by kind and name. `element kind` is
   case-insensitive. Importing:
   - `table` brings the table together with its records and refs;
   - `schema` brings all elements under that schema;
   - `tablegroup` brings the group and all tables in it.
4. `as <alias>` renames the import. Once aliased, only the alias is
   visible; the original name is not.
5. **Visibility.** Elements imported with `use` are visible only in the
   importing file. `use` is **not transitive**: if `a` uses `b` and `b`
   uses `c`, elements of `c` are not visible in `a`.
6. `reuse` imports and additionally re-exports: elements brought in with
   `reuse` are visible to files importing the current file.
7. **Circular imports are permitted.** Because DBML is declarative, files
   may import each other without restriction.
8. Name conflicts among imported and local elements are errors; resolve
   them with selective import and/or aliases.

```dbml
use * from './base'

use {
  table auth.users as u
  schema billing
} from './auth'

reuse * from './common/types'
```

---

## 8. Static Semantics

### 8.1 Schemas

1. Schemas are not declared; a schema exists if and only if at least one
   table or enum names it as qualifier.
2. Every unqualified table, enum, or relationship endpoint belongs to the
   default schema **`public`**.

### 8.2 Namespaces and Uniqueness

Within one compiled schema (after imports):

1. Table names (and aliases) MUST be unique per schema; the alias namespace
   is shared with unqualified table names.
2. Enum names MUST be unique per schema.
3. TablePartial names, TableGroup names, sticky-note names, and DiagramView
   names each form a single global namespace and MUST be unique.
4. Column names MUST be unique within their table; enum values within their
   enum.

### 8.3 Reference Resolution

1. Relationship endpoints, index columns, records columns, TableGroup
   members, and DiagramView members MUST resolve to existing elements.
2. A table alias may be used interchangeably with the table name in
   endpoints and group members.
3. Composite relationship endpoints MUST have equal arity, and referenced
   column lists must match in count and order.

### 8.4 Partial Injection Order

1. Injections are expanded in source order; the effective column order of a
   table is the concatenation of injected and direct columns in source
   order (this order also drives implicit records column lists, §6.10).
2. Conflicts resolve per §6.9.4.

### 8.5 Nullability and Cardinality

1. A column without `null`/`not null` is nullable.
2. A nullable foreign-key column yields zero-or-one / zero-or-many
   cardinality on the FK side; `not null` yields exactly-one.

---

## Appendix A: Collected Grammar

The complete grammar in EBNF (ISO/IEC 14977), collected from the sections
above.

```ebnf
(* ===== 5. Program ===== *)

program              = { import statement | element } ;

element              = project | table | table partial | enum
                     | ref element | sticky note | table group
                     | records element | diagram view ;

(* ===== 7. Module system ===== *)

import statement     = import kw, import spec, "from", import path ;
import kw            = "use" | "reuse" ;
import spec          = "*"
                     | "{", import item, newline,
                       { import item, newline }, "}" ;
import item          = element kind, table name, [ "as", name ] ;
element kind         = "table" | "enum" | "tablepartial" | "note"
                     | "schema" | "tablegroup" ;
import path          = string ;

(* ===== 6.1 Project ===== *)

project              = "Project", [ name ], "{", project body, "}" ;
project body         = { project property | note def } ;
project property     = identifier, ":", string, newline ;

(* ===== 6.2–6.6 Table ===== *)

table                = "Table", table name, [ table alias ],
                       [ table settings ], "{", table body, "}" ;
table alias          = "as", name ;
table body           = { column | indexes block | checks block
                       | note def | partial injection | records block } ;
table settings       = "[", table setting, { ",", table setting }, "]" ;
table setting        = "headercolor", ":", color
                     | "note", ":", string ;

column               = name, column type, { legacy flag },
                       [ column settings ], newline ;
column type          = type name, [ "(", type arg, { ",", type arg }, ")" ] ;
type name            = [ schema name, "." ], identifier ;
type arg             = number | identifier ;
legacy flag          = "pk" | "unique" ;
column settings      = "[", column setting, { ",", column setting }, "]" ;
column setting       = "primary key" | "pk" | "null" | "not null"
                     | "unique" | "increment"
                     | "default", ":", default value
                     | "check", ":", expression literal
                     | "note", ":", string
                     | "ref", ":", inline ref value ;
default value        = [ "-" ], number | string | boolean | null
                     | expression literal | enum constant ;

indexes block        = "indexes", "{", { index }, "}" ;
index                = index key, [ index settings ], newline ;
index key            = index atom
                     | "(", index atom, { ",", index atom }, ")" ;
index atom           = name | expression literal ;
index settings       = "[", index setting, { ",", index setting }, "]" ;
index setting        = "type", ":", identifier
                     | "name", ":", string
                     | "unique" | "pk"
                     | "note", ":", string ;

checks block         = "checks", "{", { check }, "}" ;
check                = expression literal, [ check settings ], newline ;
check settings       = "[", check setting, { ",", check setting }, "]" ;
check setting        = "name", ":", string ;

(* ===== 6.7 Ref ===== *)

ref element          = ref long | ref short ;
ref long             = "Ref", [ name ], "{", ref body, "}" ;
ref short            = "Ref", [ name ], ":", ref body ;
ref body             = ref endpoint, rel op, ref endpoint, [ ref settings ] ;
ref endpoint         = table name, ".", column group ;
column group         = name | "(", name, { ",", name }, ")" ;
rel op               = "<>" | "<" | ">" | "-" ;
inline ref value     = rel op, ref endpoint ;
ref settings         = "[", ref setting, { ",", ref setting }, "]" ;
ref setting          = "delete", ":", ref action
                     | "update", ":", ref action
                     | "color", ":", color
                     | "inactive" ;
ref action           = "cascade" | "restrict" | "set null" | "set default"
                     | "no action" ;

(* ===== 6.8 Enum ===== *)

enum                 = "Enum", table name, "{", { enum value }, "}" ;
enum value           = name, [ enum settings ], newline ;
enum settings        = "[", enum setting, { ",", enum setting }, "]" ;
enum setting         = "note", ":", string ;

(* ===== 6.9 TablePartial ===== *)

table partial        = "TablePartial", name, [ table settings ],
                       "{", partial body, "}" ;
partial body         = { column | indexes block | checks block | note def } ;
partial injection    = "~", name, newline ;

(* ===== 6.10 Records ===== *)

records element      = "Records", table name, records columns,
                       "{", { record row }, "}" ;
records block        = "Records", [ records columns ],
                       "{", { record row }, "}" ;
records columns      = "(", name, { ",", name }, ")" ;
record row           = record value, { ",", record value }, newline ;
record value         = string | [ "-" ], number | boolean | null
                     | expression literal | enum constant | empty ;
empty                = ;

(* ===== 6.11 Notes ===== *)

note def             = "Note", ":", string, newline
                     | "Note", "{", string, "}" ;
sticky note          = "Note", name, [ note settings ], "{", string, "}" ;
note settings        = "[", "color", ":", ( color | "none" ), "]" ;

(* ===== 6.12 TableGroup ===== *)

table group          = "TableGroup", name, [ table group settings ],
                       "{", table group body, "}" ;
table group body     = { ( table name, newline ) | note def } ;
table group settings = "[", table group setting,
                       { ",", table group setting }, "]" ;
table group setting  = "note", ":", string
                     | "color", ":", color ;

(* ===== 6.13 DiagramView ===== *)

diagram view         = "DiagramView", name, "{", { view category }, "}" ;
view category        = category kind, "{", category body, "}" ;
category kind        = "Tables" | "Notes" | "TableGroups" | "Schemas" ;
category body        = "*" | { table name, newline } ;

(* ===== 4. Common forms ===== *)

name                 = identifier ;
schema name          = name ;
table name           = [ schema name, "." ], name ;
column path          = [ schema name, "." ], name, ".", name ;
enum constant        = name, ".", name ;

settings             = "[", setting, { ",", setting }, "]" ;
setting              = setting name, [ ":", setting value ] ;
setting name         = identifier, { sp, { sp }, identifier } ;
setting value        = string | number | boolean | null | color
                     | expression literal | identifier
                     | inline ref value ;

(* ===== 3. Lexical grammar ===== *)

newline              = ? U+000A LINE FEED ? ;
sp                   = ? U+0020 SPACE ? | ? U+0009 TAB ? ;
any char             = ? any Unicode character ? ;

comment              = line comment | block comment ;
line comment         = "//", { any char - newline } ;
block comment        = "/*", block comment body, "*/" ;
block comment body   = { any char } - ( { any char }, "*/", { any char } ) ;

identifier           = plain identifier | quoted identifier ;
letter               = ? any character of Unicode category L (Letter) ?
                     | ? any character of Unicode category M (Mark) ?
                     | "_" ;
digit                = "0" | "1" | "2" | "3" | "4"
                     | "5" | "6" | "7" | "8" | "9" ;
ident char           = letter | digit ;
plain identifier     = letter, { ident char }
                     | digit, { ident char } ;
quoted identifier    = '"', { qi char | escape sequence }, '"' ;
qi char              = any char - ( '"' | "\" | newline ) ;

string               = single line string | multi line string ;
single line string   = "'", { sls char | escape sequence }, "'" ;
sls char             = any char - ( "'" | "\" | newline ) ;
multi line string    = "'''", mls body, "'''" ;
mls body             = { ( any char - "\" ) | escape sequence }
                     - ( { any char }, "'''", { any char } ) ;

escape sequence      = "\", escaped item ;
escaped item         = "t" | "n" | "r" | "0" | "b" | "v" | "f"
                     | "\" | "'" | '"' | "`"
                     | newline
                     | "u", 4 * hex digit
                     | any char ;
hex digit            = digit
                     | "a" | "b" | "c" | "d" | "e" | "f"
                     | "A" | "B" | "C" | "D" | "E" | "F" ;

number               = digit, { digit },
                       [ ".", digit, { digit } ], [ exponent ] ;
exponent             = ( "e" | "E" ), [ "+" | "-" ], digit, { digit } ;

boolean              = "true" | "false" ;
null                 = "null" ;

color                = "#", ( 3 * hex digit | 6 * hex digit ) ;

expression literal   = "`", { any char - "`" }, "`" ;
```

---

## Project Documents

This repository is growing into **Not an ORM** — a DBML-first code generator
for Go + SQLite. The design record lives in [`docs/`](./docs/):
[`not-an-orm.md`](./docs/not-an-orm.md) (the vision),
[`the-model-layer.md`](./docs/the-model-layer.md) (the problem analysis),
[`orm-capability-matrix.md`](./docs/orm-capability-matrix.md) (every ORM
capability with my verdict), and [`decisions.md`](./docs/decisions.md)
(locked design decisions).

---

## Conformance Test Suite

This repository is also an executable companion to the specification.

**Reference front end (Go).** The repository root is an importable Go
module implementing this specification, structured after the Go compiler's
own front end:

| Package    | Role (Go toolchain analogue)                                  |
|------------|---------------------------------------------------------------|
| `token`    | token kinds and source positions (`go/token`)                 |
| `scanner`  | state-function lexer, spec §3 (`go/scanner`)                  |
| `ast`      | typed syntax tree + `Inspect` traversal (`go/ast`)            |
| `parser`   | recursive descent, one method per EBNF production, multi-error recovery (`go/parser`) |
| `check`    | semantic errors + resolved symbol table, spec §4–§8 (`go/types`) |
| `vet`      | pluggable analyzers for legal-but-suspicious DBML (`go vet`)  |
| `gen/golang` | Go model-struct generation with notes as doc comments ([docs](./gen/golang/README.md)) |
| `gen/sqlite` | SQLite DDL + seed-INSERT generation with notes as SQL comments ([docs](./gen/sqlite/README.md)) |
| `diag`     | shared diagnostics, each citing the spec section it enforces  |
| `cmd/dbml` | CLI: `dbml parse | check | vet | gen (go|sqlite) | analyzers` |

**Conformance corpus.** [`conformance/snippets/`](./conformance/) holds one
small `.dbml` file per language feature or constraint, tagged with the spec
section it exercises (`// spec: §…`). Files under `valid/` MUST be accepted
by a conforming implementation; files under `invalid/` MUST be rejected.
The corpus runs as part of the test suite (`go test ./...`).

**Lint rules (informative annex).** [`vet/RULES.md`](./vet/RULES.md)
specifies every vet analyzer — DBML that is *valid* under this
specification but deserves a warning (dead declarations, redundancy,
modeling traps). Each rule links to executable good/bad examples under
`vet/testdata/`, and a consistency test fails the build if the annex, the
analyzer registry and the examples ever disagree. These rules are
informative: they never change what conforming DBML is.

**Cross-check.** `conformance/refcheck/` replays the corpus through the
upstream `@dbml/parse` compiler. The corpus verdicts and the upstream
implementation agree on every checkable snippet; constraints in this
document were corrected against that implementation where the original
prose documentation was stricter or looser than the code.

---

## License and Provenance

This specification is a distillation of the DBML language as documented and
implemented in [holistics/dbml](https://github.com/holistics/dbml)
(the `@dbml/parse` reference implementation). Like the upstream project, it
is licensed under the [Apache License 2.0](./LICENSE).
