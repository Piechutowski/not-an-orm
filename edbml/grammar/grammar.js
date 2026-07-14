/**
 * Tree-sitter grammar for EDBML (Extended Database Markup Language).
 *
 * Covers the full DBML specification in SPEC.md (not-an-orm repository):
 * Project, Table (settings, alias, columns, legacy flags), TablePartial and
 * ~injection, Enum, Ref (long, short and inline forms, composite endpoints),
 * TableGroup, Note (member, block and sticky forms), Records (top-level and
 * in-table), DiagramView, indexes and checks blocks, use/reuse imports.
 *
 * Design notes, driven by SPEC.md §3:
 *  - DBML is newline-sensitive: columns, enum values, record rows, index and
 *    check lines, project properties, group members and partial injections
 *    are newline-terminated. Newlines are therefore NOT extras; bodies use an
 *    explicit "items separated by one-or-more newlines" shape, which also
 *    accepts a final item flush against the closing brace.
 *  - Keywords are case-insensitive and NOT reserved (§3.5). Each keyword is a
 *    case-insensitive regex aliased to its canonical spelling, with token
 *    precedence over plain identifiers. Consequence (documented limitation):
 *    an unquoted column named exactly `note`, `indexes`, `checks` or
 *    `records` needs quotes ("note") to parse as a column.
 *  - Double quotes delimit identifiers, single quotes delimit strings (§3.4,
 *    §3.6). String and expression contents are separate tokens so editors can
 *    inject other languages (Markdown in notes, SQL in backticks).
 *  - Settings lists are generic `[name, key: value]` (§4.2); which keys are
 *    valid where is the language server's business, which also keeps the
 *    grammar open for EDBML extensions.
 */

/// <reference types="tree-sitter-cli/dsl" />
// @ts-check

/** Case-insensitive keyword, displayed under its canonical spelling. */
function kw(word, precedence = 1) {
  const pattern = word
    .split('')
    .map((c) => (/[a-zA-Z]/.test(c) ? `[${c.toLowerCase()}${c.toUpperCase()}]` : c))
    .join('');
  return alias(token(prec(precedence, new RegExp(pattern))), word);
}

/** One-or-more `rule`s separated by one-or-more newlines, then optional
 * trailing newlines. Callers wrap in optional() for possibly-empty bodies. */
function newlineSep1(rule, $) {
  return seq(rule, repeat(seq(repeat1($._newline), rule)), repeat($._newline));
}

