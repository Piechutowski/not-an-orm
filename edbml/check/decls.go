// Pass 2: local (single-element) constraints — settings whitelists, value
// types, body shape. One method per element kind, matching the spec layout.
package check

import (
	"strings"

	"github.com/Piechutowski/not-an-orm/edbml/ast"
	"github.com/Piechutowski/not-an-orm/edbml/parser"
	"github.com/Piechutowski/not-an-orm/edbml/token"
)

func (c *checker) declsCheck(f *ast.File) {
	for _, d := range f.Decls {
		switch d := d.(type) {
		case *ast.Use:
			c.useCheck(d)
		case *ast.Project:
			// properties are free-form (§6.1.2); nothing local to check
		case *ast.Table:
			c.tableBodyCheck(d.Name.String(), d.Settings, d.Body, true)
		case *ast.TablePartial:
			c.tableBodyCheck(d.Name.Name(), d.Settings, d.Body, false)
		case *ast.Enum:
			c.enumCheck(d)
		case *ast.Ref:
			c.refSettingsCheck(d.Settings)
			c.endpointArityCheck(d.Left, d.Right)
		case *ast.StickyNote:
			c.settingsCheck(d.Settings, "6.11", stickySettings)
		case *ast.TableGroup:
			c.settingsCheck(d.Settings, "6.12", groupSettings)
		case *ast.DiagramView:
			c.viewCheck(d)
		case *ast.Records:
			c.recordsCheck(d, true)
		}
	}
}

var importKinds = map[string]bool{
	"table": true, "enum": true, "tablepartial": true,
	"note": true, "schema": true, "tablegroup": true,
}

func (c *checker) useCheck(d *ast.Use) {
	for _, it := range d.Items {
		if !importKinds[strings.ToLower(it.Kind.Name())] || it.Kind.Quoted() {
			c.errorf(it.Kind.Pos(), "7", "unknown import kind %q; expected table, enum, tablepartial, note, schema or tablegroup", it.Kind.Name())
		}
	}
}

/* ===== table bodies (§6.2, §6.9) ===== */

func (c *checker) tableBodyCheck(name string, settings *ast.SettingList, body []ast.TableItem, isTable bool) {
	c.settingsCheck(settings, "6.2", tableSettings)

	columns := map[string]bool{}
	nCols, nInject, nNotes, nRecords := 0, 0, 0, 0
	for _, item := range body {
		switch item := item.(type) {
		case *ast.Column:
			c.columnCheck(item)
			if columns[item.Name.Name()] {
				c.errorf(item.Pos(), "8.2", "duplicate column name %q", item.Name.Name())
			}
			columns[item.Name.Name()] = true
			nCols++
		case *ast.PartialRef:
			if !isTable {
				c.errorf(item.Pos(), "6.9", "partial injection is not allowed inside TablePartial")
			}
			nInject++
		case *ast.IndexesBlock:
			for _, ix := range item.Indexes {
				c.settingsCheck(ix.Settings, "6.5", indexSettings)
			}
		case *ast.ChecksBlock:
			for _, ck := range item.Checks {
				c.settingsCheck(ck.Settings, "6.6", settingsCheck)
			}
		case *ast.Note:
			nNotes++
		case *ast.Records:
			if !isTable {
				c.errorf(item.Pos(), "6.9", "records are not allowed inside TablePartial")
			}
			nRecords++
			c.recordsCheck(item, false)
		}
	}
	if isTable && nCols == 0 && nInject == 0 {
		c.errorf(bodyPos(body, settings), "6.2", "table %q must contain at least one column or partial injection", name)
	}
	if nNotes > 1 {
		c.errorf(bodyPos(body, settings), "6.11", "at most one Note per %s body", kindWord(isTable))
	}
	if nRecords > 1 {
		c.errorf(bodyPos(body, settings), "6.10", "at most one records block per table")
	}
}

func kindWord(isTable bool) string {
	if isTable {
		return "Table"
	}
	return "TablePartial"
}

