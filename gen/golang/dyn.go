// Dynamic-query generation (nao_dyn.go): the v2 surface of Not an ORM
// (decisions D28-D34).
//
// Per table:
//
//	UserEmail, UserID, ...        the typed column handles (D29): one
//	                              rt.Column/rt.NullColumn var per
//	                              column, flat Model+Field names (enum
//	                              types carry an E prefix so the
//	                              idiomatic enum-per-column pattern
//	                              cannot collide with a handle)
//	UserLimit/UserOffset/...      per-model wrappers for value-less
//	                              options (D30): Go cannot infer the
//	                              phantom model type without a value
//	UserQuery(ctx, opts...)       SELECT filtered/ordered/limited by
//	                              inert option values (D28)
//	UserCount(ctx, preds...)      count over the same predicates (D32)
//	UserExists(ctx, preds...)     EXISTS over the same predicates
//	UserDeleteWhere(ctx, preds...)   predicate-guarded DELETE
//	UserUpdateWhere(ctx, set, preds...)  partial UPDATE from typed
//	                              assignments; predicates shared (D32)
//
// The interpreter lives in rt (query.go); this file only mints the typed
// handles and the terminal methods. Dynamic SQL binds positional '?'
// placeholders rendered by the interpreter (D42); the statement cache
// (D31) is wired through Queries.WithCache.
//
// Package-scope names are minted by concatenation (UserEmail, UserLimit),
// so distinct DBML declarations can demand one Go name. Generation fails
// loudly on any collision; the vet dynname rule reports the same
// collisions with both origins named.
package golang

import (
	"fmt"
	"go/format"
	"sort"
	"strings"

	"github.com/Piechutowski/not-an-orm/edbml/ast"
	"github.com/Piechutowski/not-an-orm/edbml/check"
	"github.com/Piechutowski/not-an-orm/edbml/token"
)

// GenerateDyn renders the dynamic-query file for one checked DBML file:
// the third sibling of Generate and GenerateQueries — same package,
// models and the Queries type assumed present.
func GenerateDyn(f *ast.File, info *check.Info, opts Options) ([]byte, error) {
	if opts.Package == "" {
		return nil, fmt.Errorf("no package name")
	}
	p, err := planBuild(f, info)
	if err != nil {
		return nil, err
	}
	if cs := dynNamesCheck(p, info); len(cs) > 0 {
		c := cs[0]
		return nil, fmt.Errorf("%s and %s both need the Go name %s; rename one (e.g. with [model:])", c.First, c.Second, c.Name)
	}
	e := &dynEmitter{plan: p, opts: opts}
	e.run()
	src, err := format.Source([]byte(e.out.String()))
	if err != nil {
		// unreachable if the emitter is correct; surfaced loudly if not
		return nil, fmt.Errorf("generated code does not parse: %w\n%s", err, e.out.String())
	}
	return src, nil
}

/* ===== package-scope name collisions (DYN-7) ===== */

// dynWrapperSuffixes are the per-model option-wrapper names the emitter
// mints (D30), shared with the collision check so the two cannot drift.
var dynWrapperSuffixes = []string{"Limit", "Offset", "Distinct", "OrderBy", "After", "Set"}

// NameCollision is one Go package-scope name that generation would mint
// twice. First and Second describe the colliding origins in declaration
// order; Pos locates the later of the two.
type NameCollision struct {
	Name          string
	First, Second string
	Pos           token.Position
}

// DynNameCollisions reports the package-scope Go name collisions the
// generated files would produce — model structs, enum types and
// constants, params structs, column handles (UserEmail) and option
// wrappers (UserLimit, ...). It is the analysis behind the vet dynname
// rule; GenerateDyn refuses to generate while any collision remains. A
// file that does not survive planning reports nothing: generation
// itself raises those errors.
func DynNameCollisions(f *ast.File, info *check.Info) []NameCollision {
	p, err := planBuild(f, info)
	if err != nil {
		return nil
	}
	return dynNamesCheck(p, info)
}

