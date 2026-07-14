// Package parser turns DBML source into an ast.File, mirroring go/parser.
//
// The parser is a hand-written recursive descent over the EBNF of the
// specification (SPEC.md): one method per production. Like go/parser it
// is syntax-only — it accepts any setting name in any position, any value
// shape the token stream allows, and unresolved names; the check package
// judges meaning. This split keeps the front end reusable: tools that only
// need structure (formatters, highlighters) import parser without paying
// for analysis.
//
// On a syntax error the parser records a diagnostic and synchronizes to
// the next line or the enclosing brace, so one broken line does not hide
// the rest of the file (multiple errors per run, like the Go compiler).
package parser

import (
	"strings"

	"github.com/Piechutowski/not-an-orm/edbml/ast"
	"github.com/Piechutowski/not-an-orm/edbml/diag"
	"github.com/Piechutowski/not-an-orm/edbml/scanner"
	"github.com/Piechutowski/not-an-orm/edbml/token"
)

// ParseFile parses one DBML source file. It always returns a (possibly
// partial) AST; diagnostics carry every lexical and syntax error found.
func ParseFile(filename, src string) (*ast.File, []diag.Diagnostic) {
	toks, errs := scanner.Scan(filename, src)
	p := &parser{toks: toks, diags: errs}
	f := p.file(filename)
	diag.Sort(p.diags)
	return f, p.diags
}

type parser struct {
	toks  []token.Token
	pos   int
	diags []diag.Diagnostic
}

// bailout unwinds one declaration after an unrecoverable local error.
type bailout struct{}

func (p *parser) cur() token.Token  { return p.toks[p.pos] }
func (p *parser) next() token.Token { t := p.toks[p.pos]; p.pos++; return t }
func (p *parser) at(k token.Kind) bool {
	return p.cur().Kind == k
}
func (p *parser) peekKind(n int) token.Kind {
	if p.pos+n >= len(p.toks) {
		return token.EOF
	}
	return p.toks[p.pos+n].Kind
}

// atKw reports whether the current token is the given contextual keyword
// (§1.4: case-insensitive; quoted identifiers never act as keywords).
func (p *parser) atKw(kw string) bool {
	t := p.cur()
	return t.Kind == token.IDENT && !t.Quoted && strings.EqualFold(t.Val, kw)
}

func (p *parser) errorf(t token.Token, format string, args ...any) {
	p.diags = append(p.diags, diag.Errorf(t.Pos, "syntax", format, args...))
}

// fail records the error and unwinds to the enclosing declaration.
func (p *parser) fail(t token.Token, format string, args ...any) {
	p.errorf(t, format, args...)
	panic(bailout{})
}

func (p *parser) expect(k token.Kind, ctx string) token.Token {
	if !p.at(k) {
		p.fail(p.cur(), "expected %s in %s, found %s", k, ctx, p.cur())
	}
	return p.next()
}

// endOfLine enforces the newline terminator of line-oriented productions.
func (p *parser) endOfLine(ctx string) {
	if p.at(token.RBRACE) || p.at(token.EOF) || p.cur().NLBefore {
		return
	}
	p.fail(p.cur(), "expected end of line after %s, found %s", ctx, p.cur())
}

func (p *parser) ident(ctx string) *ast.Ident {
	return &ast.Ident{Tok: p.expect(token.IDENT, ctx)}
}

// qualName = [ schema name, "." ], name (§4.1).
func (p *parser) qualName(ctx string) *ast.QualName {
	first := p.ident(ctx)
	if p.at(token.DOT) {
		p.next()
		return &ast.QualName{Parts: []*ast.Ident{first, p.ident(ctx)}}
	}
	return &ast.QualName{Parts: []*ast.Ident{first}}
}

/* ===== file = { import statement | element } (§5) ===== */

func (p *parser) file(name string) *ast.File {
	f := &ast.File{Name: name}
	for !p.at(token.EOF) {
		if d := p.decl(); d != nil {
			f.Decls = append(f.Decls, d)
		}
	}
	f.EOF = p.cur().Pos
	return f
}

