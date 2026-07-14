; EDBML highlight queries for Zed. Later patterns win, so generic rules come
; first and specific roles override them.

; ---------- comments and literals ----------

(comment) @comment

(string) @string
(number) @number
(signed_number) @number
(boolean) @boolean
(null) @constant.builtin
(color) @constant

; opaque SQL expressions in backticks
(expression) @string.special

; ---------- keywords ----------

[
  "Project"
  "Table"
  "TablePartial"
  "Enum"
  "Ref"
  "TableGroup"
  "Note"
  "Records"
  "DiagramView"
  "indexes"
  "checks"
  "as"
  "use"
  "reuse"
  "from"
  "Tables"
  "Notes"
  "TableGroups"
  "Schemas"
] @keyword

(element_kind) @keyword

; ---------- operators and punctuation ----------

(cardinality) @operator
"~" @operator
(wildcard) @operator

["{" "}" "(" ")" "[" "]"] @punctuation.bracket
[":" "," "."] @punctuation.delimiter

; ---------- names by role ----------

; schema qualifiers: core.users, v2.job_status
(schema_name) @type @namespace

; tables (definitions, aliases and references)
(table_name name: (_) @type)
(table_alias) @type
(group_member) @type
(view_member) @type
(ref_target (_) @type)

; enums
(enum_definition name: (table_name name: (_) @enum))
(enum_value name: (_) @variant)
(enum_constant enum: (_) @enum value: (_) @variant)

; table partials
(partial_name) @constructor

; columns
(column_name) @property
(column_definition name: (_) @property)
(records_columns (column_name) @property)

; column types
(type_name) @type

; project
(project_definition name: (_) @title)
(project_property key: (_) @property)

; named refs, sticky notes, groups, views
(ref_name) @label
(sticky_note_definition name: (_) @label)
(table_group_definition name: (_) @label)
(diagram_view_definition name: (_) @label)

; ---------- settings ----------

(settings_list (setting name: (_) @attribute))
(setting_value_words) @constant
(legacy_flag) @attribute