func dynNamesCheck(p *plan, info *check.Info) []NameCollision {
	type origin struct {
		desc string
		pos  token.Position
	}
	seen := map[string]origin{"Queries": {desc: "the generated Queries type"}}
	var out []NameCollision
	add := func(name, desc string, pos token.Position) {
		prev, dup := seen[name]
		if !dup {
			seen[name] = origin{desc: desc, pos: pos}
			return
		}
		first, second := prev, origin{desc: desc, pos: pos}
		if second.pos.Line < first.pos.Line || (second.pos.Line == first.pos.Line && second.pos.Column < first.pos.Column) {
			first, second = second, first
		}
		out = append(out, NameCollision{Name: name, First: first.desc, Second: second.desc, Pos: second.pos})
	}

	for _, e := range info.Enums {
		typeName, err := enumTypeName(e.Decl.Name.Schema(), e.Decl.Name.Base())
		if err != nil {
			continue // generation reports unusable names itself
		}
		add(typeName, "enum "+e.Decl.Name.String(), e.Decl.Pos())
		for _, v := range e.Decl.Values {
			constName, err := goName(v.Name.Name())
			if err != nil {
				continue
			}
			add(typeName+constName, fmt.Sprintf("enum %s value %q", e.Decl.Name.String(), v.Name.Name()), v.Pos())
		}
	}
	for _, tm := range p.tables {
		tbl := tm.ti.Decl.Name.String()
		pos := tm.ti.Decl.Pos()
		add(tm.model, fmt.Sprintf("table %s (model %s)", tbl, tm.model), pos)
		if len(tm.fields) == 0 {
			continue // no queryable shape: no queries, no dynamic layer
		}
		if len(tm.createFields()) > 0 {
			add(tm.model+"CreateParams", fmt.Sprintf("table %s (create params)", tbl), pos)
		}
		if len(tm.pk) > 0 && len(tm.nonPK()) > 0 {
			add(tm.model+"UpdateParams", fmt.Sprintf("table %s (update params)", tbl), pos)
		}
		for _, f := range tm.fields {
			add(tm.model+f.goField, fmt.Sprintf("table %s column %q (handle %s%s)", tbl, f.colName, tm.model, f.goField), f.col.Pos())
		}
		for _, sfx := range dynWrapperSuffixes {
			add(tm.model+sfx, fmt.Sprintf("table %s (option wrapper %s%s)", tbl, tm.model, sfx), pos)
		}
	}
	return out
}

/* ===== the emitter ===== */

type dynEmitter struct {
	plan *plan
	opts Options
	out  strings.Builder
	body strings.Builder
}

func (e *dynEmitter) run() {
	for _, t := range e.plan.tables {
		e.tableEmit(t)
	}
	e.header()
	e.out.WriteString(e.body.String())
}

