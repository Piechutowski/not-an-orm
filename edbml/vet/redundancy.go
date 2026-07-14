// Redundancy analyzers: constructs that repeat what the language already
// guarantees, or repeat each other. Redundancy is how two definitions
// drift apart later.
package vet

import (
	"fmt"
	"strings"

	"github.com/Piechutowski/not-an-orm/edbml/ast"
	"github.com/Piechutowski/not-an-orm/edbml/check"
)

func init() {
	register(redundantNull)
	register(redundantIndex)
	register(emptyBody)
	register(legacyFlag)
	register(shadowedColumn)
	register(projectNotes)
	register(unnecessaryQuotes)
}

var redundantNull = &Analyzer{
	Name: "redundantnull",
	Doc:  "reports explicit [null] settings; columns are nullable by default (spec §6.3.2)",
	Run: func(p *Pass) {
		ast.Inspect(p.File, func(n ast.Node) bool {
			col, ok := n.(*ast.Column)
			if !ok || col.Settings == nil {
				return true
			}
			if s := col.Settings.Get("null"); s != nil {
				p.Reportf(s.Pos(), "explicit 'null' is redundant; columns are nullable by default")
			}
			return true
		})
	},
}

var redundantIndex = &Analyzer{
	Name: "redundantindex",
	Doc:  "reports indexes that duplicate a column setting, another index, or combine pk with unique",
	Run: func(p *Pass) {
		for _, ti := range p.Info.Tables {
			seenKeys := map[string]bool{}
			for _, ix := range ti.Indexes {
				if ix.Settings.Get("pk") != nil && ix.Settings.Get("unique") != nil {
					p.Reportf(ix.Pos(), "combining 'pk' and 'unique' on one index is redundant; a primary key is already unique")
				}
				key := indexKey(ix)
				if seenKeys[key] {
					p.Reportf(ix.Pos(), "duplicate index on %s in table %q", key, ti.Decl.Name.String())
				}
				seenKeys[key] = true

				// single-column index repeating a column-level constraint
				if len(ix.Key) != 1 {
					continue
				}
				id, ok := ix.Key[0].(*ast.Ident)
				if !ok {
					continue
				}
				cd := ti.Column(id.Name())
				if cd == nil {
					continue
				}
				if ix.Settings.Get("unique") != nil && columnHas(cd, "unique") {
					p.Reportf(ix.Pos(), "unique index on %q duplicates the column's 'unique' setting", id.Name())
				}
				if ix.Settings.Get("unique") != nil && columnHas(cd, "pk") {
					p.Reportf(ix.Pos(), "unique index on %q is redundant; the column is already the primary key", id.Name())
				}
				if ix.Settings.Get("pk") != nil && columnHas(cd, "pk") {
					p.Reportf(ix.Pos(), "pk index on %q duplicates the column's 'pk' setting", id.Name())
				}
			}
		}
	},
}

// indexKey canonicalizes an index key for duplicate detection: column order
// is significant for composite indexes, so it is preserved.
func indexKey(ix *ast.Index) string {
	parts := make([]string, 0, len(ix.Key))
	for _, a := range ix.Key {
		switch a := a.(type) {
		case *ast.Ident:
			parts = append(parts, a.Name())
		case *ast.FuncExpr:
			parts = append(parts, "`"+a.Text()+"`")
		}
	}
	return "(" + strings.Join(parts, ", ") + ")"
}

// columnHas reports whether the column carries the given canonical setting,
// via the settings list or a legacy flag.
func columnHas(cd *check.ColumnDef, name string) bool {
	if cd.Col.Settings != nil {
		if cd.Col.Settings.Get(name) != nil {
			return true
		}
		if name == "pk" && cd.Col.Settings.Get("primary key") != nil {
			return true
		}
	}
	for _, f := range cd.Col.LegacyFlags {
		if strings.EqualFold(f.Name(), name) {
			return true
		}
	}
	return false
}