// decl parses one top-level declaration, recovering to the next plausible
// declaration start on error.
func (p *parser) decl() (d ast.Decl) {
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(bailout); !ok {
				panic(r)
			}
			d = nil
			p.syncTopLevel()
		}
	}()

	t := p.cur()
	if t.Kind != token.IDENT {
		p.fail(t, "expected an element type or use/reuse at top level (§5), found %s", t)
	}
	switch strings.ToLower(t.Val) {
	case "use", "reuse":
		return p.useDecl()
	case "project":
		return p.project()
	case "table":
		return p.table()
	case "tablepartial":
		return p.tablePartial()
	case "enum":
		return p.enum()
	case "ref":
		return p.ref()
	case "note":
		return p.stickyNote()
	case "tablegroup":
		return p.tableGroup()
	case "records":
		return p.recordsDecl()
	case "diagramview":
		return p.diagramView()
	default:
		p.fail(t, "unknown element type %q (§5)", t.Val)
		return nil
	}
}

// syncTopLevel skips tokens until something that can start a declaration:
// an identifier at the start of a line, balanced past any open braces.
func (p *parser) syncTopLevel() {
	if !p.at(token.EOF) {
		p.next() // always make progress past the offending token
	}
	depth := 0
	for !p.at(token.EOF) {
		t := p.cur()
		switch t.Kind {
		case token.LBRACE:
			depth++
		case token.RBRACE:
			if depth > 0 {
				depth--
			}
			p.next()
			continue
		case token.IDENT:
			if depth == 0 && t.NLBefore {
				return
			}
		}
		p.next()
	}
}

// syncLine skips to the next line inside a braced body. It always makes
// progress: without the initial next() an error raised on a line-initial
// token would be retried forever.
func (p *parser) syncLine() {
	if !p.at(token.EOF) && !p.at(token.RBRACE) {
		p.next()
	}
	for !p.at(token.EOF) && !p.at(token.RBRACE) && !p.cur().NLBefore {
		p.next()
	}
}

/* ===== import statement (§7) ===== */

func (p *parser) useDecl() *ast.Use {
	kw := p.next()
	d := &ast.Use{UsePos: kw.Pos, Reuse: strings.EqualFold(kw.Val, "reuse")}
	switch {
	case p.at(token.STAR):
		p.next()
		d.Wildcard = true
	case p.at(token.LBRACE):
		p.next()
		for !p.at(token.RBRACE) && !p.at(token.EOF) && !p.atKw("from") {
			d.Items = append(d.Items, p.useItem())
		}
		p.expect(token.RBRACE, "import specifier list (§7)")
		if len(d.Items) == 0 {
			p.fail(p.cur(), "selective import requires at least one item (§7)")
		}
	default:
		p.fail(p.cur(), "expected '*' or '{' after use/reuse (§7)")
	}
	if !p.atKw("from") {
		p.fail(p.cur(), "expected 'from' in import statement (§7)")
	}
	p.next()
	d.Path = &ast.BasicLit{Tok: p.expect(token.STRING, "import path (§7)")}
	return d
}

func (p *parser) useItem() *ast.UseItem {
	it := &ast.UseItem{Kind: p.ident("import item kind (§7)")}
	it.Name = p.qualName("import item name (§7)")
	if p.atKw("as") {
		p.next()
		it.Alias = p.ident("import alias (§7)")
	}
	return it
}

/* ===== Project (§6.1) ===== */

func (p *parser) project() *ast.Project {
	d := &ast.Project{ProjectPos: p.next().Pos}
	if p.at(token.IDENT) {
		d.Name = p.ident("project name")
	}
	p.expect(token.LBRACE, "Project (§6.1)")
	for !p.at(token.RBRACE) && !p.at(token.EOF) {
		if p.atNoteDef() {
			d.Notes = append(d.Notes, p.noteDef())
			continue
		}
		key := p.ident("project property (§6.1)")
		p.expect(token.COLON, "project property (§6.1)")
		val := p.expect(token.STRING, "project property value (§6.1)")
		d.Props = append(d.Props, &ast.ProjectProp{Key: key, Value: &ast.BasicLit{Tok: val}})
		p.endOfLine("project property (§6.1)")
	}
	d.Rbrace = p.expect(token.RBRACE, "Project (§6.1)").Pos
	return d
}

