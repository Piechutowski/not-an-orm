package parser

import (
	"testing"

	"github.com/Piechutowski/not-an-orm/edbml/ast"
	"github.com/Piechutowski/not-an-orm/edbml/diag"
	"github.com/Piechutowski/not-an-orm/edbml/token"
)

func mustParse(t *testing.T, src string) *ast.File {
	t.Helper()
	f, diags := ParseFile("test.dbml", src)
	if diag.HasErrors(diags) {
		t.Fatalf("ParseFile errors: %v", diags)
	}
	return f
}

func TestTableShape(t *testing.T) {
	f := mustParse(t, `
Table core.users as U [headercolor: #3498DB] {
  id integer [pk, not null]
  name varchar(255)
  Note: 'people'
  indexes {
    (id, name) [unique]
  }
}
`)
	if len(f.Decls) != 1 {
		t.Fatalf("decls = %d, want 1", len(f.Decls))
	}
	tb := f.Decls[0].(*ast.Table)
	if tb.Name.String() != "core.users" || tb.Name.Schema() != "core" {
		t.Errorf("name = %q schema = %q", tb.Name.String(), tb.Name.Schema())
	}
	if tb.Alias == nil || tb.Alias.Name() != "U" {
		t.Error("alias not parsed")
	}
	if tb.Settings.Get("headercolor") == nil {
		t.Error("headercolor setting missing")
	}
	if len(tb.Body) != 4 {
		t.Fatalf("body items = %d, want 4", len(tb.Body))
	}
	col := tb.Body[0].(*ast.Column)
	if col.Settings.Get("not null") == nil {
		t.Error("multi-word setting 'not null' not canonicalized")
	}
	typ := tb.Body[1].(*ast.Column).Type
	if typ.String() != "varchar(255)" {
		t.Errorf("type = %q, want varchar(255)", typ.String())
	}
	ix := tb.Body[3].(*ast.IndexesBlock).Indexes[0]
	if !ix.Composite || len(ix.Key) != 2 {
		t.Errorf("index composite=%v keys=%d", ix.Composite, len(ix.Key))
	}
}

func TestRefForms(t *testing.T) {
	f := mustParse(t, `
Ref fk: a.x > b.y [delete: set null]
Ref { c.p <> d.q }
Table e { z int [ref: - f.w] }
`)
	short := f.Decls[0].(*ast.Ref)
	if short.Name.Name() != "fk" || short.OpTok.Kind != token.GT || short.Long {
		t.Errorf("short ref parsed wrong: %+v", short)
	}
	long := f.Decls[1].(*ast.Ref)
	if !long.Long || long.OpTok.Kind != token.LTGT {
		t.Errorf("long ref parsed wrong")
	}
	col := f.Decls[2].(*ast.Table).Body[0].(*ast.Column)
	rv, ok := col.Settings.Get("ref").Value.(*ast.RefValue)
	if !ok || rv.OpTok.Kind != token.MINUS || rv.Endpoint.Table.String() != "f" {
		t.Errorf("inline one-to-one ref parsed wrong: %#v", col.Settings.Get("ref").Value)
	}
}

func TestRefLongExactlyOne(t *testing.T) {
	_, diags := ParseFile("t", "Ref { a.x > b.y\n c.p > d.q }")
	if !diag.HasErrors(diags) {
		t.Error("two relationships in one Ref must be a syntax error (§6.7)")
	}
}

func TestCompositeEndpoint(t *testing.T) {
	f := mustParse(t, "Ref: s.a.(x, y) > b.(p, q)")
	r := f.Decls[0].(*ast.Ref)
	if r.Left.Table.String() != "s.a" || len(r.Left.Columns) != 2 {
		t.Errorf("left endpoint = %q cols=%d", r.Left.Table.String(), len(r.Left.Columns))
	}
}

func TestRecordsRows(t *testing.T) {
	f := mustParse(t, `
Table t {
  a int
  records (a, b, c) {
    1, 'x', true
    , ,
    -5, s.v, `+"`now()`"+`
  }
}
`)
	rec := f.Decls[0].(*ast.Table).Body[1].(*ast.Records)
	if len(rec.Rows) != 3 {
		t.Fatalf("rows = %d, want 3", len(rec.Rows))
	}
	for i, want := range []int{3, 3, 3} {
		if got := len(rec.Rows[i].Values); got != want {
			t.Errorf("row %d values = %d, want %d", i, got, want)
		}
	}
	if _, ok := rec.Rows[2].Values[0].(*ast.NegNumber); !ok {
		t.Error("negative number not parsed")
	}
	if _, ok := rec.Rows[2].Values[1].(*ast.EnumConst); !ok {
		t.Error("enum constant not parsed")
	}
}

func TestKeywordsAreContextual(t *testing.T) {
	// §3.5: no reserved words.
	mustParse(t, `
Table table {
  note varchar
  indexes int
  ref varchar
}
`)
}

func TestErrorRecovery(t *testing.T) {
	// One bad column must not hide errors elsewhere, and the parser must
	// still deliver the rest of the tree (multiple errors per run).
	f, diags := ParseFile("t", `
Table a {
  id int [
  name varchar
}
Table b {
  ] broken
  x int
}
Enum ok { v }
`)
	if !diag.HasErrors(diags) {
		t.Fatal("want errors")
	}
	if len(diags) < 2 {
		t.Errorf("want at least 2 diagnostics from independent sites, got %d: %v", len(diags), diags)
	}
	var enums int
	for _, d := range f.Decls {
		if _, ok := d.(*ast.Enum); ok {
			enums++
		}
	}
	if enums != 1 {
		t.Errorf("recovery lost the trailing enum: decls=%d", len(f.Decls))
	}
}

func TestPartialAST(t *testing.T) {
	// Even a file that ends mid-table yields the declarations before it.
	f, diags := ParseFile("t", "Table ok { id int }\nTable broken {")
	if !diag.HasErrors(diags) {
		t.Fatal("want errors")
	}
	if len(f.Decls) == 0 {
		t.Error("want partial AST with the first table")
	}
}
