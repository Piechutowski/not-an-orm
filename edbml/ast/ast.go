// Package ast declares the types used to represent DBML syntax trees,
// mirroring the role of go/ast. The parser produces these nodes; the check
// and vet packages consume them. Nodes carry no semantic judgment — a
// Setting is a name and a value, whether or not that setting is legal where
// it appeared. Legality is the check package's business.
package ast

import "github.com/Piechutowski/not-an-orm/edbml/token"

// Node is implemented by all AST nodes.
type Node interface {
	Pos() token.Position // position of the first token of the node
	End() token.Position // position immediately after the node
}

// ----------------------------------------------------------------------------
// Expressions and shared forms

// Ident is a plain or quoted identifier (spec §3.4).
type Ident struct {
	Tok token.Token
}

func (x *Ident) Pos() token.Position { return x.Tok.Pos }
func (x *Ident) End() token.Position { return x.Tok.End() }

// Name returns the identifier's value with escapes applied.
func (x *Ident) Name() string { return x.Tok.Val }

// Quoted reports whether the identifier was written "quoted"; quoted
// identifiers never act as keywords (spec §1.4).
func (x *Ident) Quoted() bool { return x.Tok.Quoted }

// QualName is a possibly schema-qualified name: name or schema.name
// (spec §4.1).
type QualName struct {
	Parts []*Ident // 1 or 2 elements
}

func (x *QualName) Pos() token.Position { return x.Parts[0].Pos() }
func (x *QualName) End() token.Position { return x.Parts[len(x.Parts)-1].End() }

// Schema returns the schema qualifier, or "" for the default schema.
func (x *QualName) Schema() string {
	if len(x.Parts) == 2 {
		return x.Parts[0].Name()
	}
	return ""
}

// Base returns the unqualified name.
func (x *QualName) Base() string { return x.Parts[len(x.Parts)-1].Name() }

// String returns the canonical spelling, e.g. "core.users" or "users".
func (x *QualName) String() string {
	if len(x.Parts) == 2 {
		return x.Parts[0].Name() + "." + x.Parts[1].Name()
	}
	return x.Parts[0].Name()
}

// BasicLit is a STRING, NUMBER or COLOR literal.
type BasicLit struct {
	Tok token.Token
}

func (x *BasicLit) Pos() token.Position { return x.Tok.Pos }
func (x *BasicLit) End() token.Position { return x.Tok.End() }

// FuncExpr is a backtick expression literal (spec §3.12).
type FuncExpr struct {
	Tok token.Token
}

func (x *FuncExpr) Pos() token.Position { return x.Tok.Pos }
func (x *FuncExpr) End() token.Position { return x.Tok.End() }
func (x *FuncExpr) Text() string        { return x.Tok.Val }

// NegNumber is a minus sign applied to a number literal (spec §6.4, §6.10).
type NegNumber struct {
	MinusPos token.Position
	Num      *BasicLit
}

func (x *NegNumber) Pos() token.Position { return x.MinusPos }
func (x *NegNumber) End() token.Position { return x.Num.End() }

// EnumConst is a dotted enum value reference, e.g. status.active (spec §4.1).
type EnumConst struct {
	Enum  *Ident
	Value *Ident
}

func (x *EnumConst) Pos() token.Position { return x.Enum.Pos() }
func (x *EnumConst) End() token.Position { return x.Value.End() }

// Empty is a record field with nothing between separators (spec §6.10).
type Empty struct {
	At token.Position
}

func (x *Empty) Pos() token.Position { return x.At }
func (x *Empty) End() token.Position { return x.At }

// RefValue is the value of a column's ref: setting (spec §6.7):
// an operator followed by an endpoint.
type RefValue struct {
	OpTok    token.Token // LT, GT, MINUS or LTGT
	Endpoint *RefEndpoint
}

func (x *RefValue) Pos() token.Position { return x.OpTok.Pos }
func (x *RefValue) End() token.Position { return x.Endpoint.End() }

// ----------------------------------------------------------------------------
// Settings (spec §4.2)

// SettingList is a bracketed, comma-separated settings list.
type SettingList struct {
	Lbrack   token.Position
	Settings []*Setting
	Rbrack   token.Position
}

