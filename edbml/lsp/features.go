package lsp

import (
	"fmt"
	"strings"

	protocol "github.com/tliron/glsp/protocol_3_16"

	"github.com/Piechutowski/not-an-orm/edbml/ast"
	"github.com/Piechutowski/not-an-orm/edbml/check"
	"github.com/Piechutowski/not-an-orm/edbml/diag"
)

// ---------------------------------------------------------------------------
// Diagnostics

// LSPDiagnostics converts the front end's diagnostics for publishing.
func (d *Document) LSPDiagnostics() []protocol.Diagnostic {
	out := make([]protocol.Diagnostic, 0, len(d.Diags))
	for _, dg := range d.Diags {
		severity := protocol.DiagnosticSeverityError
		if dg.Severity == diag.Warning {
			severity = protocol.DiagnosticSeverityWarning
		}
		sev := severity
		code := dg.Code
		source := "nao"
		out = append(out, protocol.Diagnostic{
			Range:    d.diagnosticRange(dg.Pos),
			Severity: &sev,
			Code:     &protocol.IntegerOrString{Value: code},
			Source:   &source,
			Message:  dg.Msg,
		})
	}
	return out
}

// ---------------------------------------------------------------------------
// Document symbols (outline)

// DocumentSymbols builds the outline tree.
func (d *Document) DocumentSymbols() []protocol.DocumentSymbol {
	var out []protocol.DocumentSymbol
	for _, decl := range d.File.Decls {
		switch n := decl.(type) {
		case *ast.Project:
			name := "Project"
			var sel ast.Node = n
			if n.Name != nil {
				name = n.Name.Name()
				sel = n.Name
			}
			out = append(out, d.symbol(name, "Project", protocol.SymbolKindNamespace, n, sel, nil))
		case *ast.Table:
			detail := "Table"
			if n.Alias != nil {
				detail = "Table (as " + n.Alias.Name() + ")"
			}
			out = append(out, d.symbol(n.Name.String(), detail, protocol.SymbolKindStruct, n, n.Name, d.tableChildren(n.Body)))
		case *ast.TablePartial:
			out = append(out, d.symbol(n.Name.Name(), "TablePartial", protocol.SymbolKindInterface, n, n.Name, d.tableChildren(n.Body)))
		case *ast.Enum:
			var kids []protocol.DocumentSymbol
			for _, v := range n.Values {
				kids = append(kids, d.symbol(v.Name.Name(), "", protocol.SymbolKindEnumMember, v, v.Name, nil))
			}
			out = append(out, d.symbol(n.Name.String(), "Enum", protocol.SymbolKindEnum, n, n.Name, kids))
		case *ast.Ref:
			name := refLabel(n)
			var sel ast.Node = n
			if n.Name != nil {
				sel = n.Name
			}
			out = append(out, d.symbol(name, "Ref", protocol.SymbolKindKey, n, sel, nil))
		case *ast.TableGroup:
			out = append(out, d.symbol(n.Name.Name(), "TableGroup", protocol.SymbolKindNamespace, n, n.Name, nil))
		case *ast.StickyNote:
			out = append(out, d.symbol(n.Name.Name(), "Note", protocol.SymbolKindString, n, n.Name, nil))
		case *ast.Records:
			label := "Records"
			if n.Table != nil {
				label = "Records " + n.Table.String()
			}
			out = append(out, d.symbol(label, "", protocol.SymbolKindArray, n, n, nil))
		case *ast.DiagramView:
			out = append(out, d.symbol(n.Name.Name(), "DiagramView", protocol.SymbolKindObject, n, n.Name, nil))
		}
	}
	return out
}

func (d *Document) tableChildren(body []ast.TableItem) []protocol.DocumentSymbol {
	var kids []protocol.DocumentSymbol
	for _, item := range body {
		switch n := item.(type) {
		case *ast.Column:
			kids = append(kids, d.symbol(n.Name.Name(), n.Type.String(), protocol.SymbolKindField, n, n.Name, nil))
		case *ast.PartialRef:
			kids = append(kids, d.symbol("~"+n.Name.Name(), "injected partial", protocol.SymbolKindInterface, n, n.Name, nil))
		case *ast.IndexesBlock:
			kids = append(kids, d.symbol("indexes", "", protocol.SymbolKindProperty, n, n, nil))
		case *ast.ChecksBlock:
			kids = append(kids, d.symbol("checks", "", protocol.SymbolKindProperty, n, n, nil))
		}
	}
	return kids
}

func (d *Document) symbol(name, detail string, kind protocol.SymbolKind, full, sel ast.Node, children []protocol.DocumentSymbol) protocol.DocumentSymbol {
	s := protocol.DocumentSymbol{
		Name:           name,
		Kind:           kind,
		Range:          d.RangeOf(full),
		SelectionRange: d.RangeOf(sel),
		Children:       children,
	}
	if detail != "" {
		s.Detail = &detail
	}
	return s
}

