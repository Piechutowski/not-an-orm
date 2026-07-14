// Pass 3: cross-element resolution (§8.3) — relationship endpoints, index
// columns, records columns, TableGroup members, DiagramView members — and
// duplicate-relationship detection (§6.7.9).
//
// When the file has use/reuse imports, names may be defined elsewhere, so
// unresolved-name errors are suppressed (Info.HasImports); everything that
// is resolvable locally is still checked.
package check

import (
	"sort"
	"strings"

	"github.com/Piechutowski/not-an-orm/edbml/ast"
	"github.com/Piechutowski/not-an-orm/edbml/token"
)

func (c *checker) resolve(f *ast.File) {
	lenient := c.info.HasImports

	// Relationships from all three forms.
	for _, d := range f.Decls {
		if r, ok := d.(*ast.Ref); ok {
			ri := &RefInfo{Node: r, Op: r.OpTok.Kind}
			ri.Left = c.endpointResolve(r.Left, lenient)
			ri.Right = c.endpointResolve(r.Right, lenient)
			c.info.Refs = append(c.info.Refs, ri)
		}
	}
	for _, ti := range c.info.Tables {
		for _, cd := range ti.Columns {
			if cd.Col.Settings == nil {
				continue
			}
			for _, s := range cd.Col.Settings.Settings {
				rv, ok := s.Value.(*ast.RefValue)
				if s.Name != "ref" || !ok {
					continue
				}
				ri := &RefInfo{Node: s, Op: rv.OpTok.Kind, Inline: true}
				ri.Left = EndpointInfo{
					Table:   ti,
					Columns: []string{cd.Col.Name.Name()},
					Pos:     s.Pos(),
				}
				ri.Right = c.endpointResolve(rv.Endpoint, lenient)
				if len(ri.Right.Columns) != 1 {
					c.errorf(rv.Endpoint.Pos(), "6.7", "an inline ref endpoint has exactly one column")
				}
				c.info.Refs = append(c.info.Refs, ri)
			}
		}
	}
	c.duplicateRefsCheck()

	// Index keys must name columns of their table (§6.5.2).
	for _, ti := range c.info.Tables {
		for _, ix := range ti.Indexes {
			for _, atom := range ix.Key {
				id, ok := atom.(*ast.Ident)
				if !ok {
					continue
				}
				if ti.Column(id.Name()) == nil {
					c.errorf(id.Pos(), "6.5", "index references unknown column %q in table %q", id.Name(), ti.Decl.Name.String())
				}
			}
		}
	}

	// Records: table and column resolution (§6.10).
	for _, d := range f.Decls {
		if r, ok := d.(*ast.Records); ok {
			c.recordsResolve(r, nil, lenient)
		}
	}
	recordsSeen := map[string]token.Position{}
	for _, ti := range c.info.Tables {
		for _, item := range ti.Decl.Body {
			if r, ok := item.(*ast.Records); ok {
				c.recordsResolve(r, ti, lenient)
			}
		}
	}
	for _, d := range f.Decls {
		if r, ok := d.(*ast.Records); ok && r.Table != nil {
			key := canonKey(r.Table)
			if _, dup := recordsSeen[key]; dup {
				c.errorf(r.Pos(), "6.10", "at most one records block per table (%q)", r.Table.String())
			}
			recordsSeen[key] = r.Pos()
			if ti := c.tableLookup(r.Table); ti != nil {
				for _, item := range ti.Decl.Body {
					if _, ok := item.(*ast.Records); ok {
						c.errorf(r.Pos(), "6.10", "table %q already has an in-table records block", r.Table.String())
					}
				}
			}
		}
	}

	// TableGroups: members exist, one group per table (§6.12).
	memberOf := map[string]string{}
	for _, d := range f.Decls {
		g, ok := d.(*ast.TableGroup)
		if !ok {
			continue
		}
		for _, m := range g.Members {
			ti := c.tableLookup(m)
			if ti == nil {
				if !lenient {
					c.errorf(m.Pos(), "6.12", "unknown table %q in TableGroup %q", m.String(), g.Name.Name())
				}
				continue
			}
			if prev, ok := memberOf[ti.Key]; ok {
				c.errorf(m.Pos(), "6.12", "table %q belongs to more than one TableGroup (%q and %q)", m.String(), prev, g.Name.Name())
				continue
			}
			memberOf[ti.Key] = g.Name.Name()
		}
	}

	// DiagramView members (§6.13.3).
	for _, d := range f.Decls {
		v, ok := d.(*ast.DiagramView)
		if !ok {
			continue
		}
		for _, cat := range v.Categories {
			if cat.Wildcard || lenient {
				continue
			}
			for _, nm := range cat.Names {
				switch strings.ToLower(cat.Kind.Name()) {
				case "tables":
					if c.tableLookup(nm) == nil {
						c.errorf(nm.Pos(), "6.13", "unknown table %q in DiagramView", nm.String())
					}
				case "notes":
					if _, ok := c.sticky[nm.String()]; !ok {
						c.errorf(nm.Pos(), "6.13", "unknown sticky note %q in DiagramView", nm.String())
					}
				case "tablegroups":
					if _, ok := c.groups[nm.String()]; !ok {
						c.errorf(nm.Pos(), "6.13", "unknown TableGroup %q in DiagramView", nm.String())
					}
				case "schemas":
					if !c.schemas[nm.String()] {
						c.errorf(nm.Pos(), "6.13", "unknown schema %q in DiagramView", nm.String())
					}
				}
			}
		}
	}
}