func (x *SettingList) Pos() token.Position { return x.Lbrack }
func (x *SettingList) End() token.Position { return x.Rbrack }

// Get returns the first setting whose canonical name matches, or nil.
func (x *SettingList) Get(name string) *Setting {
	if x == nil {
		return nil
	}
	for _, s := range x.Settings {
		if s.Name == name {
			return s
		}
	}
	return nil
}

// Setting is one entry: a flag (pk) or a key-value pair (note: '...').
// Name is the canonical lowercased, space-normalized form ("not null").
type Setting struct {
	NameTok token.Token // first word of the name
	Name    string
	Value   Node // nil for flags; BasicLit, Ident, FuncExpr, NegNumber, EnumConst or RefValue
	endPos  token.Position
}

func (x *Setting) Pos() token.Position { return x.NameTok.Pos }
func (x *Setting) End() token.Position { return x.endPos }

// SetEnd is used by the parser to record the setting's extent.
func (x *Setting) SetEnd(p token.Position) { x.endPos = p }

// ----------------------------------------------------------------------------
// File and declarations (spec §5)

// File is one parsed DBML source file.
type File struct {
	Name  string // file name as given to the parser
	Decls []Decl // top-level declarations in source order
	EOF   token.Position
}

func (f *File) Pos() token.Position {
	if len(f.Decls) > 0 {
		return f.Decls[0].Pos()
	}
	return f.EOF
}
func (f *File) End() token.Position { return f.EOF }

// HasImports reports whether the file contains use/reuse statements; when
// it does, names may resolve in other files and the checker relaxes
// unresolved-name errors (spec §7).
func (f *File) HasImports() bool {
	for _, d := range f.Decls {
		if _, ok := d.(*Use); ok {
			return true
		}
	}
	return false
}

// Decl is implemented by all top-level declarations.
type Decl interface {
	Node
	declNode()
}

// Use is a use/reuse import statement (spec §7).
type Use struct {
	UsePos   token.Position
	Reuse    bool
	Wildcard bool
	Items    []*UseItem // nil when Wildcard
	Path     *BasicLit  // string literal
}

func (d *Use) Pos() token.Position { return d.UsePos }
func (d *Use) End() token.Position { return d.Path.End() }
func (d *Use) declNode()           {}

// UseItem is one selective-import item: kind name [as alias].
type UseItem struct {
	Kind  *Ident
	Name  *QualName
	Alias *Ident // nil if not aliased
}

func (x *UseItem) Pos() token.Position { return x.Kind.Pos() }
func (x *UseItem) End() token.Position {
	if x.Alias != nil {
		return x.Alias.End()
	}
	return x.Name.End()
}

// Project is a Project element (spec §6.1).
type Project struct {
	ProjectPos token.Position
	Name       *Ident // may be nil
	Props      []*ProjectProp
	Notes      []*Note
	Rbrace     token.Position
}

func (d *Project) Pos() token.Position { return d.ProjectPos }
func (d *Project) End() token.Position { return d.Rbrace }
func (d *Project) declNode()           {}

// ProjectProp is one key: 'value' line inside a Project body.
type ProjectProp struct {
	Key   *Ident
	Value *BasicLit
}

func (x *ProjectProp) Pos() token.Position { return x.Key.Pos() }
func (x *ProjectProp) End() token.Position { return x.Value.End() }

// Table is a Table element (spec §6.2).
type Table struct {
	TablePos token.Position
	Name     *QualName
	Alias    *Ident // may be nil
	Settings *SettingList
	Body     []TableItem
	Rbrace   token.Position
}

func (d *Table) Pos() token.Position { return d.TablePos }
func (d *Table) End() token.Position { return d.Rbrace }
func (d *Table) declNode()           {}

// TablePartial is a TablePartial element (spec §6.9).
type TablePartial struct {
	PartialPos token.Position
	Name       *Ident
	Settings   *SettingList
	Body       []TableItem
	Rbrace     token.Position
}

func (d *TablePartial) Pos() token.Position { return d.PartialPos }
func (d *TablePartial) End() token.Position { return d.Rbrace }
func (d *TablePartial) declNode()           {}

