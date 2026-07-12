package lsp

import (
	"regexp"
	"strings"

	protocol "github.com/tliron/glsp/protocol_3_16"

	"github.com/Piechutowski/not-an-orm/check"
)

// Completion contexts are derived textually (the file rarely parses while a
// line is being typed) and filled from the last semantic model, which the
// checker returns even for files with errors.

var (
	elementKeywords = []string{
		"Project", "Table", "TablePartial", "Enum", "Ref", "TableGroup",
		"Note", "Records", "DiagramView", "use", "reuse",
	}
	tableBodyKeywords = []string{"indexes", "checks", "Note", "Records"}

	settingsByContext = map[string][]string{
		"column":    {"pk", "primary key", "null", "not null", "unique", "increment", "default: ", "check: ", "note: ", "ref: "},
		"table":     {"headercolor: ", "note: "},
		"index":     {"type: ", "name: ", "unique", "pk", "note: "},
		"check":     {"name: "},
		"ref":       {"delete: ", "update: ", "color: ", "inactive"},
		"enumvalue": {"note: "},
		"sticky":    {"color: "},
		"group":     {"note: ", "color: "},
	}

	builtinTypes = []string{
		"int", "integer", "bigint", "smallint", "serial", "bigserial",
		"varchar", "char", "text", "bool", "boolean", "uuid",
		"timestamp", "timestamptz", "date", "time", "interval",
		"decimal", "numeric", "float", "double", "real",
		"json", "jsonb", "bytea", "blob",
	}

	refActionValues = []string{"cascade", "restrict", "set null", "set default", "no action"}

	partialInjectRE = regexp.MustCompile(`~\s*[\w"]*$`)
	dotChainRE      = regexp.MustCompile(`([\p{L}\p{M}\d_]+|"[^"]*")(\.([\p{L}\p{M}\d_]+|"[^"]*"))?\.$`)
	refValueRE      = regexp.MustCompile(`(?i)\bref\s*:\s*(<>|[<>-])?\s*$`)
	afterCardRE     = regexp.MustCompile(`(<>|[<>-])\s*$`)
	defaultValueRE  = regexp.MustCompile(`(?i)\bdefault\s*:\s*$`)
	actionValueRE   = regexp.MustCompile(`(?i)\b(delete|update)\s*:\s*\w*$`)
	columnShapeRE   = regexp.MustCompile(`^\s*([\p{L}\p{M}\d_]+|"[^"]*")\s+[\p{L}\p{M}\d_".]*$`)
	lineStartRE     = regexp.MustCompile(`^\s*[\p{L}\p{M}\d_"~]*$`)
)

// Complete computes completion items for the position.
func (d *Document) Complete(pos protocol.Position) []protocol.CompletionItem {
	offset := d.FromLSP(pos)
	lineStart := d.lineOffsets[min(int(pos.Line), len(d.lineOffsets)-1)]
	prefix := d.Text[lineStart:offset]
	ctx := d.blockContext(int(pos.Line))

	// ~partial injection
	if (ctx == "table" || ctx == "tablepartial") && partialInjectRE.MatchString(prefix) {
		return d.partialItems()
	}

	inSettings := strings.Count(prefix, "[") > strings.Count(prefix, "]")
	if inSettings {
		return d.completeInSettings(prefix, ctx)
	}

	// endpoint chains: `users.`, `core.orders.`, `status.`
	if m := dotChainRE.FindStringSubmatch(prefix); m != nil {
		return d.completeDotChain(m)
	}

	// ref bodies: `Ref: ` / `Ref name: a.b > ` / inside Ref { }
	trimmed := strings.TrimSpace(prefix)
	if ctx == "ref" || strings.HasPrefix(strings.ToLower(trimmed), "ref") && strings.Contains(trimmed, ":") ||
		afterCardRE.MatchString(prefix) {
		return d.tableItems()
	}

	switch ctx {
	case "":
		if lineStartRE.MatchString(prefix) {
			return keywordItems(elementKeywords)
		}
	case "table", "tablepartial":
		if columnShapeRE.MatchString(prefix) && !lineStartRE.MatchString(prefix) {
			return d.typeItems()
		}
		if lineStartRE.MatchString(prefix) {
			items := keywordItems(tableBodyKeywords)
			items = append(items, d.partialInjectionItems()...)
			return items
		}
	case "tablegroup", "diagramview":
		if lineStartRE.MatchString(prefix) {
			return d.tableItems()
		}
	}
	return nil
}

