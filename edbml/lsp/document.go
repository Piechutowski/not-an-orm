// Package lsp implements the EDBML language server. It wraps the DBML front
// end (scanner, parser, check, vet) behind the Language Server Protocol:
// diagnostics, completion, hover, definition, references, rename and
// document symbols. One Document per open file holds the parse result, the
// semantic model and the symbol occurrence index.
package lsp

import (
	"strings"
	"unicode/utf16"

	protocol "github.com/tliron/glsp/protocol_3_16"

	"github.com/Piechutowski/not-an-orm/edbml/ast"
	"github.com/Piechutowski/not-an-orm/edbml/check"
	"github.com/Piechutowski/not-an-orm/edbml/diag"
	"github.com/Piechutowski/not-an-orm/edbml/parser"
	"github.com/Piechutowski/not-an-orm/edbml/token"
	"github.com/Piechutowski/not-an-orm/edbml/vet"
)

// Document is one open text document plus everything derived from it.
type Document struct {
	URI  string
	Text string

	File  *ast.File
	Info  *check.Info
	Diags []diag.Diagnostic
	Index *Index

	lineOffsets []int // byte offset of the start of each line
}

// NewDocument analyzes text and returns a ready document.
func NewDocument(uri, text string) *Document {
	d := &Document{URI: uri}
	d.Update(text)
	return d
}

// Update replaces the document text and re-runs the whole front end.
func (d *Document) Update(text string) {
	d.Text = text
	d.lineOffsets = d.lineOffsets[:0]
	d.lineOffsets = append(d.lineOffsets, 0)
	for i, b := range []byte(text) {
		if b == '\n' {
			d.lineOffsets = append(d.lineOffsets, i+1)
		}
	}

	file, diags := parser.ParseFile(pathFromURI(d.URI), text)
	info, semDiags := check.File(file)
	diags = append(diags, semDiags...)
	// vet warnings only make sense on files that already check clean;
	// stacking style advice on top of hard errors is noise while typing.
	if !diag.HasErrors(diags) {
		diags = append(diags, vet.Run(file, info, activeAnalyzers()...)...)
	}
	diag.Sort(diags)

	d.File = file
	d.Info = info
	d.Diags = diags
	d.Index = BuildIndex(file, info)
}

// activeAnalyzers is every registered vet analyzer except modelname: the
// [model:] setting it wants is part of EDBML, which this DBML server does
// not implement yet (see docs/editor-architecture.md).
func activeAnalyzers() []*vet.Analyzer {
	var out []*vet.Analyzer
	for _, a := range vet.All() {
		if a.Name != "modelname" {
			out = append(out, a)
		}
	}
	return out
}

// pathFromURI strips the file:// scheme for prettier diagnostics.
func pathFromURI(uri string) string {
	return strings.TrimPrefix(uri, "file://")
}

// ---------------------------------------------------------------------------
// Position conversion. token.Position counts 1-based lines and 1-based rune
// columns; LSP counts 0-based lines and 0-based UTF-16 code units.

// lineText returns the 0-based line without its trailing newline.
func (d *Document) lineText(line int) string {
	if line < 0 || line >= len(d.lineOffsets) {
		return ""
	}
	start := d.lineOffsets[line]
	end := len(d.Text)
	if line+1 < len(d.lineOffsets) {
		end = d.lineOffsets[line+1] - 1
	}
	if start > end {
		return ""
	}
	return d.Text[start:end]
}

// ToLSP converts a front-end position to an LSP position.
func (d *Document) ToLSP(p token.Position) protocol.Position {
	line := p.Line - 1
	if line < 0 {
		return protocol.Position{}
	}
	text := d.lineText(line)
	col := 0
	need := p.Column - 1
	for _, r := range text {
		if need <= 0 {
			break
		}
		col += len(utf16.Encode([]rune{r}))
		need--
	}
	return protocol.Position{Line: protocol.UInteger(line), Character: protocol.UInteger(col)}
}

// FromLSP converts an LSP position to a byte offset into d.Text.
func (d *Document) FromLSP(pos protocol.Position) int {
	line := int(pos.Line)
	if line >= len(d.lineOffsets) {
		return len(d.Text)
	}
	text := d.lineText(line)
	need := int(pos.Character)
	byteCol := 0
	for _, r := range text {
		if need <= 0 {
			break
		}
		u := len(utf16.Encode([]rune{r}))
		if u > need {
			break
		}
		need -= u
		byteCol += len(string(r))
	}
	return d.lineOffsets[line] + byteCol
}

// RangeOf converts a node's extent to an LSP range.
func (d *Document) RangeOf(n ast.Node) protocol.Range {
	return protocol.Range{Start: d.ToLSP(n.Pos()), End: d.ToLSP(n.End())}
}

// tokenRange converts a token's extent to an LSP range.
func (d *Document) tokenRange(t token.Token) protocol.Range {
	return protocol.Range{Start: d.ToLSP(t.Pos), End: d.ToLSP(t.End())}
}

// isIdentByte loosely matches identifier bytes for diagnostic ranges.
func isIdentByte(b byte) bool {
	return b == '_' || b >= '0' && b <= '9' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= 0x80
}

// diagnosticRange widens a diagnostic's point position to the token-ish
// span starting there, so editors underline something visible.
func (d *Document) diagnosticRange(p token.Position) protocol.Range {
	start := d.ToLSP(p)
	off := p.Offset
	end := off
	for end < len(d.Text) && isIdentByte(d.Text[end]) {
		end++
	}
	if end == off && off < len(d.Text) && d.Text[off] != '\n' {
		end = off + 1
	}
	width := end - off
	return protocol.Range{
		Start: start,
		End:   protocol.Position{Line: start.Line, Character: start.Character + protocol.UInteger(width)},
	}
}