// TableItem is implemented by everything that can appear in a Table or
// TablePartial body: *Column, *IndexesBlock, *ChecksBlock, *Note,
// *PartialRef, *Records.
type TableItem interface {
	Node
	tableItemNode()
}

// Column is one column definition (spec §6.3).
type Column struct {
	Name        *Ident
	Type        *TypeRef
	LegacyFlags []*Ident // bare pk/unique between type and settings (§6.3.7)
	Settings    *SettingList
}

func (x *Column) Pos() token.Position { return x.Name.Pos() }
func (x *Column) End() token.Position {
	if x.Settings != nil {
		return x.Settings.End()
	}
	if n := len(x.LegacyFlags); n > 0 {
		return x.LegacyFlags[n-1].End()
	}
	return x.Type.End()
}
func (x *Column) tableItemNode() {}

// TypeRef is a column type: name, schema-qualified name, and optional
// parenthesized arguments, e.g. varchar(255) or v2.job_status (spec §6.3).
type TypeRef struct {
	Name   *QualName
	Args   []token.Token // NUMBER or IDENT argument tokens, verbatim
	Rparen token.Position
}

func (x *TypeRef) Pos() token.Position { return x.Name.Pos() }
func (x *TypeRef) End() token.Position {
	if len(x.Args) > 0 {
		return x.Rparen
	}
	return x.Name.End()
}

// String returns the type as written, e.g. "decimal(10,2)".
func (x *TypeRef) String() string {
	s := x.Name.String()
	if len(x.Args) > 0 {
		s += "("
		for i, a := range x.Args {
			if i > 0 {
				s += ","
			}
			s += a.Val
		}
		s += ")"
	}
	return s
}

// IndexesBlock is one indexes { ... } block (spec §6.5).
type IndexesBlock struct {
	IndexesPos token.Position
	Indexes    []*Index
	Rbrace     token.Position
}

func (x *IndexesBlock) Pos() token.Position { return x.IndexesPos }
func (x *IndexesBlock) End() token.Position { return x.Rbrace }
func (x *IndexesBlock) tableItemNode()      {}

// Index is one index definition. Key atoms are *Ident (columns) or
// *FuncExpr (expressions).
type Index struct {
	Key       []Node
	Composite bool // written with parentheses
	Settings  *SettingList
}

func (x *Index) Pos() token.Position { return x.Key[0].Pos() }
func (x *Index) End() token.Position {
	if x.Settings != nil {
		return x.Settings.End()
	}
	return x.Key[len(x.Key)-1].End()
}

// ChecksBlock is one checks { ... } block (spec §6.6).
type ChecksBlock struct {
	ChecksPos token.Position
	Checks    []*Check
	Rbrace    token.Position
}

func (x *ChecksBlock) Pos() token.Position { return x.ChecksPos }
func (x *ChecksBlock) End() token.Position { return x.Rbrace }
func (x *ChecksBlock) tableItemNode()      {}

// Check is one check constraint line.
type Check struct {
	Expr     *FuncExpr
	Settings *SettingList
}

func (x *Check) Pos() token.Position { return x.Expr.Pos() }
func (x *Check) End() token.Position {
	if x.Settings != nil {
		return x.Settings.End()
	}
	return x.Expr.End()
}

// PartialRef is a ~partial injection line (spec §6.9).
type PartialRef struct {
	TildePos token.Position
	Name     *Ident
}

func (x *PartialRef) Pos() token.Position { return x.TildePos }
func (x *PartialRef) End() token.Position { return x.Name.End() }
func (x *PartialRef) tableItemNode()      {}

// Note is an in-body note definition (spec §6.11).
type Note struct {
	NotePos token.Position
	Text    *BasicLit
	endPos  token.Position
}

func (x *Note) Pos() token.Position     { return x.NotePos }
func (x *Note) End() token.Position     { return x.endPos }
func (x *Note) SetEnd(p token.Position) { x.endPos = p }
func (x *Note) tableItemNode()          {}