// blockContext classifies the innermost unclosed block above the line by
// scanning upward and tracking brace balance. "" means top level.
func (d *Document) blockContext(line int) string {
	depth := 0
	for l := line; l >= 0; l-- {
		text := d.lineText(l)
		if l == line {
			// only the part left of the cursor's line matters for openers
			// on the same line; approximate with the whole line minus any
			// trailing close (settings brackets are handled separately).
			text = strings.SplitN(text, "//", 2)[0]
		}
		opens := strings.Count(text, "{")
		closes := strings.Count(text, "}")
		if l == line {
			// a `{` on the cursor line only encloses the cursor if it is
			// before it; being conservative here is fine.
			depth -= 0
		}
		depth += closes - opens
		if depth < 0 {
			head := strings.ToLower(strings.TrimSpace(text))
			switch {
			case strings.HasPrefix(head, "indexes"):
				return "indexes"
			case strings.HasPrefix(head, "checks"):
				return "checks"
			case strings.HasPrefix(head, "tablepartial"):
				return "tablepartial"
			case strings.HasPrefix(head, "tablegroup"):
				return "tablegroup"
			case strings.HasPrefix(head, "table"):
				return "table"
			case strings.HasPrefix(head, "enum"):
				return "enum"
			case strings.HasPrefix(head, "project"):
				return "project"
			case strings.HasPrefix(head, "records"):
				return "records"
			case strings.HasPrefix(head, "ref"):
				return "ref"
			case strings.HasPrefix(head, "diagramview"):
				return "diagramview"
			case strings.HasPrefix(head, "note"):
				return "note"
			default:
				// unnamed opener inside another block: keep scanning with
				// the parent block's depth.
				depth = 0
				continue
			}
		}
	}
	return ""
}

func (d *Document) completeInSettings(prefix, ctx string) []protocol.CompletionItem {
	segment := prefix[strings.LastIndexAny(prefix, "[,")+1:]

	if refValueRE.MatchString(segment) || afterCardRE.MatchString(segment) {
		return d.tableItems()
	}
	if defaultValueRE.MatchString(segment) {
		items := d.enumItems()
		items = append(items, keywordItemsKind([]string{"true", "false", "null"}, protocol.CompletionItemKindValue)...)
		return items
	}
	if actionValueRE.MatchString(segment) {
		return keywordItemsKind(refActionValues, protocol.CompletionItemKindValue)
	}

	kind := "column"
	head := strings.ToLower(strings.TrimSpace(prefix))
	switch {
	case strings.HasPrefix(head, "table") && !strings.HasPrefix(head, "tablegroup"):
		kind = "table"
	case strings.HasPrefix(head, "tablegroup"):
		kind = "group"
	case strings.HasPrefix(head, "ref"):
		kind = "ref"
	case strings.HasPrefix(head, "note"):
		kind = "sticky"
	default:
		switch ctx {
		case "indexes":
			kind = "index"
		case "checks":
			kind = "check"
		case "enum":
			kind = "enumvalue"
		case "ref":
			kind = "ref"
		}
	}
	return keywordItemsKind(settingsByContext[kind], protocol.CompletionItemKindProperty)
}

