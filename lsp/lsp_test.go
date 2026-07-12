package lsp

import (
	"strings"
	"testing"

	protocol "github.com/tliron/glsp/protocol_3_16"

	"github.com/Piechutowski/not-an-orm/token"
)

const sample = `Enum status {
  active [note: 'ok']
  retired
}

TablePartial base {
  id int [pk]
}

Table users as U [note: 'people'] {
  ~base
  name varchar(255) [not null, unique]
  status status [default: status.active]
}

Table core.orders {
  id int [pk]
  user_id int
  owner_name varchar(255) [ref: > U.name]
}

Ref: users.id < core.orders.user_id

TableGroup g {
  users
  core.orders
}
`

func doc(t *testing.T, text string) *Document {
	t.Helper()
	return NewDocument("file:///test.dbml", text)
}

// posOf returns the LSP position of the i-th byte of the needle's n-th
// occurrence (0-based occurrence index).
func posOf(t *testing.T, text, needle string, occurrence int) protocol.Position {
	t.Helper()
	off := -1
	for i := 0; i <= occurrence; i++ {
		next := strings.Index(text[off+1:], needle)
		if next < 0 {
			t.Fatalf("needle %q occurrence %d not found", needle, occurrence)
		}
		off += 1 + next
	}
	line := strings.Count(text[:off], "\n")
	col := off - (strings.LastIndex(text[:off], "\n") + 1)
	return protocol.Position{Line: protocol.UInteger(line), Character: protocol.UInteger(col)}
}

func TestCleanSampleHasNoDiagnostics(t *testing.T) {
	d := doc(t, sample)
	if len(d.Diags) != 0 {
		t.Fatalf("expected clean sample, got %v", d.Diags)
	}
}

func TestDiagnosticsOnError(t *testing.T) {
	d := doc(t, "Table t {\n  id int [pk\n}\n")
	got := d.LSPDiagnostics()
	if len(got) == 0 {
		t.Fatal("expected diagnostics for unclosed settings")
	}
	if *got[0].Severity != protocol.DiagnosticSeverityError {
		t.Errorf("expected error severity, got %v", *got[0].Severity)
	}
}

func TestVetWarningsSurface(t *testing.T) {
	d := doc(t, "Table t {\n  id int [pk, null]\n}\n")
	found := false
	for _, dg := range d.Diags {
		if strings.HasPrefix(dg.Code, "vet/") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a vet warning, got %v", d.Diags)
	}
}

func TestModelnameAnalyzerExcluded(t *testing.T) {
	// `status` has an -us ending that trips vet/modelname; the LSP must not
	// run that analyzer because [model:] is EDBML-only.
	d := doc(t, "Table status {\n  id int [pk]\n}\nTable other {\n  id int [pk]\n  s int [ref: > status.id]\n}\n")
	for _, dg := range d.Diags {
		if strings.Contains(dg.Code, "modelname") {
			t.Fatalf("modelname analyzer should be excluded, got %v", dg)
		}
	}
}

func TestDefinitionFromRefEndpoint(t *testing.T) {
	d := doc(t, sample)
	// "users" inside `Ref: users.id < ...`
	pos := posOf(t, sample, "users.id <", 0)
	loc := d.Definition(pos)
	if loc == nil {
		t.Fatal("no definition for ref endpoint table")
	}
	want := posOf(t, sample, "Table users as U", 0)
	if loc.Range.Start.Line != want.Line+0 {
		t.Errorf("definition line = %d, want %d", loc.Range.Start.Line, want.Line)
	}
}

func TestDefinitionAliasResolvesToTable(t *testing.T) {
	d := doc(t, sample)
	pos := posOf(t, sample, "U.name", 0) // inline ref via alias
	loc := d.Definition(pos)
	if loc == nil {
		t.Fatal("no definition via alias")
	}
	declPos := posOf(t, sample, "users", 0) // decl in `Table users as U`
	if loc.Range.Start.Line != declPos.Line {
		t.Errorf("alias definition line = %d, want %d", loc.Range.Start.Line, declPos.Line)
	}
}

