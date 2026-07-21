// Dynamic-layer name analyzer — feature DYN-7: the dynamic query layer
// (decisions D28-D30) mints package-scope Go names by concatenation
// (UserCols, UserLimit), so distinct DBML declarations can demand one Go
// name. Generation refuses such schemas outright; this rule surfaces the
// collision at vet time with both origins named. The name derivation is
// the generator's own (gen/golang), so rule and generator cannot drift.
package vet

import golang "github.com/Piechutowski/not-an-orm/gen/golang"

func init() { register(dynName) }

var dynName = &Analyzer{
	Name: "dynname",
	Doc:  "reports declarations whose generated dynamic-query names (column-handle sets, option wrappers) collide in Go package scope",
	Run: func(p *Pass) {
		for _, c := range golang.DynNameCollisions(p.File, p.Info) {
			p.Reportf(c.Pos, "%s and %s both generate the Go name %s; rename one (e.g. with [model: '...'])",
				c.First, c.Second, c.Name)
		}
	},
}