/* ===== Table and TablePartial bodies (§6.2, §6.9) ===== */

func (p *parser) table() *ast.Table {
	d := &ast.Table{TablePos: p.next().Pos}
	d.Name = p.qualName("table name (§6.2)")
	if p.atKw("as") {
		p.next()
		d.Alias = p.ident("table alias (§6.2)")
	}
	if p.at(token.LBRACKET) {
		d.Settings = p.settingList()
	}
	p.expect(token.LBRACE, "Table (§6.2)")
	d.Body = p.tableBody("Table")
	d.Rbrace = p.expect(token.RBRACE, "Table (§6.2)").Pos
	return d
}

func (p *parser) tablePartial() *ast.TablePartial {
	d := &ast.TablePartial{PartialPos: p.next().Pos}
	d.Name = p.ident("TablePartial name (§6.9)")
	if p.at(token.LBRACKET) {
		d.Settings = p.settingList()
	}
	p.expect(token.LBRACE, "TablePartial (§6.9)")
	d.Body = p.tableBody("TablePartial")
	d.Rbrace = p.expect(token.RBRACE, "TablePartial (§6.9)").Pos
	return d
}

// atNoteDef reports whether the body cursor sits on "Note:" or "Note {".
func (p *parser) atNoteDef() bool {
	return p.atKw("note") && (p.peekKind(1) == token.COLON || p.peekKind(1) == token.LBRACE)
}

func (p *parser) tableBody(what string) []ast.TableItem {
	var items []ast.TableItem
	for !p.at(token.RBRACE) && !p.at(token.EOF) {
		item := p.tableItem(what)
		if item != nil {
			items = append(items, item)
		}
	}
	return items
}

// tableItem parses one body item, recovering to the next line on error so
// a broken column does not abort the whole table.
func (p *parser) tableItem(what string) (item ast.TableItem) {
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(bailout); !ok {
				panic(r)
			}
			item = nil
			p.syncLine()
		}
	}()

	t := p.cur()
	switch {
	case t.Kind == token.TILDE:
		p.next()
		pr := &ast.PartialRef{TildePos: t.Pos, Name: p.ident("partial injection (§6.9)")}
		p.endOfLine("partial injection (§6.9)")
		return pr
	case p.atKw("indexes") && p.peekKind(1) == token.LBRACE:
		return p.indexesBlock()
	case p.atKw("checks") && p.peekKind(1) == token.LBRACE:
		return p.checksBlock()
	case p.atNoteDef():
		return p.noteDef()
	case p.atKw("records") && (p.peekKind(1) == token.LPAREN || p.peekKind(1) == token.LBRACE):
		return p.records(nil)
	case t.Kind == token.IDENT:
		return p.column()
	default:
		p.fail(t, "expected a column, indexes, checks, Note, records or '~injection' in %s body, found %s", what, t)
		return nil
	}
}

// column = name, column type, { legacy flag }, [ column settings ], newline (§6.3).
func (p *parser) column() *ast.Column {
	c := &ast.Column{Name: p.ident("column name (§6.3)")}
	c.Type = p.typeRef()
	for p.at(token.IDENT) && !p.cur().NLBefore {
		c.LegacyFlags = append(c.LegacyFlags, p.ident("legacy flag (§6.3)"))
	}
	if p.at(token.LBRACKET) && !p.cur().NLBefore {
		c.Settings = p.settingList()
	}
	p.endOfLine("column definition (§6.3)")
	return c
}

func (p *parser) typeRef() *ast.TypeRef {
	tr := &ast.TypeRef{Name: p.qualName("column type (§6.3)")}
	if p.at(token.LPAREN) && !p.cur().SpBefore {
		p.next()
		for {
			t := p.cur()
			if t.Kind != token.NUMBER && t.Kind != token.IDENT {
				p.fail(t, "type argument must be a number or identifier (§6.3), found %s", t)
			}
			tr.Args = append(tr.Args, p.next())
			if p.at(token.COMMA) {
				p.next()
				continue
			}
			break
		}
		tr.Rparen = p.expect(token.RPAREN, "type arguments (§6.3)").End()
	}
	return tr
}