func refLabel(n *ast.Ref) string {
	if n.Name != nil {
		return n.Name.Name()
	}
	left, right := "?", "?"
	if n.Left != nil {
		left = n.Left.Table.String() + "." + strings.Join(identNames(n.Left.Columns), ",")
	}
	if n.Right != nil {
		right = n.Right.Table.String() + "." + strings.Join(identNames(n.Right.Columns), ",")
	}
	return fmt.Sprintf("%s %s %s", left, n.OpTok.Text, right)
}

func identNames(ids []*ast.Ident) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = id.Name()
	}
	return out
}

// ---------------------------------------------------------------------------
// Definition / references / rename

// Definition returns the location of the declaration of the symbol at pos.
func (d *Document) Definition(pos protocol.Position) *protocol.Location {
	occ := d.Index.At(d.FromLSP(pos))
	if occ == nil {
		return nil
	}
	decl := d.Index.Decl(occ.ID)
	if decl == nil {
		return nil
	}
	return &protocol.Location{URI: d.URI, Range: d.tokenRange(decl.Tok)}
}

// References returns every occurrence of the symbol at pos.
func (d *Document) References(pos protocol.Position, includeDecl bool) []protocol.Location {
	occ := d.Index.At(d.FromLSP(pos))
	if occ == nil {
		return nil
	}
	var out []protocol.Location
	for _, o := range d.Index.OccurrencesOf(occ.ID) {
		if o.IsDecl && !includeDecl {
			continue
		}
		out = append(out, protocol.Location{URI: d.URI, Range: d.tokenRange(o.Ident.Tok)})
	}
	return out
}

// Rename renames the symbol at pos. Only occurrences spelled like the ident
// under the cursor are rewritten: renaming a table by its name leaves alias
// usages alone, and renaming the alias leaves the table name alone.
func (d *Document) Rename(pos protocol.Position, newName string) (*protocol.WorkspaceEdit, error) {
	if !validIdentifier(newName) {
		return nil, fmt.Errorf("%q is not a valid EDBML identifier", newName)
	}
	occ := d.Index.At(d.FromLSP(pos))
	if occ == nil {
		return nil, fmt.Errorf("nothing renameable at this position")
	}
	spelling := occ.Ident.Name()
	var edits []protocol.TextEdit
	for _, o := range d.Index.OccurrencesOf(occ.ID) {
		if o.Ident.Name() != spelling {
			continue
		}
		edits = append(edits, protocol.TextEdit{Range: d.tokenRange(o.Ident.Tok), NewText: newName})
	}
	if len(edits) == 0 {
		return nil, fmt.Errorf("nothing renameable at this position")
	}
	return &protocol.WorkspaceEdit{
		Changes: map[protocol.DocumentUri][]protocol.TextEdit{d.URI: edits},
	}, nil
}

// validIdentifier reports whether s can be written as a plain identifier
// (spec §3.4): letters, marks, digits and _, not all digits.
func validIdentifier(s string) bool {
	if s == "" {
		return false
	}
	hasLetter := false
	for _, r := range s {
		switch {
		case r == '_':
			hasLetter = true
		case r >= '0' && r <= '9':
		case isLetter(r):
			hasLetter = true
		default:
			return false
		}
	}
	return hasLetter
}

func isLetter(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r > 0x7f
}

// ---------------------------------------------------------------------------
// Hover

// Hover renders documentation for the symbol at pos.
func (d *Document) Hover(pos protocol.Position) *protocol.Hover {
	occ := d.Index.At(d.FromLSP(pos))
	if occ == nil {
		return nil
	}
	var md string
	switch occ.ID.Kind {
	case SymTable:
		md = d.hoverTable(occ.ID.Name)
	case SymEnum:
		md = d.hoverEnum(occ.ID.Name)
	case SymPartial:
		md = d.hoverPartial(occ.ID.Name)
	case SymColumn:
		md = d.hoverColumn(occ.ID)
	case SymEnumValue:
		md = d.hoverEnumValue(occ.ID)
	}
	if md == "" {
		return nil
	}
	rng := d.tokenRange(occ.Ident.Tok)
	return &protocol.Hover{
		Contents: protocol.MarkupContent{Kind: protocol.MarkupKindMarkdown, Value: md},
		Range:    &rng,
	}
}

func (d *Document) hoverTable(key string) string {
	ti, ok := d.Index.Tables[key]
	if !ok {
		return ""
	}
	var b strings.Builder
	header := "Table " + ti.Decl.Name.String()
	if ti.Alias != "" {
		header += " as " + ti.Alias
	}
	fmt.Fprintf(&b, "```edbml\n%s\n```\n", header)
	for _, cd := range ti.Columns {
		line := "- `" + cd.Col.Name.Name() + " " + cd.Col.Type.String() + "`" + settingsSuffix(cd.Col.Settings, cd.Col.LegacyFlags)
		if cd.Partial != nil {
			line += " *(from ~" + cd.Partial.Name.Name() + ")*"
		}
		b.WriteString(line + "\n")
	}
	if note := tableNote(ti.Decl); note != "" {
		b.WriteString("\n---\n" + note + "\n")
	}
	return b.String()
}

