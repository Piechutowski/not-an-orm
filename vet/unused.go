// Unused-declaration analyzers — DBML's version of Go's "imported and not
// used". A declared-but-unreferenced enum, partial or alias is usually a
// leftover from a refactor.
package vet

import (
	"strings"

	"github.com/Piechutowski/not-an-orm/ast"
)

func init() {
	register(unusedEnum)
	register(unusedPartial)
	register(unusedAlias)
}

var unusedEnum = &Analyzer{
	Name: "unusedenum",
	Doc:  "reports enums never used as a column type in this file (other files may still use exported enums)",
	Run: func(p *Pass) {
		used := map[string]bool{}
		for _, ti := range p.Info.Tables {
			for _, cd := range ti.Columns {
				t := cd.Col.Type.Name
				if t.Schema() == "" {
					used["public."+t.Base()] = true
				}
				used[t.String()] = true
			}
		}
		for _, e := range p.Info.Enums {
			if !used[e.Key] {
				p.Reportf(e.Decl.Pos(), "enum %q is declared but never used as a column type", e.Decl.Name.String())
			}
		}
	},
}

var unusedPartial = &Analyzer{
	Name: "unusedpartial",
	Doc:  "reports TablePartials never injected into a table in this file",
	Run: func(p *Pass) {
		for _, tp := range p.Info.Partials {
			if tp.Uses == 0 {
				p.Reportf(tp.Decl.Pos(), "TablePartial %q is declared but never injected", tp.Decl.Name.Name())
			}
		}
	},
}

var unusedAlias = &Analyzer{
	Name: "unusedalias",
	Doc:  "reports table aliases that nothing references",
	Run: func(p *Pass) {
		used := map[string]bool{}
		markQual := func(q *ast.QualName) {
			if q.Schema() == "" {
				used[q.Base()] = true
			}
		}
		for _, r := range p.Info.Refs {
			// endpoint tables were written as names; re-walk the syntax to
			// see the spelling actually used
			switch n := r.Node.(type) {
			case *ast.Ref:
				markQual(n.Left.Table)
				markQual(n.Right.Table)
			case *ast.Setting:
				if rv, ok := n.Value.(*ast.RefValue); ok {
					markQual(rv.Endpoint.Table)
				}
			}
		}
		ast.Inspect(p.File, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.TableGroup:
				for _, m := range n.Members {
					markQual(m)
				}
			case *ast.ViewCategory:
				for _, m := range n.Names {
					markQual(m)
				}
			case *ast.Records:
				if n.Table != nil {
					markQual(n.Table)
				}
			}
			return true
		})
		for _, ti := range p.Info.Tables {
			if ti.Alias != "" && !used[ti.Alias] && !strings.EqualFold(ti.Alias, ti.Decl.Name.Base()) {
				p.Reportf(ti.Decl.Alias.Pos(), "alias %q of table %q is never used", ti.Alias, ti.Decl.Name.String())
			}
		}
	},
}