/* ===== indexes and checks (§6.5, §6.6) ===== */

func (p *parser) indexesBlock() *ast.IndexesBlock {
	b := &ast.IndexesBlock{IndexesPos: p.next().Pos}
	p.expect(token.LBRACE, "indexes block (§6.5)")
	for !p.at(token.RBRACE) && !p.at(token.EOF) {
		b.Indexes = append(b.Indexes, p.index())
	}
	b.Rbrace = p.expect(token.RBRACE, "indexes block (§6.5)").Pos
	return b
}

func (p *parser) indexAtom() ast.Node {
	t := p.cur()
	switch t.Kind {
	case token.IDENT:
		return p.ident("index key (§6.5)")
	case token.FUNCEXPR:
		return &ast.FuncExpr{Tok: p.next()}
	default:
		p.fail(t, "index key must be a column name or backtick expression (§6.5), found %s", t)
		return nil
	}
}

func (p *parser) index() *ast.Index {
	ix := &ast.Index{}
	if p.at(token.LPAREN) {
		ix.Composite = true
		p.next()
		for {
			ix.Key = append(ix.Key, p.indexAtom())
			if p.at(token.COMMA) {
				p.next()
				continue
			}
			break
		}
		p.expect(token.RPAREN, "composite index key (§6.5)")
	} else {
		ix.Key = append(ix.Key, p.indexAtom())
	}
	if p.at(token.LBRACKET) && !p.cur().NLBefore {
		ix.Settings = p.settingList()
	}
	p.endOfLine("index definition (§6.5)")
	return ix
}

func (p *parser) checksBlock() *ast.ChecksBlock {
	b := &ast.ChecksBlock{ChecksPos: p.next().Pos}
	p.expect(token.LBRACE, "checks block (§6.6)")
	for !p.at(token.RBRACE) && !p.at(token.EOF) {
		c := &ast.Check{Expr: &ast.FuncExpr{Tok: p.expect(token.FUNCEXPR, "check expression (§6.6)")}}
		if p.at(token.LBRACKET) && !p.cur().NLBefore {
			c.Settings = p.settingList()
		}
		p.endOfLine("check definition (§6.6)")
		b.Checks = append(b.Checks, c)
	}
	b.Rbrace = p.expect(token.RBRACE, "checks block (§6.6)").Pos
	return b
}

/* ===== Ref (§6.7) ===== */

func (p *parser) ref() *ast.Ref {
	d := &ast.Ref{RefPos: p.next().Pos}
	if p.at(token.IDENT) {
		d.Name = p.ident("relationship name")
	}
	if p.at(token.COLON) {
		p.next()
		p.refBody(d)
		d.SetEnd(p.toks[p.pos-1].End())
		p.endOfLine("Ref (§6.7)")
		return d
	}
	d.Long = true
	p.expect(token.LBRACE, "Ref (§6.7)")
	if p.at(token.RBRACE) {
		p.fail(p.cur(), "a Ref declares exactly one relationship (§6.7)")
	}
	p.refBody(d)
	if !p.at(token.RBRACE) {
		p.fail(p.cur(), "a Ref declares exactly one relationship (§6.7); found %s after the first", p.cur())
	}
	d.SetEnd(p.expect(token.RBRACE, "Ref (§6.7)").End())
	return d
}

func (p *parser) refBody(d *ast.Ref) {
	d.Left = p.refEndpoint()
	d.OpTok = p.relOp()
	d.Right = p.refEndpoint()
	if p.at(token.LBRACKET) && !p.cur().NLBefore {
		d.Settings = p.settingList()
	}
}

func (p *parser) relOp() token.Token {
	t := p.cur()
	switch t.Kind {
	case token.LT, token.GT, token.MINUS, token.LTGT:
		return p.next()
	}
	p.fail(t, "expected relationship operator <, >, - or <> (§6.7), found %s", t)
	return token.Token{}
}

