// Relationship and modeling analyzers: legal schemas that are likely to
// misbehave as databases — missing primary keys, foreign keys onto
// non-unique columns, type drift across a relationship, dependency cycles.
package vet

import (
	"sort"
	"strings"

	"github.com/Piechutowski/not-an-orm/edbml/check"
	"github.com/Piechutowski/not-an-orm/edbml/token"
)

func init() {
	register(missingPK)
	register(refTypeMismatch)
	register(refTargetNotUnique)
	register(selfRef)
	register(refCycle)
	register(caseCollision)
}

var missingPK = &Analyzer{
	Name: "missingpk",
	Doc:  "reports tables with no primary key (column setting or pk index)",
	Run: func(p *Pass) {
		for _, ti := range p.Info.Tables {
			if tableHasPK(ti) {
				continue
			}
			p.Reportf(ti.Decl.Pos(), "table %q has no primary key", ti.Decl.Name.String())
		}
	},
}

func tableHasPK(ti *check.TableInfo) bool {
	for _, cd := range ti.Columns {
		if columnHas(cd, "pk") {
			return true
		}
	}
	for _, ix := range ti.Indexes {
		if ix.Settings.Get("pk") != nil {
			return true
		}
	}
	return false
}

// fkSides returns the foreign-key endpoint and the referenced endpoint of a
// relationship, following spec §6.7.3, or ok=false for many-to-many.
func fkSides(r *check.RefInfo) (fk, target check.EndpointInfo, ok bool) {
	switch r.Op {
	case token.GT: // many-to-one: left is the FK
		return r.Left, r.Right, true
	case token.LT: // one-to-many: right is the FK
		return r.Right, r.Left, true
	case token.MINUS: // one-to-one: the second endpoint is the FK
		return r.Right, r.Left, true
	default: // <> has no FK side
		return fk, target, false
	}
}

var refTypeMismatch = &Analyzer{
	Name: "reftype",
	Doc:  "reports relationships whose two columns have different declared types",
	Run: func(p *Pass) {
		for _, r := range p.Info.Refs {
			if r.Left.Table == nil || r.Right.Table == nil {
				continue
			}
			n := min(len(r.Left.Columns), len(r.Right.Columns))
			for i := 0; i < n; i++ {
				lc := r.Left.Table.Column(r.Left.Columns[i])
				rc := r.Right.Table.Column(r.Right.Columns[i])
				if lc == nil || rc == nil {
					continue
				}
				lt, rt := lc.Col.Type.String(), rc.Col.Type.String()
				if !strings.EqualFold(lt, rt) {
					p.Reportf(r.Node.Pos(), "relationship joins %s.%s (%s) to %s.%s (%s); the types differ",
						r.Left.Table.Decl.Name.String(), r.Left.Columns[i], lt,
						r.Right.Table.Decl.Name.String(), r.Right.Columns[i], rt)
				}
			}
		}
	},
}

var refTargetNotUnique = &Analyzer{
	Name: "reftarget",
	Doc:  "reports foreign keys referencing columns that are not pk or unique on the one side",
	Run: func(p *Pass) {
		for _, r := range p.Info.Refs {
			fk, target, ok := fkSides(r)
			_ = fk
			if !ok || target.Table == nil {
				continue
			}
			if endpointUnique(target) {
				continue
			}
			p.Reportf(r.Node.Pos(), "referenced column(s) %s.(%s) are not pk or unique; the database cannot enforce this foreign key",
				target.Table.Decl.Name.String(), strings.Join(target.Columns, ", "))
		}
	},
}

// endpointUnique reports whether the endpoint's column set is guaranteed
// unique: a single pk/unique column, or a matching composite pk/unique index.
func endpointUnique(e check.EndpointInfo) bool {
	if len(e.Columns) == 1 {
		if cd := e.Table.Column(e.Columns[0]); cd != nil {
			if columnHas(cd, "pk") || columnHas(cd, "unique") {
				return true
			}
		}
	}
	want := append([]string(nil), e.Columns...)
	sort.Strings(want)
	for _, ix := range e.Table.Indexes {
		if ix.Settings.Get("pk") == nil && ix.Settings.Get("unique") == nil {
			continue
		}
		var got []string
		for _, a := range ix.Key {
			if id, ok := a.(interface{ Name() string }); ok {
				got = append(got, id.Name())
			}
		}
		sort.Strings(got)
		if len(got) == len(want) && strings.Join(got, ",") == strings.Join(want, ",") {
			return true
		}
	}
	return false
}

