package lsp

import (
	"github.com/Piechutowski/not-an-orm/edbml/ast"
	"github.com/Piechutowski/not-an-orm/edbml/check"
)

// SymKind classifies the symbols the server can navigate.
type SymKind int

const (
	SymTable SymKind = iota
	SymEnum
	SymPartial
	SymColumn
	SymEnumValue
)

// SymbolID names one symbol. Tables and enums use their canonical
// "schema.name" key; columns live in a container — "table:<key>" for direct
// definitions or "partial:<name>" for columns defined in a TablePartial, so
// every table sharing the partial shares the symbol (a rename edits the
// partial once). Enum values live in "enum:<key>".
type SymbolID struct {
	Kind      SymKind
	Container string
	Name      string
}

// Occurrence is one identifier that denotes a symbol.
type Occurrence struct {
	ID     SymbolID
	Ident  *ast.Ident
	IsDecl bool
}

// Index is every symbol occurrence in a document, plus resolution maps.
type Index struct {
	Occs []Occurrence

	Tables   map[string]*check.TableInfo // canonical key AND alias -> table
	Enums    map[string]*check.EnumInfo  // canonical key -> enum
	Partials map[string]*check.PartialInfo

	decls map[SymbolID]*ast.Ident
}

// canon normalizes a possibly-qualified name with the default schema.
func canon(q *ast.QualName) string {
	if q.Schema() == "" {
		return "public." + q.Base()
	}
	return q.String()
}

// NewIndex resolves every name in the file against the semantic model.
func NewIndex(f *ast.File, info *check.Info) *Index {
	ix := &Index{
		Tables:   map[string]*check.TableInfo{},
		Enums:    map[string]*check.EnumInfo{},
		Partials: map[string]*check.PartialInfo{},
		decls:    map[SymbolID]*ast.Ident{},
	}
	for _, ti := range info.Tables {
		ix.Tables[ti.Key] = ti
		if ti.Alias != "" {
			ix.Tables["alias:"+ti.Alias] = ti
		}
	}
	for _, ei := range info.Enums {
		ix.Enums[ei.Key] = ei
	}
	for _, pi := range info.Partials {
		ix.Partials[pi.Decl.Name.Name()] = pi
	}

	// --- declarations ---
	for _, ti := range info.Tables {
		id := SymbolID{SymTable, "", ti.Key}
		ix.add(id, lastPart(ti.Decl.Name), true)
		if ti.Decl.Alias != nil {
			ix.add(id, ti.Decl.Alias, false)
		}
	}
	for _, ei := range info.Enums {
		ix.add(SymbolID{SymEnum, "", ei.Key}, lastPart(ei.Decl.Name), true)
		for _, v := range ei.Decl.Values {
			ix.add(SymbolID{SymEnumValue, "enum:" + ei.Key, v.Name.Name()}, v.Name, true)
		}
	}
	for _, pi := range info.Partials {
		ix.add(SymbolID{SymPartial, "", pi.Decl.Name.Name()}, pi.Decl.Name, true)
	}
	// Column declarations, container-aware: a column written in a partial is
	// declared once even when injected into many tables.
	for _, pi := range info.Partials {
		for _, item := range pi.Decl.Body {
			if col, ok := item.(*ast.Column); ok {
				ix.add(SymbolID{SymColumn, "partial:" + pi.Decl.Name.Name(), col.Name.Name()}, col.Name, true)
			}
		}
	}
	for _, ti := range info.Tables {
		for _, cd := range ti.Columns {
			if cd.Partial == nil {
				ix.add(SymbolID{SymColumn, "table:" + ti.Key, cd.Col.Name.Name()}, cd.Col.Name, true)
			}
		}
	}

	// --- references ---
	for _, decl := range f.Decls {
		switch n := decl.(type) {
		case *ast.Table:
			ti := ix.resolveTable(n.Name)
			ix.walkTableBody(n.Body, ti, "")
		case *ast.TablePartial:
			ix.walkTableBody(n.Body, nil, n.Name.Name())
		case *ast.Ref:
			ix.walkEndpoint(n.Left)
			ix.walkEndpoint(n.Right)
		case *ast.Records:
			ti := ix.resolveTable(n.Table)
			if ti != nil {
				ix.add(SymbolID{SymTable, "", ti.Key}, lastPart(n.Table), false)
			}
			for _, c := range n.Columns {
				ix.addColumnRef(ti, "", c)
			}
			ix.walkRecordRows(n.Rows)
		case *ast.TableGroup:
			for _, m := range n.Members {
				if ti := ix.resolveTable(m); ti != nil {
					ix.add(SymbolID{SymTable, "", ti.Key}, lastPart(m), false)
				}
			}
		case *ast.DiagramView:
			for _, cat := range n.Categories {
				if !equalFold(cat.Kind.Name(), "tables") {
					continue
				}
				for _, m := range cat.Names {
					if ti := ix.resolveTable(m); ti != nil {
						ix.add(SymbolID{SymTable, "", ti.Key}, lastPart(m), false)
					}
				}
			}
		}
	}
	return ix
}