func (d *Document) hoverEnum(key string) string {
	ei, ok := d.Index.Enums[key]
	if !ok {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "```edbml\nEnum %s\n```\n", ei.Decl.Name.String())
	for _, v := range ei.Decl.Values {
		b.WriteString("- `" + v.Name.Name() + "`")
		if note := settingString(v.Settings, "note"); note != "" {
			b.WriteString(" — " + note)
		}
		b.WriteString("\n")
	}
	return b.String()
}

func (d *Document) hoverPartial(name string) string {
	pi, ok := d.Index.Partials[name]
	if !ok {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "```edbml\nTablePartial %s\n```\n", name)
	for _, item := range pi.Decl.Body {
		if col, ok := item.(*ast.Column); ok {
			b.WriteString("- `" + col.Name.Name() + " " + col.Type.String() + "`" + settingsSuffix(col.Settings, col.LegacyFlags) + "\n")
		}
	}
	fmt.Fprintf(&b, "\nInjected %d time(s).\n", pi.Uses)
	return b.String()
}

func (d *Document) hoverColumn(id SymbolID) string {
	col := d.findColumn(id)
	if col == nil {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "```edbml\n%s %s\n```\n", col.Name.Name(), col.Type.String())
	if s := settingsSuffix(col.Settings, col.LegacyFlags); s != "" {
		b.WriteString(strings.TrimSpace(s) + "\n")
	}
	if strings.HasPrefix(id.Container, "partial:") {
		b.WriteString("\nDefined in TablePartial `" + strings.TrimPrefix(id.Container, "partial:") + "`.\n")
	}
	if note := settingString(col.Settings, "note"); note != "" {
		b.WriteString("\n---\n" + note + "\n")
	}
	return b.String()
}

func (d *Document) hoverEnumValue(id SymbolID) string {
	key := strings.TrimPrefix(id.Container, "enum:")
	ei, ok := d.Index.Enums[key]
	if !ok {
		return ""
	}
	for _, v := range ei.Decl.Values {
		if v.Name.Name() == id.Name {
			md := fmt.Sprintf("```edbml\n%s.%s\n```\n", ei.Decl.Name.String(), id.Name)
			if note := settingString(v.Settings, "note"); note != "" {
				md += "\n" + note + "\n"
			}
			return md
		}
	}
	return ""
}

// findColumn resolves a column SymbolID back to its AST node.
func (d *Document) findColumn(id SymbolID) *ast.Column {
	if name, ok := strings.CutPrefix(id.Container, "partial:"); ok {
		pi, ok := d.Index.Partials[name]
		if !ok {
			return nil
		}
		for _, item := range pi.Decl.Body {
			if col, ok := item.(*ast.Column); ok && col.Name.Name() == id.Name {
				return col
			}
		}
		return nil
	}
	key := strings.TrimPrefix(id.Container, "table:")
	ti, ok := d.Index.Tables[key]
	if !ok {
		return nil
	}
	if cd := ti.Column(id.Name); cd != nil {
		return cd.Col
	}
	return nil
}

// settingsSuffix renders " — pk, not null, default: x" for hover lines.
func settingsSuffix(s *ast.SettingList, legacy []*ast.Ident) string {
	var parts []string
	for _, f := range legacy {
		parts = append(parts, f.Name())
	}
	if s != nil {
		for _, setting := range s.Settings {
			if setting.Name == "note" {
				continue
			}
			part := setting.Name
			if setting.Value != nil {
				part += ": " + valueString(setting.Value)
			}
			parts = append(parts, part)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return " — " + strings.Join(parts, ", ")
}

func settingString(s *ast.SettingList, name string) string {
	setting := s.Get(name)
	if setting == nil {
		return ""
	}
	return valueString(setting.Value)
}

func valueString(v ast.Node) string {
	switch v := v.(type) {
	case *ast.BasicLit:
		return v.Tok.Val
	case *ast.Ident:
		return v.Name()
	case *ast.FuncExpr:
		return "`" + v.Text() + "`"
	case *ast.NegNumber:
		return "-" + v.Num.Tok.Val
	case *ast.EnumConst:
		return v.Enum.Name() + "." + v.Value.Name()
	case *ast.RefValue:
		cols := identNames(v.Endpoint.Columns)
		return v.OpTok.Text + " " + v.Endpoint.Table.String() + "." + strings.Join(cols, ", ")
	}
	return ""
}

func tableNote(t *ast.Table) string {
	if note := settingString(t.Settings, "note"); note != "" {
		return note
	}
	for _, item := range t.Body {
		if n, ok := item.(*ast.Note); ok {
			return n.Text.Tok.Val
		}
	}
	return ""
}

// unused import guard
var _ = check.Info{}
