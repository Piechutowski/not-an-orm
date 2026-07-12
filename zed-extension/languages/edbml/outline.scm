(project_definition
  "Project" @context
  name: (_) @name) @item

(table_definition
  "Table" @context
  name: (table_name) @name
  alias: (table_alias)? @context.extra) @item

(table_partial_definition
  "TablePartial" @context
  name: (partial_name) @name) @item

(enum_definition
  "Enum" @context
  name: (table_name) @name) @item

(enum_value
  name: (_) @name) @item

(column_definition
  name: (column_name) @name
  type: (column_type) @context.extra) @item

(indexes_block
  "indexes" @name) @item

(checks_block
  "checks" @name) @item

(ref_definition
  "Ref" @context
  name: (ref_name) @name) @item

(table_group_definition
  "TableGroup" @context
  name: (_) @name) @item

(sticky_note_definition
  "Note" @context
  name: (_) @name) @item

(records_definition
  "Records" @context
  table: (table_name) @name) @item

(diagram_view_definition
  "DiagramView" @context
  name: (_) @name) @item