var selfRef = &Analyzer{
	Name: "selfref",
	Doc:  "reports relationships whose two endpoints are the same column",
	Run: func(p *Pass) {
		for _, r := range p.Info.Refs {
			if r.Left.Table == nil || r.Right.Table == nil || r.Left.Table != r.Right.Table {
				continue
			}
			if strings.Join(r.Left.Columns, ",") == strings.Join(r.Right.Columns, ",") {
				p.Reportf(r.Node.Pos(), "relationship references column %s.%s on both sides",
					r.Left.Table.Decl.Name.String(), strings.Join(r.Left.Columns, ","))
			}
		}
	},
}

var refCycle = &Analyzer{
	Name: "refcycle",
	Doc:  "reports foreign-key dependency cycles between tables; cyclic FKs complicate inserts and deletes",
	Run: func(p *Pass) {
		// Build FK edges: table with the FK -> referenced table.
		edges := map[*check.TableInfo][]*check.TableInfo{}
		for _, r := range p.Info.Refs {
			fk, target, ok := fkSides(r)
			if !ok || fk.Table == nil || target.Table == nil || fk.Table == target.Table {
				continue
			}
			edges[fk.Table] = append(edges[fk.Table], target.Table)
		}
		const (
			white = 0
			gray  = 1
			black = 2
		)
		state := map[*check.TableInfo]int{}
		var stack []*check.TableInfo
		var visit func(t *check.TableInfo)
		reported := map[string]bool{}
		visit = func(t *check.TableInfo) {
			state[t] = gray
			stack = append(stack, t)
			for _, next := range edges[t] {
				switch state[next] {
				case white:
					visit(next)
				case gray:
					// found a cycle: slice it out of the stack
					var names []string
					for i := len(stack) - 1; i >= 0; i-- {
						names = append(names, stack[i].Decl.Name.String())
						if stack[i] == next {
							break
						}
					}
					sort.Strings(names)
					key := strings.Join(names, "->")
					if !reported[key] {
						reported[key] = true
						p.Reportf(t.Decl.Pos(), "foreign-key cycle between tables: %s", strings.Join(names, ", "))
					}
				}
			}
			stack = stack[:len(stack)-1]
			state[t] = black
		}
		for _, ti := range p.Info.Tables {
			if state[ti] == white {
				visit(ti)
			}
		}
	},
}

var caseCollision = &Analyzer{
	Name: "casecollision",
	Doc:  "reports names within one namespace that differ only in letter case",
	Run: func(p *Pass) {
		report := func(kind string, names map[string]token.Position) {
			byLower := map[string][]string{}
			for n := range names {
				l := strings.ToLower(n)
				byLower[l] = append(byLower[l], n)
			}
			for _, group := range byLower {
				if len(group) < 2 {
					continue
				}
				// warn at the later declaration; list names in source order
				sort.Slice(group, func(i, j int) bool {
					a, b := names[group[i]], names[group[j]]
					if a.Line != b.Line {
						return a.Line < b.Line
					}
					return a.Column < b.Column
				})
				p.Reportf(names[group[len(group)-1]], "%s names %s differ only in case", kind, strings.Join(group, " and "))
			}
		}
		tables := map[string]token.Position{}
		for _, ti := range p.Info.Tables {
			tables[ti.Key] = ti.Decl.Pos()
		}
		report("table", tables)
		for _, ti := range p.Info.Tables {
			cols := map[string]token.Position{}
			for _, cd := range ti.Columns {
				cols[cd.Col.Name.Name()] = cd.Col.Pos()
			}
			report("column", cols)
		}
		enums := map[string]token.Position{}
		for _, e := range p.Info.Enums {
			enums[e.Key] = e.Decl.Pos()
		}
		report("enum", enums)
	},
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