func TestDefinitionPartialInjection(t *testing.T) {
	d := doc(t, sample)
	pos := posOf(t, sample, "~base", 0)
	pos.Character += 2 // inside "base"
	loc := d.Definition(pos)
	if loc == nil {
		t.Fatal("no definition for ~base")
	}
	want := posOf(t, sample, "TablePartial base", 0)
	if loc.Range.Start.Line != want.Line {
		t.Errorf("partial definition line = %d, want %d", loc.Range.Start.Line, want.Line)
	}
}

func TestDefinitionEnumType(t *testing.T) {
	d := doc(t, sample)
	// second "status" on the column line `status status [...]` is the type
	pos := posOf(t, sample, "status status", 0)
	pos.Character += protocol.UInteger(len("status "))
	loc := d.Definition(pos)
	if loc == nil {
		t.Fatal("no definition for enum type reference")
	}
	if loc.Range.Start.Line != 0 {
		t.Errorf("enum definition line = %d, want 0", loc.Range.Start.Line)
	}
}

func TestReferencesForTable(t *testing.T) {
	d := doc(t, sample)
	pos := posOf(t, sample, "Table users as U", 0)
	pos.Character += protocol.UInteger(len("Table "))
	refs := d.References(pos, true)
	// decl + alias decl + ref endpoint + tablegroup member = at least 4
	if len(refs) < 4 {
		t.Fatalf("expected >= 4 references for users, got %d: %v", len(refs), refs)
	}
}

func TestRenameColumnFromPartial(t *testing.T) {
	d := doc(t, sample)
	// rename `id` at its declaration inside the partial
	pos := posOf(t, sample, "id int [pk]", 0)
	edit, err := d.Rename(pos, "uid")
	if err != nil {
		t.Fatal(err)
	}
	edits := edit.Changes["file:///test.dbml"]
	// decl in partial + `users.id` ref endpoint; core.orders.id is a
	// DIFFERENT column and must not be touched.
	if len(edits) != 2 {
		t.Fatalf("expected 2 edits, got %d: %v", len(edits), edits)
	}
}

func TestRenameRejectsInvalidName(t *testing.T) {
	d := doc(t, sample)
	pos := posOf(t, sample, "Table users", 0)
	pos.Character += protocol.UInteger(len("Table "))
	if _, err := d.Rename(pos, "not valid!"); err == nil {
		t.Fatal("expected error for invalid identifier")
	}
	if _, err := d.Rename(pos, "1234"); err == nil {
		t.Fatal("expected error for all-digit name")
	}
}

func TestHoverTable(t *testing.T) {
	d := doc(t, sample)
	pos := posOf(t, sample, "users.id <", 0)
	h := d.Hover(pos)
	if h == nil {
		t.Fatal("no hover for table reference")
	}
	md := h.Contents.(protocol.MarkupContent).Value
	for _, want := range []string{"Table users as U", "name varchar(255)", "people", "from ~base"} {
		if !strings.Contains(md, want) {
			t.Errorf("hover missing %q:\n%s", want, md)
		}
	}
}

func TestHoverEnumValue(t *testing.T) {
	d := doc(t, sample)
	pos := posOf(t, sample, "status.active", 0)
	pos.Character += protocol.UInteger(len("status."))
	h := d.Hover(pos)
	if h == nil {
		t.Fatal("no hover for enum value")
	}
	md := h.Contents.(protocol.MarkupContent).Value
	if !strings.Contains(md, "status.active") || !strings.Contains(md, "ok") {
		t.Errorf("unexpected enum value hover:\n%s", md)
	}
}

func TestDocumentSymbols(t *testing.T) {
	d := doc(t, sample)
	syms := d.DocumentSymbols()
	var names []string
	for _, s := range syms {
		names = append(names, s.Name)
	}
	joined := strings.Join(names, " ")
	for _, want := range []string{"status", "base", "users", "core.orders", "g"} {
		if !strings.Contains(joined, want) {
			t.Errorf("outline missing %q: %v", want, names)
		}
	}
	// users has children: ~base injection, name, status columns
	for _, s := range syms {
		if s.Name == "users" && len(s.Children) < 3 {
			t.Errorf("users should have >= 3 children, got %d", len(s.Children))
		}
	}
}