// tableLookup resolves a table reference by canonical name or alias.
func (c *checker) tableLookup(q *ast.QualName) *TableInfo {
	return c.info.byTable[canonKey(q)]
}

func (c *checker) endpointResolve(ep *ast.RefEndpoint, lenient bool) EndpointInfo {
	out := EndpointInfo{Pos: ep.Pos()}
	for _, col := range ep.Columns {
		out.Columns = append(out.Columns, col.Name())
	}
	ti := c.tableLookup(ep.Table)
	if ti == nil {
		if !lenient {
			c.errorf(ep.Table.Pos(), "8.3", "unknown table %q in relationship endpoint", ep.Table.String())
		}
		return out
	}
	out.Table = ti
	for _, col := range ep.Columns {
		if ti.Column(col.Name()) == nil {
			c.errorf(col.Pos(), "8.3", "unknown column %q in table %q", col.Name(), ti.Decl.Name.String())
		}
	}
	return out
}

// refKey builds a direction-insensitive identity for a relationship
// (§6.7.9): endpoints are canonicalized and sorted.
func refKey(r *RefInfo) (string, bool) {
	side := func(e EndpointInfo) (string, bool) {
		if e.Table == nil {
			return "", false
		}
		return e.Table.Key + "(" + strings.Join(e.Columns, ",") + ")", true
	}
	l, okL := side(r.Left)
	rr, okR := side(r.Right)
	if !okL || !okR {
		return "", false
	}
	pair := []string{l, rr}
	sort.Strings(pair)
	return pair[0] + "|" + pair[1], true
}

func (c *checker) duplicateRefsCheck() {
	seen := map[string]bool{}
	for _, r := range c.info.Refs {
		key, ok := refKey(r)
		if !ok {
			continue
		}
		if seen[key] {
			c.errorf(r.Node.Pos(), "6.7", "duplicate relationship: the same endpoints are already related")
			continue
		}
		seen[key] = true
	}
}

func (c *checker) recordsResolve(r *ast.Records, ti *TableInfo, lenient bool) {
	if r.Table != nil {
		ti = c.tableLookup(r.Table)
		if ti == nil {
			if !lenient {
				c.errorf(r.Table.Pos(), "6.10", "unknown table %q in records", r.Table.String())
			}
			return
		}
	}
	if ti == nil {
		return
	}
	for _, col := range r.Columns {
		if ti.Column(col.Name()) == nil {
			c.errorf(col.Pos(), "6.10", "column %q does not exist in table %q", col.Name(), ti.Decl.Name.String())
		}
	}
	if !r.HasColumns {
		for _, row := range r.Rows {
			if len(row.Values) != len(ti.Columns) {
				c.errorf(row.Pos(), "6.10", "record row has %d values but table %q has %d columns", len(row.Values), ti.Decl.Name.String(), len(ti.Columns))
			}
		}
	}
}