var emptyBody = &Analyzer{
	Name: "emptybody",
	Doc:  "reports empty indexes/checks blocks, TableGroups, DiagramView categories and Project bodies",
	Run: func(p *Pass) {
		ast.Inspect(p.File, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.IndexesBlock:
				if len(n.Indexes) == 0 {
					p.Reportf(n.Pos(), "empty indexes block")
				}
			case *ast.ChecksBlock:
				if len(n.Checks) == 0 {
					p.Reportf(n.Pos(), "empty checks block")
				}
			case *ast.TableGroup:
				if len(n.Members) == 0 {
					p.Reportf(n.Pos(), "TableGroup %q has no members", n.Name.Name())
				}
			case *ast.ViewCategory:
				if !n.Wildcard && len(n.Names) == 0 {
					p.Reportf(n.Pos(), "empty %s category selects nothing", n.Kind.Name())
				}
			case *ast.Project:
				if len(n.Props) == 0 && len(n.Notes) == 0 {
					p.Reportf(n.Pos(), "empty Project body")
				}
			}
			return true
		})
	},
}

var legacyFlag = &Analyzer{
	Name: "legacyflag",
	Doc:  "reports legacy bare pk/unique flags between type and settings; prefer the settings list (spec §6.3.7)",
	Run: func(p *Pass) {
		ast.Inspect(p.File, func(n ast.Node) bool {
			col, ok := n.(*ast.Column)
			if !ok {
				return true
			}
			for _, f := range col.LegacyFlags {
				p.Reportf(f.Pos(), "legacy flag %q; write it in the settings list: [%s]", f.Name(), strings.ToLower(f.Name()))
			}
			return true
		})
	},
}

var shadowedColumn = &Analyzer{
	Name: "shadowedcolumn",
	Doc:  "reports column definitions silently overridden by spec §6.9.4 partial-injection conflict resolution",
	Run: func(p *Pass) {
		for _, ti := range p.Info.Tables {
			for _, sh := range ti.Shadowed {
				winner := ti.Column(sh.Col.Name.Name())
				if winner == nil {
					continue
				}
				p.Reportf(winner.Col.Pos(), "column %q in table %q silently overrides the definition from %s",
					sh.Col.Name.Name(), ti.Decl.Name.String(), originOf(sh))
			}
		}
	},
}

func originOf(cd *check.ColumnDef) string {
	if cd.Partial == nil {
		return "the table body"
	}
	return fmt.Sprintf("TablePartial %q", cd.Partial.Name.Name())
}

var projectNotes = &Analyzer{
	Name: "projectnotes",
	Doc:  "reports multiple Note definitions in a Project body; only one survives in tools",
	Run: func(p *Pass) {
		for _, d := range p.File.Decls {
			if pr, ok := d.(*ast.Project); ok && len(pr.Notes) > 1 {
				p.Reportf(pr.Notes[1].Pos(), "Project has %d Note definitions; tools keep only one", len(pr.Notes))
			}
		}
	},
}

var unnecessaryQuotes = &Analyzer{
	Name: "unnecessaryquotes",
	Doc:  "reports quoted identifiers that are already valid plain identifiers",
	Run: func(p *Pass) {
		seen := map[string]bool{} // dedupe repeated spellings per line
		ast.Inspect(p.File, func(n ast.Node) bool {
			id, ok := n.(*ast.Ident)
			if !ok || !id.Quoted() || !isPlainIdent(id.Name()) {
				return true
			}
			key := fmt.Sprintf("%d:%s", id.Pos().Line, id.Name())
			if seen[key] {
				return true
			}
			seen[key] = true
			p.Reportf(id.Pos(), "identifier %q does not need quotes", id.Name())
			return true
		})
	},
}

func isPlainIdent(s string) bool {
	if s == "" {
		return false
	}
	hasLetter := false
	for _, r := range s {
		switch {
		case r == '_' || ('a' <= r && r <= 'z') || ('A' <= r && r <= 'Z'):
			hasLetter = true
		case '0' <= r && r <= '9':
		default:
			return false
		}
	}
	return hasLetter
}