// completeDotChain resolves `a.` and `a.b.` chains to columns, tables of a
// schema, or enum values.
func (d *Document) completeDotChain(m []string) []protocol.CompletionItem {
	first := unquote(m[1])
	second := ""
	if m[3] != "" {
		second = unquote(m[3])
	}
	var items []protocol.CompletionItem

	if second != "" {
		// schema.table. -> columns of that table
		if ti, ok := d.Index.Tables[first+"."+second]; ok {
			items = append(items, d.columnItems(ti)...)
		}
		return items
	}

	// alias. or table. -> columns
	if ti, ok := d.Index.Tables["alias:"+first]; ok {
		items = append(items, d.columnItems(ti)...)
	}
	if ti, ok := d.Index.Tables["public."+first]; ok {
		items = append(items, d.columnItems(ti)...)
	}
	// enum. -> values
	if ei, ok := d.Index.Enums["public."+first]; ok {
		for _, v := range ei.Decl.Values {
			items = append(items, item(v.Name.Name(), protocol.CompletionItemKindEnumMember, "value of "+ei.Decl.Name.String()))
		}
	}
	// schema. -> tables and enums under it
	for key, ti := range d.Index.Tables {
		if strings.HasPrefix(key, first+".") {
			items = append(items, item(ti.Decl.Name.Base(), protocol.CompletionItemKindStruct, "Table "+key))
		}
	}
	for key, ei := range d.Index.Enums {
		if strings.HasPrefix(key, first+".") {
			items = append(items, item(ei.Decl.Name.Base(), protocol.CompletionItemKindEnum, "Enum "+key))
		}
	}
	return items
}

func (d *Document) tableItems() []protocol.CompletionItem {
	var items []protocol.CompletionItem
	seen := map[string]bool{}
	for _, ti := range d.Info.Tables {
		label := ti.Decl.Name.String()
		if !seen[label] {
			seen[label] = true
			items = append(items, item(label, protocol.CompletionItemKindStruct, "Table"))
		}
		if ti.Alias != "" && !seen[ti.Alias] {
			seen[ti.Alias] = true
			items = append(items, item(ti.Alias, protocol.CompletionItemKindStruct, "alias of "+label))
		}
	}
	return items
}

func (d *Document) columnItems(ti *check.TableInfo) []protocol.CompletionItem {
	var items []protocol.CompletionItem
	for _, cd := range ti.Columns {
		items = append(items, item(cd.Col.Name.Name(), protocol.CompletionItemKindField, cd.Col.Type.String()))
	}
	return items
}

func (d *Document) enumItems() []protocol.CompletionItem {
	var items []protocol.CompletionItem
	for _, ei := range d.Info.Enums {
		items = append(items, item(ei.Decl.Name.String(), protocol.CompletionItemKindEnum, "Enum"))
	}
	return items
}

func (d *Document) typeItems() []protocol.CompletionItem {
	items := keywordItemsKind(builtinTypes, protocol.CompletionItemKindStruct)
	items = append(items, d.enumItems()...)
	return items
}

func (d *Document) partialItems() []protocol.CompletionItem {
	var items []protocol.CompletionItem
	for _, pi := range d.Info.Partials {
		items = append(items, item(pi.Decl.Name.Name(), protocol.CompletionItemKindInterface, "TablePartial"))
	}
	return items
}

func (d *Document) partialInjectionItems() []protocol.CompletionItem {
	var items []protocol.CompletionItem
	for _, pi := range d.Info.Partials {
		items = append(items, item("~"+pi.Decl.Name.Name(), protocol.CompletionItemKindInterface, "inject TablePartial"))
	}
	return items
}

func keywordItems(words []string) []protocol.CompletionItem {
	return keywordItemsKind(words, protocol.CompletionItemKindKeyword)
}

func keywordItemsKind(words []string, kind protocol.CompletionItemKind) []protocol.CompletionItem {
	items := make([]protocol.CompletionItem, 0, len(words))
	for _, w := range words {
		items = append(items, item(w, kind, ""))
	}
	return items
}

func item(label string, kind protocol.CompletionItemKind, detail string) protocol.CompletionItem {
	k := kind
	it := protocol.CompletionItem{Label: label, Kind: &k}
	if detail != "" {
		det := detail
		it.Detail = &det
	}
	return it
}

func unquote(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
