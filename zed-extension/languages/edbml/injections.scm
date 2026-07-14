; DBML notes conventionally contain Markdown (SPEC.md §6.11) — inject Zed's
; built-in Markdown language into every note string.

(note_definition
  value: (string
    content: (string_content) @injection.content)
  (#set! injection.language "markdown"))

(sticky_note_definition
  value: (string
    content: (string_content) @injection.content)
  (#set! injection.language "markdown"))

; [note: '...'] settings on columns, indexes, enum values, groups
(setting
  name: (setting_name) @_key
  value: (string
    content: (string_content) @injection.content)
  (#match? @_key "^[nN][oO][tT][eE]$")
  (#set! injection.language "markdown"))

; backtick expressions are opaque SQL — highlights if a SQL extension is
; installed, harmless otherwise
(expression
  content: (expression_content) @injection.content
  (#set! injection.language "sql"))