module.exports = grammar({
  name: 'edbml',

  // Newlines are significant; \r is discarded everywhere (§2). Comments may
  // appear anywhere and a line comment does not consume its newline (§3.3).
  extras: ($) => [/[ \t\r]/, $.comment],

  word: ($) => $.identifier,

  conflicts: ($) => [
    // `a.b.c` in a ref endpoint: the target is `a` (alias/table) with column
    // `b.c`? No — the chain is either table.column or schema.table.column;
    // which reading holds is only known once the token after the second name
    // arrives, so both stacks are explored (GLR).
    [$.ref_target],
  ],

  rules: {
    source_file: ($) =>
      seq(
        repeat($._newline),
        optional(newlineSep1($._element, $)),
      ),

    _element: ($) =>
      choice(
        $.project_definition,
        $.table_definition,
        $.table_partial_definition,
        $.enum_definition,
        $.ref_definition,
        $.table_group_definition,
        $.sticky_note_definition,
        $.records_definition,
        $.diagram_view_definition,
        $.import_statement,
      ),

    // ==================== Project (§6.1) ====================

    project_definition: ($) =>
      seq(
        kw('Project'),
        optional(field('name', $._name)),
        '{',
        repeat($._newline),
        optional(newlineSep1(choice($.project_property, $.note_definition), $)),
        '}',
      ),

    project_property: ($) =>
      seq(field('key', $._name), ':', field('value', $.string)),

    // ==================== Table (§6.2, §6.3) ====================

    table_definition: ($) =>
      seq(
        kw('Table'),
        field('name', $.table_name),
        optional(seq(kw('as'), field('alias', alias($._name, $.table_alias)))),
        optional(field('settings', $.settings_list)),
        '{',
        repeat($._newline),
        optional(newlineSep1($._table_item, $)),
        '}',
      ),

    _table_item: ($) =>
      choice(
        $.column_definition,
        $.indexes_block,
        $.checks_block,
        $.note_definition,
        $.partial_injection,
        $.records_block,
      ),

    // A table (or enum, or record target) name, optionally schema-qualified.
    table_name: ($) =>
      seq(
        optional(seq(field('schema', alias($._name, $.schema_name)), '.')),
        field('name', $._name),
      ),

    column_definition: ($) =>
      seq(
        field('name', alias($._name, $.column_name)),
        field('type', $.column_type),
        repeat(field('flag', $.legacy_flag)),
        optional(field('settings', $.settings_list)),
      ),

    column_type: ($) =>
      seq(
        optional(seq(field('schema', alias($._name, $.schema_name)), '.')),
        field('name', alias($._name, $.type_name)),
        optional(field('arguments', $.type_arguments)),
      ),

    type_arguments: ($) =>
      seq(
        token.immediate('('),
        choice($.number, $._name),
        repeat(seq(',', choice($.number, $._name))),
        ')',
      ),

    // Bare `pk` / `unique` between type and settings (§6.3, legacy).
    legacy_flag: () => choice(kw('pk', 2), kw('unique', 2)),

    // ==================== TablePartial (§6.9) ====================

    table_partial_definition: ($) =>
      seq(
        kw('TablePartial'),
        field('name', alias($._name, $.partial_name)),
        optional(field('settings', $.settings_list)),
        '{',
        repeat($._newline),
        optional(newlineSep1($._partial_item, $)),
        '}',
      ),

    _partial_item: ($) =>
      choice($.column_definition, $.indexes_block, $.checks_block, $.note_definition),

    partial_injection: ($) =>
      seq('~', field('name', alias($._name, $.partial_name))),

    // ==================== Enum (§6.8) ====================

    enum_definition: ($) =>
      seq(
        kw('Enum'),
        field('name', $.table_name),
        '{',
        repeat($._newline),
        optional(newlineSep1($.enum_value, $)),
        '}',
      ),

    enum_value: ($) =>
      seq(
        field('name', $._name),
        optional(field('settings', $.settings_list)),
      ),

    // ==================== Ref (§6.7) ====================

    ref_definition: ($) =>
      seq(
        kw('Ref'),
        optional(field('name', alias($._name, $.ref_name))),
        choice(
          // short form: Ref name: a.x > b.y [settings]
          seq(':', $._ref_body),
          // long form: Ref name { a.x > b.y [settings] }
          seq(
            '{',
            repeat($._newline),
            $._ref_body,
            repeat($._newline),
            '}',
          ),
        ),
      ),

    _ref_body: ($) =>
      seq(
        $.ref_relation,
        optional(field('settings', $.settings_list)),
      ),

    ref_relation: ($) =>
      seq(
        field('left', $.ref_endpoint),
        field('operator', $.cardinality),
        field('right', $.ref_endpoint),
      ),

    cardinality: () => choice('<>', '<', '>', '-'),

    // schema.table.column, table.column, alias.column, or the same with a
    // parenthesized column group. The chain is one or more dot-separated
    // names ending in a name or a column group; how many leading segments
    // are schema/table is resolved semantically by the language server.
    ref_endpoint: ($) =>
      seq(
        field('table', $.ref_target),
        '.',
        field('columns', choice(alias($._name, $.column_name), $.column_group)),
      ),

    ref_target: ($) =>
      seq($._name, optional(seq('.', $._name))),

    column_group: ($) =>
      seq(
        '(',
        alias($._name, $.column_name),
        repeat(seq(',', alias($._name, $.column_name))),
        ')',
      ),

    // ==================== indexes / checks (§6.5, §6.6) ====================

    indexes_block: ($) =>
      seq(
        kw('indexes'),
        '{',
        repeat($._newline),
        optional(newlineSep1($.index_definition, $)),
        '}',
      ),

    index_definition: ($) =>
      seq(
        choice($._index_atom, $.composite_index),
        optional(field('settings', $.settings_list)),
      ),

    _index_atom: ($) => choice(alias($._name, $.column_name), $.expression),

    composite_index: ($) =>
      seq('(', $._index_atom, repeat(seq(',', $._index_atom)), ')'),

    checks_block: ($) =>
      seq(
        kw('checks'),
        '{',
        repeat($._newline),
        optional(newlineSep1($.check_definition, $)),
        '}',
      ),

    check_definition: ($) =>
      seq($.expression, optional(field('settings', $.settings_list))),

    // ==================== Note (§6.11) ====================

    // Member form inside Project/Table/TablePartial/TableGroup bodies:
    //   Note: 'text'      or      Note { 'text' }
    note_definition: ($) =>
      seq(
        kw('Note'),
        choice(
          seq(':', field('value', $.string)),
          seq('{', repeat($._newline), field('value', $.string), repeat($._newline), '}'),
        ),
      ),

    // Top-level sticky note: Note name [color: #ff0000] { 'text' }
    sticky_note_definition: ($) =>
      seq(
        kw('Note'),
        field('name', $._name),
        optional(field('settings', $.settings_list)),
        '{',
        repeat($._newline),
        field('value', $.string),
        repeat($._newline),
        '}',
      ),

    // ==================== TableGroup (§6.12) ====================

    table_group_definition: ($) =>
      seq(
        kw('TableGroup'),
        field('name', $._name),
        optional(field('settings', $.settings_list)),
        '{',
        repeat($._newline),
        optional(newlineSep1(choice($.note_definition, alias($.table_name, $.group_member)), $)),
        '}',
      ),

    // ==================== Records (§6.10) ====================

    // Top level: Records users(id, name) { ... }
    records_definition: ($) =>
      seq(
        kw('Records'),
        field('table', $.table_name),
        field('columns', $.records_columns),
        $._records_body,
      ),

    // In-table: Records { ... } or Records (id, name) { ... }
    records_block: ($) =>
      seq(
        kw('Records'),
        optional(field('columns', $.records_columns)),
        $._records_body,
      ),

    records_columns: ($) =>
      seq(
        '(',
        alias($._name, $.column_name),
        repeat(seq(',', alias($._name, $.column_name))),
        ')',
      ),

    _records_body: ($) =>
      seq(
        '{',
        repeat($._newline),
        optional(newlineSep1($.record_row, $)),
        '}',
      ),

    // A row is comma-separated values; a value may be empty (§6.10). A row
    // is either a single value or anything containing at least one comma.
    record_row: ($) =>
      choice(
        $._record_value,
        seq(
          optional($._record_value),
          repeat1(seq(',', optional($._record_value))),
        ),
      ),

    _record_value: ($) =>
      choice(
        $.string,
        $.signed_number,
        $.number,
        $.boolean,
        $.null,
        $.expression,
        $.enum_constant,
      ),

    // ==================== DiagramView (§6.13) ====================

    diagram_view_definition: ($) =>
      seq(
        kw('DiagramView'),
        field('name', $._name),
        '{',
        repeat($._newline),
        optional(newlineSep1($.view_category, $)),
        '}',
      ),

    view_category: ($) =>
      seq(
        field('kind', choice(kw('Tables'), kw('Notes'), kw('TableGroups'), kw('Schemas'))),
        '{',
        repeat($._newline),
        optional(
          choice(
            seq(alias('*', $.wildcard), repeat($._newline)),
            newlineSep1(alias($.table_name, $.view_member), $),
          ),
        ),
        '}',
      ),

    // ==================== Imports (§7) ====================

    import_statement: ($) =>
      seq(
        field('keyword', choice(kw('use'), kw('reuse'))),
        choice(
          alias('*', $.wildcard),
          seq(
            '{',
            repeat($._newline),
            optional(newlineSep1($.import_item, $)),
            '}',
          ),
        ),
        kw('from'),
        field('path', $.string),
      ),

    import_item: ($) =>
      seq(
        field('kind', alias($._name, $.element_kind)),
        field('name', $.table_name),
        optional(seq(kw('as'), field('alias', $._name))),
      ),

    // ==================== Settings (§4.2) ====================

    settings_list: ($) =>
      seq('[', $.setting, repeat(seq(',', $.setting)), ']'),

    setting: ($) =>
      seq(
        field('name', $.setting_name),
        optional(seq(':', field('value', $._setting_value))),
      ),

    // Setting names may be multi-word: `primary key`, `not null` (§4.2).
    setting_name: ($) => prec.right(repeat1($._name)),

    _setting_value: ($) =>
      choice(
        $.string,
        $.signed_number,
        $.number,
        $.boolean,
        $.null,
        $.color,
        $.expression,
        $.enum_constant,
        $.inline_ref,
        alias($.setting_name, $.setting_value_words),
      ),

    // value of a column's `ref:` setting: `> schema.table.column` (§6.7)
    inline_ref: ($) =>
      seq(field('operator', $.cardinality), field('target', $.ref_endpoint)),

    enum_constant: ($) =>
      prec(1, seq(
        optional(seq(field('schema', alias($._name, $.schema_name)), '.')),
        field('enum', alias($._name, $.enum_name)),
        '.',
        field('value', $._name),
      )),

    signed_number: ($) => seq('-', $.number),

    // ==================== Terminals ====================

    _name: ($) => choice($.identifier, $.quoted_identifier),

    // Letters are Unicode L and M categories; identifiers may begin with
    // digits (`2fa_codes`) but an all-digit token is a number (§3.4).
    identifier: () => /[0-9]*[\p{L}\p{M}_][\p{L}\p{M}\p{Nd}_]*/,

    quoted_identifier: () => /"([^"\\\n]|\\[\s\S])*"/,

    // Single-line ('...') and multi-line ('''...''') strings (§3.6, §3.7).
    // Contents are separate tokens so editors can inject Markdown.
    string: ($) =>
      choice(
        seq(
          "'''",
          optional(field('content', alias($._triple_string_content, $.string_content))),
          "'''",
        ),
        seq(
          "'",
          optional(field('content', alias($._single_string_content, $.string_content))),
          token.immediate("'"),
        ),
      ),

    _single_string_content: () =>
      token.immediate(/([^'\\\n]|\\[\s\S])+/),

    _triple_string_content: () =>
      token.immediate(/([^'\\]|\\[\s\S]|'(\\[\s\S]|[^'\\])|''(\\[\s\S]|[^'\\]))+/),

    // Backtick expression: opaque SQL, no escapes, may span lines (§3.12).
    expression: ($) =>
      seq(
        '`',
        optional(field('content', alias(token.immediate(/[^`]+/), $.expression_content))),
        token.immediate('`'),
      ),

    number: () => /[0-9]+(\.[0-9]+)?([eE][+-]?[0-9]+)?/,

    boolean: () => choice(kw('true', 2), kw('false', 2)),

    null: () => kw('null', 2),

    // #rgb or #rrggbb (§3.11)
    color: () => /#[0-9a-fA-F]{3}([0-9a-fA-F]{3})?/,

    comment: () =>
      token(
        choice(
          seq('//', /[^\n]*/),
          seq('/*', /([^*]|\*+[^*/])*\*+/, '/'),
        ),
      ),

    _newline: () => /\n/,
  },
});