func (e *dynEmitter) header() {
	fmt.Fprintf(&e.out, "// Code generated by nao gen from %s; DO NOT EDIT.\n", e.opts.Source)
	e.out.WriteString("//\n// The dynamic query layer over the models in nao_models.go: typed\n")
	e.out.WriteString("// column handles, per-model option wrappers, and predicate-driven\n")
	e.out.WriteString("// terminals — see docs/decisions.md D28-D34.\n\n")
	fmt.Fprintf(&e.out, "package %s\n", e.opts.Package)

	if e.body.Len() == 0 {
		return // nothing emitted (no queryable tables): no imports either
	}
	imports := map[string]bool{"context": true, rtImport: true}
	for imp := range e.plan.imports {
		imports[imp] = true
	}
	paths := make([]string, 0, len(imports))
	for p := range imports {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	e.out.WriteString("\nimport (\n")
	for _, p := range paths {
		fmt.Fprintf(&e.out, "\t%q\n", p)
	}
	e.out.WriteString(")\n\n")
}

func (e *dynEmitter) tableEmit(t *tableModel) {
	if len(t.fields) == 0 {
		return // a columnless table has no queryable shape
	}
	b := &e.body
	lower := argName(t.model)
	tbl := t.ti.Decl.Name.String()

	// the pre-rendered shape the interpreter fills in around
	fmt.Fprintf(b, "// %sTableSQL and %sColumnsSQL are the fixed shape of %s's\n", lower, lower, t.model)
	fmt.Fprintf(b, "// dynamic statements; the interpreter renders everything else (D28).\nconst (\n")
	fmt.Fprintf(b, "\t%sTableSQL   = `%s`\n", lower, sqlIdentQuote(t.sqlName))
	fmt.Fprintf(b, "\t%sColumnsSQL = `%s`\n)\n\n", lower, t.columnList())

	e.handlesEmit(t, lower, tbl)
	e.wrappersEmit(t, tbl)
	e.queryEmit(t, lower, tbl)
	e.countEmit(t, lower, tbl)
	e.existsEmit(t, lower, tbl)
	e.deleteWhereEmit(t, lower, tbl)
	e.updateWhereEmit(t, lower, tbl)
}

func (e *dynEmitter) handlesEmit(t *tableModel, lower, tbl string) {
	b := &e.body
	fmt.Fprintf(b, "// Typed column handles of %s (D29): inert predicate, order and\n", t.model)
	fmt.Fprintf(b, "// assignment builders for the dynamic query layer (D28). A predicate\n")
	fmt.Fprintf(b, "// built here can only enter %s queries; mixing models is a compile\n", t.model)
	fmt.Fprintf(b, "// error.\n")
	b.WriteString("var (\n")
	for _, f := range t.fields {
		if f.nullable {
			fmt.Fprintf(b, "\t%s%s = rt.NullColumn[%s, %s]{Column: rt.Column[%s, %s]{Name: %q}}\n",
				t.model, f.goField, t.model, f.baseType, t.model, f.baseType, f.colName)
			continue
		}
		fmt.Fprintf(b, "\t%s%s = rt.Column[%s, %s]{Name: %q}\n", t.model, f.goField, t.model, f.baseType, f.colName)
	}
	b.WriteString(")\n\n")
}

func (e *dynEmitter) wrappersEmit(t *tableModel, tbl string) {
	b := &e.body
	m := t.model
	fmt.Fprintf(b, "// %sLimit caps how many rows %sQuery returns (D30).\n", m, m)
	fmt.Fprintf(b, "func %sLimit(n int) rt.Opt[%s] { return rt.Limit[%s](n) }\n\n", m, m, m)
	fmt.Fprintf(b, "// %sOffset skips n rows; keyset pagination (%sAfter) scales better (D34).\n", m, m)
	fmt.Fprintf(b, "func %sOffset(n int) rt.Opt[%s] { return rt.Offset[%s](n) }\n\n", m, m, m)
	fmt.Fprintf(b, "// %sDistinct deduplicates the rows %sQuery returns.\n", m, m)
	fmt.Fprintf(b, "func %sDistinct() rt.Opt[%s] { return rt.Distinct[%s]() }\n\n", m, m, m)
	fmt.Fprintf(b, "// %sOrderBy sorts %sQuery's rows by Asc/Desc terms built on the\n// %s column handles.\n", m, m, m)
	fmt.Fprintf(b, "func %sOrderBy(terms ...rt.Order[%s]) rt.Opt[%s] { return rt.OrderBy(terms...) }\n\n", m, m, m)
	fmt.Fprintf(b, "// %sAfter resumes strictly after the row with the given key — keyset\n", m)
	fmt.Fprintf(b, "// pagination (D34): one value per %sOrderBy term, in the same order.\n", m)
	fmt.Fprintf(b, "func %sAfter(key ...any) rt.Opt[%s] { return rt.After[%s](key...) }\n\n", m, m, m)
	fmt.Fprintf(b, "// %sSet collects the typed assignments of a %sUpdateWhere, built\n", m, m)
	fmt.Fprintf(b, "// with Set/SetNull on the %s column handles.\n", m)
	fmt.Fprintf(b, "func %sSet(assigns ...rt.Assign[%s]) []rt.Assign[%s] { return assigns }\n\n", m, m, m)
}

func (e *dynEmitter) queryEmit(t *tableModel, lower, tbl string) {
	b := &e.body
	fmt.Fprintf(b, "// %sQuery returns the %s rows matching every given predicate,\n", t.model, tbl)
	fmt.Fprintf(b, "// shaped by the remaining options (order, limit, offset, keyset,\n")
	fmt.Fprintf(b, "// distinct). No options means every row.\n")
	fmt.Fprintf(b, "func (q *Queries) %sQuery(ctx context.Context, opts ...rt.Opt[%s]) ([]%s, error) {\n", t.model, t.model, t.model)
	fmt.Fprintf(b, "\tquery, args, err := rt.SelectRender(%sTableSQL, %sColumnsSQL, opts)\n", lower, lower)
	b.WriteString("\tif err != nil {\n\t\treturn nil, err\n\t}\n")
	fmt.Fprintf(b, "\trows, err := rt.StmtQuery(ctx, q.db, q.cache, query, args...)\n")
	rowsScanEmit(b, t.model, lower)
}

func (e *dynEmitter) countEmit(t *tableModel, lower, tbl string) {
	b := &e.body
	fmt.Fprintf(b, "// %sCount reports how many %s rows match every given predicate;\n", t.model, tbl)
	fmt.Fprintf(b, "// none counts the whole table.\n")
	fmt.Fprintf(b, "func (q *Queries) %sCount(ctx context.Context, preds ...rt.Pred[%s]) (int64, error) {\n", t.model, t.model)
	fmt.Fprintf(b, "\tquery, args, err := rt.CountRender(%sTableSQL, preds)\n", lower)
	b.WriteString("\tif err != nil {\n\t\treturn 0, err\n\t}\n")
	b.WriteString("\tvar n int64\n\tif err := rt.RowScan(ctx, q.db, q.cache, query, args, &n); err != nil {\n\t\treturn 0, err\n\t}\n\treturn n, nil\n}\n\n")
}

func (e *dynEmitter) existsEmit(t *tableModel, lower, tbl string) {
	b := &e.body
	fmt.Fprintf(b, "// %sExists reports whether any %s row matches every given predicate.\n", t.model, tbl)
	fmt.Fprintf(b, "func (q *Queries) %sExists(ctx context.Context, preds ...rt.Pred[%s]) (bool, error) {\n", t.model, t.model)
	fmt.Fprintf(b, "\tquery, args, err := rt.ExistsRender(%sTableSQL, preds)\n", lower)
	b.WriteString("\tif err != nil {\n\t\treturn false, err\n\t}\n")
	b.WriteString("\tvar found bool\n\tif err := rt.RowScan(ctx, q.db, q.cache, query, args, &found); err != nil {\n\t\treturn false, err\n\t}\n\treturn found, nil\n}\n\n")
}

func (e *dynEmitter) deleteWhereEmit(t *tableModel, lower, tbl string) {
	b := &e.body
	fmt.Fprintf(b, "// %sDeleteWhere removes every %s row matching the predicates and\n", t.model, tbl)
	fmt.Fprintf(b, "// reports how many went. No effective predicate is an error, never a\n")
	fmt.Fprintf(b, "// full-table delete (D42).\n")
	fmt.Fprintf(b, "func (q *Queries) %sDeleteWhere(ctx context.Context, preds ...rt.Pred[%s]) (int64, error) {\n", t.model, t.model)
	fmt.Fprintf(b, "\tquery, args, err := rt.DeleteRender(%sTableSQL, preds)\n", lower)
	b.WriteString("\tif err != nil {\n\t\treturn 0, err\n\t}\n")
	b.WriteString("\tres, err := rt.StmtExec(ctx, q.db, q.cache, query, args...)\n")
	b.WriteString("\tif err != nil {\n\t\treturn 0, err\n\t}\n\treturn res.RowsAffected()\n}\n\n")
}

func (e *dynEmitter) updateWhereEmit(t *tableModel, lower, tbl string) {
	b := &e.body
	fmt.Fprintf(b, "// %sUpdateWhere applies the typed assignments (%sSet) to every %s\n", t.model, t.model, tbl)
	fmt.Fprintf(b, "// row matching the predicates and reports how many changed — the\n")
	fmt.Fprintf(b, "// partial update of the dynamic layer (D32). No effective predicate\n")
	fmt.Fprintf(b, "// is an error, never a full-table rewrite (D42).\n")
	fmt.Fprintf(b, "func (q *Queries) %sUpdateWhere(ctx context.Context, set []rt.Assign[%s], preds ...rt.Pred[%s]) (int64, error) {\n", t.model, t.model, t.model)
	fmt.Fprintf(b, "\tquery, args, err := rt.UpdateRender(%sTableSQL, set, preds)\n", lower)
	b.WriteString("\tif err != nil {\n\t\treturn 0, err\n\t}\n")
	b.WriteString("\tres, err := rt.StmtExec(ctx, q.db, q.cache, query, args...)\n")
	b.WriteString("\tif err != nil {\n\t\treturn 0, err\n\t}\n\treturn res.RowsAffected()\n}\n\n")
}
