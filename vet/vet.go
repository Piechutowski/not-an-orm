// Package vet reports DBML that is legal but suspicious — the analogue of
// go vet. Where the check package answers "is this valid DBML?", vet
// answers "is this what you meant?": redundancy, silent overrides, likely
// modeling mistakes.
//
// The design follows golang.org/x/tools/go/analysis: each check is an
// *Analyzer value with a name, a doc string and a Run function over a
// *Pass. Analyzers are independent of each other and of the CLI; they see
// the AST plus the check package's resolved Info, so nothing is computed
// twice. Adding a check is one file and one registry entry.
package vet

import (
	"github.com/Piechutowski/not-an-orm/ast"
	"github.com/Piechutowski/not-an-orm/check"
	"github.com/Piechutowski/not-an-orm/diag"
	"github.com/Piechutowski/not-an-orm/token"
)

// Pass carries everything an analyzer may inspect for one file.
type Pass struct {
	File *ast.File
	Info *check.Info

	analyzer *Analyzer
	diags    *[]diag.Diagnostic
}

// Reportf records a warning attributed to the running analyzer.
func (p *Pass) Reportf(pos token.Position, format string, args ...any) {
	*p.diags = append(*p.diags, diag.Warningf(pos, "vet/"+p.analyzer.Name, format, args...))
}

// Analyzer is one vet check.
type Analyzer struct {
	Name string // short lower-case identifier, used in diagnostic codes
	Doc  string // one-sentence description of what it reports
	Run  func(*Pass)
}

// All returns the registered analyzers in a stable order.
func All() []*Analyzer {
	out := make([]*Analyzer, len(registry))
	copy(out, registry)
	return out
}

// ByName returns the named analyzer, or nil.
func ByName(name string) *Analyzer {
	for _, a := range registry {
		if a.Name == name {
			return a
		}
	}
	return nil
}

var registry []*Analyzer

func register(a *Analyzer) { registry = append(registry, a) }

// Run executes the given analyzers (all registered ones if none are named)
// over a checked file and returns their warnings, sorted by position.
func Run(f *ast.File, info *check.Info, analyzers ...*Analyzer) []diag.Diagnostic {
	if len(analyzers) == 0 {
		analyzers = All()
	}
	var diags []diag.Diagnostic
	for _, a := range analyzers {
		pass := &Pass{File: f, Info: info, analyzer: a, diags: &diags}
		a.Run(pass)
	}
	diag.Sort(diags)
	return diags
}