func TestCompletionTopLevel(t *testing.T) {
	text := sample + "\nT"
	d := doc(t, text)
	items := d.Complete(protocol.Position{Line: protocol.UInteger(strings.Count(text, "\n")), Character: 1})
	if !hasLabel(items, "Table") || !hasLabel(items, "TablePartial") {
		t.Fatalf("top-level completion missing element keywords: %v", labels(items))
	}
}

func TestCompletionColumnSettings(t *testing.T) {
	text := "Table t {\n  id int [p\n}\n"
	d := doc(t, text)
	items := d.Complete(protocol.Position{Line: 1, Character: protocol.UInteger(len("  id int [p"))})
	if !hasLabel(items, "pk") || !hasLabel(items, "not null") {
		t.Fatalf("column settings completion missing: %v", labels(items))
	}
}

func TestCompletionRefEndpointColumns(t *testing.T) {
	text := sample + "\nRef: x.y > users."
	d := doc(t, text)
	line := protocol.UInteger(strings.Count(text, "\n"))
	items := d.Complete(protocol.Position{Line: line, Character: protocol.UInteger(len("Ref: x.y > users."))})
	if !hasLabel(items, "name") || !hasLabel(items, "id") {
		t.Fatalf("ref endpoint completion missing columns: %v", labels(items))
	}
}

func TestCompletionInlineRefTables(t *testing.T) {
	text := "Table a {\n  x int [ref: > \n}\n" + sample
	d := doc(t, text)
	items := d.Complete(protocol.Position{Line: 1, Character: protocol.UInteger(len("  x int [ref: > "))})
	if !hasLabel(items, "users") || !hasLabel(items, "core.orders") {
		t.Fatalf("inline ref completion missing tables: %v", labels(items))
	}
}

func TestCompletionPartialInjection(t *testing.T) {
	text := sample + "\nTable extra {\n  ~\n}\n"
	d := doc(t, text)
	line := protocol.UInteger(strings.Count(sample, "\n") + 2)
	items := d.Complete(protocol.Position{Line: line, Character: 3})
	if !hasLabel(items, "base") {
		t.Fatalf("~ completion missing partial: %v", labels(items))
	}
}

func TestCompletionTypesIncludeEnums(t *testing.T) {
	text := sample + "\nTable extra {\n  col v\n}\n"
	d := doc(t, text)
	line := protocol.UInteger(strings.Count(sample, "\n") + 2)
	items := d.Complete(protocol.Position{Line: line, Character: protocol.UInteger(len("  col v"))})
	if !hasLabel(items, "varchar") || !hasLabel(items, "status") {
		t.Fatalf("type completion missing: %v", labels(items))
	}
}

func TestUTF16Conversion(t *testing.T) {
	// 𝔘 is a surrogate pair (2 UTF-16 units, 4 UTF-8 bytes)
	text := "Table \"𝔘ser\" {\n  id int [pk]\n}\n"
	d := doc(t, text)
	if len(d.Diags) != 0 {
		t.Fatalf("unexpected diagnostics: %v", d.Diags)
	}
	// position of `id`: line 1, char 2 in both encodings
	off := d.FromLSP(protocol.Position{Line: 1, Character: 2})
	if d.Text[off:off+2] != "id" {
		t.Errorf("FromLSP landed on %q", d.Text[off:off+2])
	}
	// round-trip a position after the surrogate pair on line 0
	lsp := d.ToLSP(tokenPosition(t, d, `"𝔘ser"`))
	if lsp.Line != 0 || lsp.Character != protocol.UInteger(len(`Table `)) {
		t.Errorf("ToLSP = %+v", lsp)
	}
}

func tokenPosition(t *testing.T, d *Document, needle string) (p token.Position) {
	t.Helper()
	off := strings.Index(d.Text, needle)
	if off < 0 {
		t.Fatalf("needle %q not found", needle)
	}
	p.Offset = off
	p.Line = strings.Count(d.Text[:off], "\n") + 1
	// column in runes
	lineStart := strings.LastIndex(d.Text[:off], "\n") + 1
	p.Column = len([]rune(d.Text[lineStart:off])) + 1
	return p
}

func hasLabel(items []protocol.CompletionItem, label string) bool {
	for _, it := range items {
		if it.Label == label {
			return true
		}
	}
	return false
}

func labels(items []protocol.CompletionItem) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.Label
	}
	return out
}