func bodyPos(body []ast.TableItem, settings *ast.SettingList) token.Position {
	if len(body) > 0 {
		return body[0].Pos()
	}
	if settings != nil {
		return settings.Pos()
	}
	return token.Position{}
}

func (c *checker) columnCheck(col *ast.Column) {
	for _, f := range col.LegacyFlags {
		low := strings.ToLower(f.Name())
		if f.Quoted() || (low != "pk" && low != "unique") {
			c.errorf(f.Pos(), "6.3", "only the legacy flags pk and unique may follow the column type; found %q", f.Name())
		}
	}
	c.settingsCheck(col.Settings, "6.3", columnSettings)
}

/* ===== Enum (§6.8) ===== */

func (c *checker) enumCheck(d *ast.Enum) {
	if len(d.Values) == 0 {
		c.errorf(d.Pos(), "6.8", "enum %q must contain at least one value", d.Name.String())
	}
	seen := map[string]bool{}
	for _, v := range d.Values {
		if seen[v.Name.Name()] {
			c.errorf(v.Pos(), "8.2", "duplicate enum value %q", v.Name.Name())
		}
		seen[v.Name.Name()] = true
		c.settingsCheck(v.Settings, "6.8", enumValueSettings)
	}
}

/* ===== Ref (§6.7) ===== */

func (c *checker) endpointArityCheck(l, r *ast.RefEndpoint) {
	if l == nil || r == nil {
		return
	}
	if len(l.Columns) != len(r.Columns) {
		c.errorf(r.Pos(), "6.7", "composite relationship endpoints must have the same number of columns: %d vs %d", len(l.Columns), len(r.Columns))
	}
}

var refActions = map[string]bool{
	"cascade": true, "restrict": true, "set null": true,
	"set default": true, "no action": true,
}

func (c *checker) refSettingsCheck(sl *ast.SettingList) {
	c.settingsCheck(sl, "6.7", refSettings)
	if sl == nil {
		return
	}
	for _, s := range sl.Settings {
		if s.Name != "delete" && s.Name != "update" {
			continue
		}
		action := ""
		switch v := s.Value.(type) {
		case *ast.Ident:
			action = strings.ToLower(v.Name())
		case parser.MultiWord:
			action = strings.Join(v.Words(), " ")
		}
		if !refActions[action] {
			c.errorf(s.Pos(), "6.7", "invalid referential action %q for '%s'; expected cascade, restrict, set null, set default or no action", action, s.Name)
		}
	}
}

/* ===== Records (§6.10) ===== */

func (c *checker) recordsCheck(d *ast.Records, topLevel bool) {
	if topLevel && !d.HasColumns {
		c.errorf(d.Pos(), "6.10", "top-level records must list columns")
	}
	for _, row := range d.Rows {
		if d.HasColumns && len(row.Values) != len(d.Columns) {
			c.errorf(row.Pos(), "6.10", "record row has %d values but %d columns are listed", len(row.Values), len(d.Columns))
		}
		for _, v := range row.Values {
			if id, ok := v.(*ast.Ident); ok {
				low := strings.ToLower(id.Name())
				if id.Quoted() || (low != "true" && low != "false" && low != "null") {
					c.errorf(id.Pos(), "6.10", "record values must be literals, null, true, false or enum constants; found bare identifier %q", id.Name())
				}
			}
		}
	}
}

/* ===== DiagramView (§6.13) ===== */

var categoryKinds = map[string]bool{
	"tables": true, "notes": true, "tablegroups": true, "schemas": true,
}

func (c *checker) viewCheck(d *ast.DiagramView) {
	seen := map[string]bool{}
	for _, cat := range d.Categories {
		low := strings.ToLower(cat.Kind.Name())
		if !categoryKinds[low] || cat.Kind.Quoted() {
			c.errorf(cat.Kind.Pos(), "6.13", "unknown view category %q; expected Tables, Notes, TableGroups or Schemas", cat.Kind.Name())
			continue
		}
		if seen[low] {
			c.errorf(cat.Kind.Pos(), "6.13", "category %q may appear at most once per view", cat.Kind.Name())
		}
		seen[low] = true
	}
}