// refEndpoint = table name, ".", column group (§6.7). The table part may be
// one or two names ([schema.]table); the final segment is the column(s).
func (p *parser) refEndpoint() *ast.RefEndpoint {
	first := p.ident("relationship endpoint (§6.7)")
	parts := []*ast.Ident{first}
	ep := &ast.RefEndpoint{}
	for {
		if !p.at(token.DOT) {
			p.fail(p.cur(), "endpoint must reference a column as [schema.]table.column (§6.7)")
		}
		p.next()
		if p.at(token.LPAREN) { // composite column group
			p.next()
			for {
				ep.Columns = append(ep.Columns, p.ident("endpoint column (§6.7)"))
				if p.at(token.COMMA) {
					p.next()
					continue
				}
				break
			}
			ep.SetEnd(p.expect(token.RPAREN, "endpoint column group (§6.7)").End())
			break
		}
		parts = append(parts, p.ident("relationship endpoint (§6.7)"))
		if !p.at(token.DOT) {
			// last segment is the single column
			ep.Columns = []*ast.Ident{parts[len(parts)-1]}
			parts = parts[:len(parts)-1]
			ep.SetEnd(ep.Columns[0].End())
			break
		}
		if len(parts) == 3 {
			p.fail(p.cur(), "endpoint has too many name parts; expected [schema.]table.column (§6.7)")
		}
	}
	if len(parts) == 0 || len(parts) > 2 {
		p.fail(p.cur(), "endpoint must be [schema.]table.column (§6.7)")
	}
	ep.Table = &ast.QualName{Parts: parts}
	return ep
}

/* ===== Enum (§6.8) ===== */

func (p *parser) enum() *ast.Enum {
	d := &ast.Enum{EnumPos: p.next().Pos}
	d.Name = p.qualName("enum name (§6.8)")
	p.expect(token.LBRACE, "Enum (§6.8)")
	for !p.at(token.RBRACE) && !p.at(token.EOF) {
		v := &ast.EnumValue{Name: p.ident("enum value (§6.8)")}
		if p.at(token.LBRACKET) && !p.cur().NLBefore {
			v.Settings = p.settingList()
		}
		p.endOfLine("enum value (§6.8)")
		d.Values = append(d.Values, v)
	}
	d.Rbrace = p.expect(token.RBRACE, "Enum (§6.8)").Pos
	return d
}

/* ===== Records (§6.10) ===== */

func (p *parser) recordsDecl() *ast.Records {
	pos := p.next().Pos
	table := p.qualName("records target table (§6.10)")
	return p.recordsRest(pos, table)
}

// records parses an in-table records block; the caller has seen the keyword.
func (p *parser) records(_ *ast.QualName) *ast.Records {
	pos := p.next().Pos
	return p.recordsRest(pos, nil)
}

func (p *parser) recordsRest(pos token.Position, table *ast.QualName) *ast.Records {
	d := &ast.Records{RecordsPos: pos, Table: table}
	if p.at(token.LPAREN) {
		d.HasColumns = true
		p.next()
		for {
			d.Columns = append(d.Columns, p.ident("records column (§6.10)"))
			if p.at(token.COMMA) {
				p.next()
				continue
			}
			break
		}
		p.expect(token.RPAREN, "records column list (§6.10)")
	}
	p.expect(token.LBRACE, "records body (§6.10)")
	for !p.at(token.RBRACE) && !p.at(token.EOF) {
		d.Rows = append(d.Rows, p.recordRow())
	}
	d.Rbrace = p.expect(token.RBRACE, "records body (§6.10)").Pos
	return d
}

func (p *parser) recordRow() *ast.RecordRow {
	row := &ast.RecordRow{}
	rowStart := true
	for {
		// one field: empty when the cursor sits on a separator or row end
		switch {
		case p.at(token.COMMA) || p.at(token.RBRACE) || p.at(token.EOF),
			!rowStart && p.cur().NLBefore:
			row.Values = append(row.Values, &ast.Empty{At: p.cur().Pos})
		default:
			row.Values = append(row.Values, p.recordValue())
		}
		// separator: a comma on the same line (the first token of a row
		// legitimately carries NLBefore)
		if p.at(token.COMMA) && (!p.cur().NLBefore || rowStart) {
			p.next()
			rowStart = false
			continue
		}
		p.endOfLine("record row (§6.10)")
		return row
	}
}

