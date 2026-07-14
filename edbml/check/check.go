// Package check implements semantic analysis of a parsed DBML file — the
// role go/types plays for Go. It validates every constraint of the spec
// that goes beyond grammar (setting whitelists, duplicate names, reference
// resolution, records arity) and builds an Info symbol table that the vet
// package reuses, so analysis is done once.
//
// Diagnostic codes cite the spec section they enforce, e.g. "spec/6.7".
package check

import (
	"github.com/Piechutowski/not-an-orm/edbml/ast"
	"github.com/Piechutowski/not-an-orm/edbml/diag"
	"github.com/Piechutowski/not-an-orm/edbml/token"
)

// Info is the semantic model of one file, produced by File.
type Info struct {
	Tables     []*TableInfo
	Enums      []*EnumInfo
	Partials   []*PartialInfo
	Refs       []*RefInfo
	HasImports bool

	byTable map[string]*TableInfo // canonical key and alias -> table
}

// TableInfo describes one table with partial injection applied.
type TableInfo struct {
	Decl  *ast.Table
	Key   string // canonical "schema.name" with the public default applied
	Alias string // "" if none

	// Columns is the effective column list in definition order after
	// injection, conflicts resolved per spec §6.9.4.
	Columns []*ColumnDef
	// Shadowed lists column definitions that lost a §6.9.4 conflict.
	Shadowed []*ColumnDef

	Indexes []*ast.Index // all indexes, direct and injected
}

// ColumnDef is one column definition and where it came from.
type ColumnDef struct {
	Col     *ast.Column
	Partial *ast.TablePartial // nil for direct definitions
}

// Column returns the effective definition of the named column, or nil.
func (t *TableInfo) Column(name string) *ColumnDef {
	for _, c := range t.Columns {
		if c.Col.Name.Name() == name {
			return c
		}
	}
	return nil
}

type EnumInfo struct {
	Decl *ast.Enum
	Key  string
	Uses int // column type references within this file
}

type PartialInfo struct {
	Decl *ast.TablePartial
	Uses int
}

// RefInfo is one resolved (or unresolved) relationship, from any of the
// three syntactic forms.
type RefInfo struct {
	Node   ast.Node // *ast.Ref, or the *ast.Setting of an inline ref
	Op     token.Kind
	Inline bool
	Left   EndpointInfo
	Right  EndpointInfo
}

type EndpointInfo struct {
	Table   *TableInfo // nil if unresolved
	Columns []string
	Pos     token.Position
}

// canonKey normalizes a table name with the default schema (spec §8.1).
func canonKey(q *ast.QualName) string {
	if q.Schema() == "" {
		return "public." + q.Base()
	}
	return q.String()
}

// File checks one parsed file and returns its semantic model together with
// all diagnostics. When the file imports other files (§7), unresolved-name
// errors are suppressed, because the definitions may live elsewhere; all
// local constraints still apply.
func File(f *ast.File) (*Info, []diag.Diagnostic) {
	c := &checker{
		info: &Info{
			HasImports: f.HasImports(),
			byTable:    map[string]*TableInfo{},
		},
	}
	c.collect(f)
	c.checkDecls(f)
	c.resolve(f)
	diag.Sort(c.diags)
	return c.info, c.diags
}

type checker struct {
	info  *Info
	diags []diag.Diagnostic

	partials map[string]*PartialInfo
	enums    map[string]*EnumInfo
	groups   map[string]*ast.TableGroup
	sticky   map[string]*ast.StickyNote
	views    map[string]*ast.DiagramView
	schemas  map[string]bool
	project  *ast.Project
}

func (c *checker) errorf(pos token.Position, section, format string, args ...any) {
	c.diags = append(c.diags, diag.Errorf(pos, "spec/"+section, format, args...))
}

/* ===== pass 1: collect declarations, detect duplicate names (§8.2) ===== */