// walkTableBody records references inside one Table or TablePartial body.
// Exactly one of ti / partial is set and names the column container.
func (ix *Index) walkTableBody(body []ast.TableItem, ti *check.TableInfo, partial string) {
	for _, item := range body {
		switch n := item.(type) {
		case *ast.Column:
			// column type may reference an enum
			if ei := ix.resolveEnum(n.Type.Name); ei != nil {
				ix.add(SymbolID{SymEnum, "", ei.Key}, lastPart(n.Type.Name), false)
			}
			ix.walkSettings(n.Settings)
		case *ast.PartialRef:
			if _, ok := ix.Partials[n.Name.Name()]; ok {
				ix.add(SymbolID{SymPartial, "", n.Name.Name()}, n.Name, false)
			}
		case *ast.IndexesBlock:
			for _, index := range n.Indexes {
				for _, key := range index.Key {
					if id, ok := key.(*ast.Ident); ok {
						ix.addColumnRef(ti, partial, id)
					}
				}
				ix.walkSettings(index.Settings)
			}
		case *ast.Records:
			for _, c := range n.Columns {
				ix.addColumnRef(ti, "", c)
			}
			ix.walkRecordRows(n.Rows)
		case *ast.ChecksBlock:
			for _, c := range n.Checks {
				ix.walkSettings(c.Settings)
			}
		}
	}
}

// walkSettings records references in setting values: inline refs and enum
// constants (default: status.active).
func (ix *Index) walkSettings(s *ast.SettingList) {
	if s == nil {
		return
	}
	for _, setting := range s.Settings {
		switch v := setting.Value.(type) {
		case *ast.RefValue:
			ix.walkEndpoint(v.Endpoint)
		case *ast.EnumConst:
			ix.walkEnumConst(v)
		}
	}
}

func (ix *Index) walkRecordRows(rows []*ast.RecordRow) {
	for _, row := range rows {
		for _, v := range row.Values {
			if ec, ok := v.(*ast.EnumConst); ok {
				ix.walkEnumConst(ec)
			}
		}
	}
}

func (ix *Index) walkEnumConst(ec *ast.EnumConst) {
	key := "public." + ec.Enum.Name()
	ei, ok := ix.Enums[key]
	if !ok {
		return
	}
	ix.add(SymbolID{SymEnum, "", ei.Key}, ec.Enum, false)
	for _, v := range ei.Decl.Values {
		if v.Name.Name() == ec.Value.Name() {
			ix.add(SymbolID{SymEnumValue, "enum:" + ei.Key, ec.Value.Name()}, ec.Value, false)
			break
		}
	}
}

// walkEndpoint records the table and column references of one ref endpoint.
func (ix *Index) walkEndpoint(ep *ast.RefEndpoint) {
	if ep == nil {
		return
	}
	ti := ix.resolveTable(ep.Table)
	if ti == nil {
		return
	}
	ix.add(SymbolID{SymTable, "", ti.Key}, lastPart(ep.Table), false)
	for _, col := range ep.Columns {
		ix.addColumnRef(ti, "", col)
	}
}

// addColumnRef resolves a column identifier against its table (or partial)
// container and records the occurrence.
func (ix *Index) addColumnRef(ti *check.TableInfo, partial string, id *ast.Ident) {
	if id == nil {
		return
	}
	if partial != "" {
		if pi, ok := ix.Partials[partial]; ok {
			for _, item := range pi.Decl.Body {
				if col, ok := item.(*ast.Column); ok && col.Name.Name() == id.Name() {
					ix.add(SymbolID{SymColumn, "partial:" + partial, id.Name()}, id, false)
					return
				}
			}
		}
		return
	}
	if ti == nil {
		return
	}
	cd := ti.Column(id.Name())
	if cd == nil {
		return
	}
	container := "table:" + ti.Key
	if cd.Partial != nil {
		container = "partial:" + cd.Partial.Name.Name()
	}
	ix.add(SymbolID{SymColumn, container, id.Name()}, id, false)
}

// resolveTable resolves a table name or alias to its info.
func (ix *Index) resolveTable(q *ast.QualName) *check.TableInfo {
	if q == nil {
		return nil
	}
	if ti, ok := ix.Tables[canon(q)]; ok {
		return ti
	}
	if q.Schema() == "" {
		if ti, ok := ix.Tables["alias:"+q.Base()]; ok {
			return ti
		}
	}
	return nil
}

// resolveEnum resolves a type name to an enum, if it names one.
func (ix *Index) resolveEnum(q *ast.QualName) *check.EnumInfo {
	if q == nil {
		return nil
	}
	if ei, ok := ix.Enums[canon(q)]; ok {
		return ei
	}
	return nil
}

func (ix *Index) add(id SymbolID, ident *ast.Ident, isDecl bool) {
	if ident == nil {
		return
	}
	ix.Occs = append(ix.Occs, Occurrence{ID: id, Ident: ident, IsDecl: isDecl})
	if isDecl {
		if _, exists := ix.decls[id]; !exists {
			ix.decls[id] = ident
		}
	}
}

// At returns the occurrence whose identifier spans the byte offset.
func (ix *Index) At(offset int) *Occurrence {
	for i := range ix.Occs {
		tok := ix.Occs[i].Ident.Tok
		if tok.Pos.Offset <= offset && offset <= tok.End().Offset {
			return &ix.Occs[i]
		}
	}
	return nil
}

// Decl returns the declaring identifier of a symbol, or nil.
func (ix *Index) Decl(id SymbolID) *ast.Ident { return ix.decls[id] }

// OccurrencesOf returns all occurrences of a symbol.
func (ix *Index) OccurrencesOf(id SymbolID) []Occurrence {
	var out []Occurrence
	for _, o := range ix.Occs {
		if o.ID == id {
			out = append(out, o)
		}
	}
	return out
}

func lastPart(q *ast.QualName) *ast.Ident {
	if q == nil || len(q.Parts) == 0 {
		return nil
	}
	return q.Parts[len(q.Parts)-1]
}

func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if 'A' <= ca && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if 'A' <= cb && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}