func (p *parser) recordValue() ast.Node {
	t := p.cur()
	switch t.Kind {
	case token.STRING, token.NUMBER:
		return &ast.BasicLit{Tok: p.next()}
	case token.FUNCEXPR:
		return &ast.FuncExpr{Tok: p.next()}
	case token.MINUS:
		p.next()
		return &ast.NegNumber{MinusPos: t.Pos, Num: &ast.BasicLit{Tok: p.expect(token.NUMBER, "record value (§6.10)")}}
	case token.IDENT:
		id := p.ident("record value (§6.10)")
		if p.at(token.DOT) {
			p.next()
			return &ast.EnumConst{Enum: id, Value: p.ident("enum constant (§6.10)")}
		}
		return id // true/false/null; anything else is check's business
	default:
		p.fail(t, "invalid record value (§6.10): found %s", t)
		return nil
	}
}

/* ===== Notes (§6.11) ===== */

// noteDef parses "Note: 'text'" or "Note { 'text' }" with the cursor on Note.
func (p *parser) noteDef() *ast.Note {
	n := &ast.Note{NotePos: p.next().Pos}
	if p.at(token.COLON) {
		p.next()
		n.Text = &ast.BasicLit{Tok: p.expect(token.STRING, "note value (§6.11)")}
		n.SetEnd(n.Text.End())
		p.endOfLine("note definition (§6.11)")
		return n
	}
	p.expect(token.LBRACE, "note definition (§6.11)")
	n.Text = &ast.BasicLit{Tok: p.expect(token.STRING, "note value (§6.11)")}
	n.SetEnd(p.expect(token.RBRACE, "note definition (§6.11)").End())
	return n
}

func (p *parser) stickyNote() *ast.StickyNote {
	d := &ast.StickyNote{NotePos: p.next().Pos}
	d.Name = p.ident("sticky note name (§6.11)")
	if p.at(token.LBRACKET) {
		d.Settings = p.settingList()
	}
	p.expect(token.LBRACE, "sticky note (§6.11)")
	d.Text = &ast.BasicLit{Tok: p.expect(token.STRING, "sticky note value (§6.11)")}
	d.Rbrace = p.expect(token.RBRACE, "sticky note (§6.11)").Pos
	return d
}

/* ===== TableGroup (§6.12) ===== */

func (p *parser) tableGroup() *ast.TableGroup {
	d := &ast.TableGroup{GroupPos: p.next().Pos}
	d.Name = p.ident("TableGroup name (§6.12)")
	if p.at(token.LBRACKET) {
		d.Settings = p.settingList()
	}
	p.expect(token.LBRACE, "TableGroup (§6.12)")
	for !p.at(token.RBRACE) && !p.at(token.EOF) {
		if p.atNoteDef() {
			d.Notes = append(d.Notes, p.noteDef())
			continue
		}
		d.Members = append(d.Members, p.qualName("TableGroup member (§6.12)"))
		p.endOfLine("TableGroup member (§6.12)")
	}
	d.Rbrace = p.expect(token.RBRACE, "TableGroup (§6.12)").Pos
	return d
}

/* ===== DiagramView (§6.13) ===== */

func (p *parser) diagramView() *ast.DiagramView {
	d := &ast.DiagramView{ViewPos: p.next().Pos}
	d.Name = p.ident("DiagramView name (§6.13)")
	p.expect(token.LBRACE, "DiagramView (§6.13)")
	for !p.at(token.RBRACE) && !p.at(token.EOF) {
		c := &ast.ViewCategory{Kind: p.ident("view category (§6.13)")}
		p.expect(token.LBRACE, "view category body (§6.13)")
		if p.at(token.STAR) {
			p.next()
			c.Wildcard = true
		} else {
			for !p.at(token.RBRACE) && !p.at(token.EOF) {
				c.Names = append(c.Names, p.qualName("view category member (§6.13)"))
			}
		}
		c.Rbrace = p.expect(token.RBRACE, "view category body (§6.13)").Pos
		d.Categories = append(d.Categories, c)
	}
	d.Rbrace = p.expect(token.RBRACE, "DiagramView (§6.13)").Pos
	return d
}