func (c *checker) collect(f *ast.File) {
	c.partials = map[string]*PartialInfo{}
	c.enums = map[string]*EnumInfo{}
	c.groups = map[string]*ast.TableGroup{}
	c.sticky = map[string]*ast.StickyNote{}
	c.views = map[string]*ast.DiagramView{}
	c.schemas = map[string]bool{"public": true}

	// Partials first: tables need them for injection regardless of order (§5.2).
	for _, d := range f.Decls {
		if tp, ok := d.(*ast.TablePartial); ok {
			if _, dup := c.partials[tp.Name.Name()]; dup {
				c.errorf(tp.Pos(), "8.2", "duplicate TablePartial name %q", tp.Name.Name())
				continue
			}
			c.partials[tp.Name.Name()] = &PartialInfo{Decl: tp}
		}
	}

	for _, d := range f.Decls {
		switch d := d.(type) {
		case *ast.Table:
			key := canonKey(d.Name)
			if _, dup := c.info.byTable[key]; dup {
				c.errorf(d.Pos(), "8.2", "duplicate table name %q", d.Name.String())
				continue
			}
			ti := &TableInfo{Decl: d, Key: key}
			if d.Alias != nil {
				ti.Alias = d.Alias.Name()
				aliasKey := "public." + ti.Alias
				if _, dup := c.info.byTable[aliasKey]; dup {
					c.errorf(d.Alias.Pos(), "8.2", "table alias %q collides with an existing table name or alias", ti.Alias)
				} else {
					c.info.byTable[aliasKey] = ti
				}
			}
			c.info.byTable[key] = ti
			c.info.Tables = append(c.info.Tables, ti)
			if s := d.Name.Schema(); s != "" {
				c.schemas[s] = true
			}
		case *ast.Enum:
			key := canonKey(d.Name)
			if _, dup := c.enums[key]; dup {
				c.errorf(d.Pos(), "8.2", "duplicate enum name %q", d.Name.String())
				continue
			}
			ei := &EnumInfo{Decl: d, Key: key}
			c.enums[key] = ei
			c.info.Enums = append(c.info.Enums, ei)
			if s := d.Name.Schema(); s != "" {
				c.schemas[s] = true
			}
		case *ast.TableGroup:
			if _, dup := c.groups[d.Name.Name()]; dup {
				c.errorf(d.Pos(), "8.2", "duplicate TableGroup name %q", d.Name.Name())
				continue
			}
			c.groups[d.Name.Name()] = d
		case *ast.StickyNote:
			if _, dup := c.sticky[d.Name.Name()]; dup {
				c.errorf(d.Pos(), "8.2", "duplicate sticky note name %q", d.Name.Name())
				continue
			}
			c.sticky[d.Name.Name()] = d
		case *ast.DiagramView:
			if _, dup := c.views[d.Name.Name()]; dup {
				c.errorf(d.Pos(), "8.2", "duplicate DiagramView name %q", d.Name.Name())
				continue
			}
			c.views[d.Name.Name()] = d
		case *ast.Project:
			if c.project != nil {
				c.errorf(d.Pos(), "6.1", "at most one Project element per file")
				continue
			}
			c.project = d
		}
	}
	for _, pi := range c.partials {
		c.info.Partials = append(c.info.Partials, pi)
	}

	// Effective columns: expand injections in source order (§8.4).
	for _, ti := range c.info.Tables {
		c.expandColumns(ti)
	}
}

// expandColumns applies §6.9.4 conflict resolution: direct definitions win;
// otherwise the last-injected partial wins.
func (c *checker) expandColumns(ti *TableInfo) {
	type slot struct {
		def   *ColumnDef
		order int
	}
	byName := map[string]*slot{}
	direct := map[string]bool{}
	order := 0
	addCol := func(col *ast.Column, from *ast.TablePartial) {
		name := col.Name.Name()
		def := &ColumnDef{Col: col, Partial: from}
		if s, ok := byName[name]; ok {
			if from == nil && direct[name] {
				// duplicate direct definition — an error, reported in checkDecls
				return
			}
			if from == nil || !direct[name] {
				// direct wins; among partials, the later injection wins
				ti.Shadowed = append(ti.Shadowed, s.def)
				s.def = def
				direct[name] = from == nil
			} else {
				ti.Shadowed = append(ti.Shadowed, def)
			}
			return
		}
		byName[name] = &slot{def: def, order: order}
		direct[name] = from == nil
		order++
	}
	for _, item := range ti.Decl.Body {
		switch item := item.(type) {
		case *ast.Column:
			addCol(item, nil)
		case *ast.PartialRef:
			pi, ok := c.partials[item.Name.Name()]
			if !ok {
				if !c.info.HasImports {
					c.errorf(item.Pos(), "6.9", "unknown TablePartial %q", item.Name.Name())
				}
				continue
			}
			pi.Uses++
			for _, pit := range pi.Decl.Body {
				switch pit := pit.(type) {
				case *ast.Column:
					addCol(pit, pi.Decl)
				case *ast.IndexesBlock:
					ti.Indexes = append(ti.Indexes, pit.Indexes...)
				}
			}
		case *ast.IndexesBlock:
			ti.Indexes = append(ti.Indexes, item.Indexes...)
		}
	}
	slots := make([]*slot, 0, len(byName))
	for _, s := range byName {
		slots = append(slots, s)
	}
	for i := 0; i < len(slots); i++ {
		for j := i + 1; j < len(slots); j++ {
			if slots[j].order < slots[i].order {
				slots[i], slots[j] = slots[j], slots[i]
			}
		}
	}
	for _, s := range slots {
		ti.Columns = append(ti.Columns, s.def)
	}
}
