// Package diag defines the diagnostic type shared by the parser, the
// semantic checker and vet analyzers, so each stage stays decoupled while
// producing a uniform stream of findings.
package diag

import (
	"fmt"
	"sort"

	"github.com/Piechutowski/not-an-orm/edbml/token"
)

type Severity int

const (
	Error Severity = iota
	Warning
)

func (s Severity) String() string {
	if s == Warning {
		return "warning"
	}
	return "error"
}

// Diagnostic is one finding. Code identifies the rule that produced it:
// syntax errors use "syntax", semantic errors cite their spec section
// (e.g. "spec/6.7"), vet warnings carry the analyzer name (e.g. "vet/unusedenum").
type Diagnostic struct {
	Pos      token.Position
	Severity Severity
	Code     string
	Msg      string
}

func (d Diagnostic) String() string {
	return fmt.Sprintf("%s: %s: %s: %s", d.Pos, d.Severity, d.Code, d.Msg)
}

// Errorf builds an error diagnostic.
func Errorf(pos token.Position, code, format string, args ...any) Diagnostic {
	return Diagnostic{Pos: pos, Severity: Error, Code: code, Msg: fmt.Sprintf(format, args...)}
}

// Warningf builds a warning diagnostic.
func Warningf(pos token.Position, code, format string, args ...any) Diagnostic {
	return Diagnostic{Pos: pos, Severity: Warning, Code: code, Msg: fmt.Sprintf(format, args...)}
}

// Sort orders diagnostics by source position, errors before warnings on ties.
func Sort(ds []Diagnostic) {
	sort.SliceStable(ds, func(i, j int) bool {
		a, b := ds[i], ds[j]
		if a.Pos.Line != b.Pos.Line {
			return a.Pos.Line < b.Pos.Line
		}
		if a.Pos.Column != b.Pos.Column {
			return a.Pos.Column < b.Pos.Column
		}
		return a.Severity < b.Severity
	})
}

// HasErrors reports whether any diagnostic is an error.
func HasErrors(ds []Diagnostic) bool {
	for _, d := range ds {
		if d.Severity == Error {
			return true
		}
	}
	return false
}