/* ===== settings (§4.2) ===== */

// settingList parses "[" setting { "," setting } "]" generically. Which
// settings are legal where — and what value types they take — is decided
// by the check package, keeping parser and semantics decoupled.
func (p *parser) settingList() *ast.SettingList {
	sl := &ast.SettingList{Lbrack: p.expect(token.LBRACKET, "settings list (§4.2)").Pos}
	for {
		s := p.setting()
		sl.Settings = append(sl.Settings, s)
		if p.at(token.COMMA) {
			p.next()
			continue
		}
		break
	}
	sl.Rbrack = p.expect(token.RBRACKET, "settings list (§4.2)").End()
	return sl
}

func (p *parser) setting() *ast.Setting {
	nameTok := p.expect(token.IDENT, "setting name (§4.2)")
	words := []string{strings.ToLower(nameTok.Val)}
	end := nameTok.End()
	// multi-word setting names (§4.2.2): "not null", "primary key"
	for p.at(token.IDENT) && !p.cur().NLBefore {
		w := p.next()
		words = append(words, strings.ToLower(w.Val))
		end = w.End()
	}
	s := &ast.Setting{NameTok: nameTok, Name: strings.Join(words, " ")}
	s.SetEnd(end)
	if p.at(token.COLON) {
		p.next()
		s.Value = p.settingValue()
		s.SetEnd(s.Value.End())
	}
	return s
}

func (p *parser) settingValue() ast.Node {
	t := p.cur()
	switch t.Kind {
	case token.STRING, token.NUMBER, token.COLOR:
		return &ast.BasicLit{Tok: p.next()}
	case token.FUNCEXPR:
		return &ast.FuncExpr{Tok: p.next()}
	case token.MINUS:
		if p.peekKind(1) == token.IDENT { // one-to-one inline ref: ref: - a.b
			op := p.next()
			return &ast.RefValue{OpTok: op, Endpoint: p.refEndpoint()}
		}
		p.next()
		return &ast.NegNumber{MinusPos: t.Pos, Num: &ast.BasicLit{Tok: p.expect(token.NUMBER, "setting value (§4.2)")}}
	case token.LT, token.GT, token.LTGT:
		op := p.next()
		return &ast.RefValue{OpTok: op, Endpoint: p.refEndpoint()}
	case token.IDENT:
		id := p.ident("setting value (§4.2)")
		if p.at(token.DOT) {
			p.next()
			return &ast.EnumConst{Enum: id, Value: p.ident("enum constant (§4.1)")}
		}
		// multi-word identifier values: "no action", "set null"
		words := []*ast.Ident{id}
		for p.at(token.IDENT) && !p.cur().NLBefore {
			words = append(words, p.ident("setting value (§4.2)"))
		}
		if len(words) == 1 {
			return id
		}
		return &multiWordValue{words: words}
	default:
		p.fail(t, "invalid setting value (§4.2): found %s", t)
		return nil
	}
}

// multiWordValue represents space-separated identifier values such as the
// referential actions "no action" and "set null" (§6.7).
type multiWordValue struct{ words []*ast.Ident }

func (m *multiWordValue) Pos() token.Position { return m.words[0].Pos() }
func (m *multiWordValue) End() token.Position { return m.words[len(m.words)-1].End() }

// Words exposes the phrase for the check package.
func (m *multiWordValue) Words() []string {
	out := make([]string, len(m.words))
	for i, w := range m.words {
		out[i] = strings.ToLower(w.Name())
	}
	return out
}

// MultiWord is implemented by setting values consisting of several
// space-separated identifiers.
type MultiWord interface{ Words() []string }