// Ref is a top-level relationship element, short or long form (spec §6.7).
type Ref struct {
	RefPos   token.Position
	Name     *Ident // may be nil
	Long     bool
	Left     *RefEndpoint
	OpTok    token.Token
	Right    *RefEndpoint
	Settings *SettingList
	endPos   token.Position
}

func (d *Ref) Pos() token.Position     { return d.RefPos }
func (d *Ref) End() token.Position     { return d.endPos }
func (d *Ref) SetEnd(p token.Position) { d.endPos = p }
func (d *Ref) declNode()               {}

// RefEndpoint is one side of a relationship: a table (or alias), optionally
// schema-qualified, and one or more columns (spec §6.7).
type RefEndpoint struct {
	Table   *QualName
	Columns []*Ident
	endPos  token.Position
}

func (x *RefEndpoint) Pos() token.Position     { return x.Table.Pos() }
func (x *RefEndpoint) End() token.Position     { return x.endPos }
func (x *RefEndpoint) SetEnd(p token.Position) { x.endPos = p }

// Enum is an Enum element (spec §6.8).
type Enum struct {
	EnumPos token.Position
	Name    *QualName
	Values  []*EnumValue
	Rbrace  token.Position
}

func (d *Enum) Pos() token.Position { return d.EnumPos }
func (d *Enum) End() token.Position { return d.Rbrace }
func (d *Enum) declNode()           {}

// EnumValue is one enum member.
type EnumValue struct {
	Name     *Ident
	Settings *SettingList
}

func (x *EnumValue) Pos() token.Position { return x.Name.Pos() }
func (x *EnumValue) End() token.Position {
	if x.Settings != nil {
		return x.Settings.End()
	}
	return x.Name.End()
}

// Records is sample data, top-level or in-table (spec §6.10).
type Records struct {
	RecordsPos token.Position
	Table      *QualName // nil for in-table records
	Columns    []*Ident  // nil when the column list is implicit
	HasColumns bool
	Rows       []*RecordRow
	Rbrace     token.Position
}

func (d *Records) Pos() token.Position { return d.RecordsPos }
func (d *Records) End() token.Position { return d.Rbrace }
func (d *Records) declNode()           {}
func (d *Records) tableItemNode()      {}

// RecordRow is one CSV-style row. Values are *BasicLit, *Ident (true/false/
// null), *FuncExpr, *NegNumber, *EnumConst or *Empty.
type RecordRow struct {
	Values []Node
}

func (x *RecordRow) Pos() token.Position { return x.Values[0].Pos() }
func (x *RecordRow) End() token.Position { return x.Values[len(x.Values)-1].End() }

// StickyNote is a top-level named note (spec §6.11).
type StickyNote struct {
	NotePos  token.Position
	Name     *Ident
	Settings *SettingList
	Text     *BasicLit
	Rbrace   token.Position
}

func (d *StickyNote) Pos() token.Position { return d.NotePos }
func (d *StickyNote) End() token.Position { return d.Rbrace }
func (d *StickyNote) declNode()           {}

// TableGroup is a TableGroup element (spec §6.12).
type TableGroup struct {
	GroupPos token.Position
	Name     *Ident
	Settings *SettingList
	Members  []*QualName
	Notes    []*Note
	Rbrace   token.Position
}

func (d *TableGroup) Pos() token.Position { return d.GroupPos }
func (d *TableGroup) End() token.Position { return d.Rbrace }
func (d *TableGroup) declNode()           {}

// DiagramView is a DiagramView element (spec §6.13).
type DiagramView struct {
	ViewPos    token.Position
	Name       *Ident
	Categories []*ViewCategory
	Rbrace     token.Position
}

func (d *DiagramView) Pos() token.Position { return d.ViewPos }
func (d *DiagramView) End() token.Position { return d.Rbrace }
func (d *DiagramView) declNode()           {}

// ViewCategory is one Tables/Notes/TableGroups/Schemas selector.
type ViewCategory struct {
	Kind     *Ident
	Wildcard bool
	Names    []*QualName
	Rbrace   token.Position
}

func (x *ViewCategory) Pos() token.Position { return x.Kind.Pos() }
func (x *ViewCategory) End() token.Position { return x.Rbrace }
