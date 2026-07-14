// CRUD generation (nao_queries.go): the v0 query surface of Not an ORM.
//
// Per table (decisions D9, D15, D16, D17):
//
//	UserGet(ctx, id)              by primary key; composite keys take one
//	                              positional argument per key column
//	UserGetMany(ctx, ids)         single-column keys only; one IN query
//	UserGetByEmail(ctx, email)    per unique column
//	UserList(ctx)                 every row
//	UserCount(ctx)                SELECT count(*)
//	UserCreate(ctx, params)       INSERT ... RETURNING; params exclude the
//	                              auto-increment key and defaulted columns
//	UserUpdate(ctx, id, params)   rewrites every non-key column, RETURNING
//	UserDelete(ctx, id)           rt.ErrNotFound when nothing matched
//
// Tables without a primary key get only List/Create/Count (D17). SQL uses
// named placeholders (:email) bound with sql.Named — the placeholder is
// the column name, so argument order cannot exist as a concept (D15).
// Identity arguments are positional, data arguments ride in a generated
// params struct (D15). Table and column naming matches gen/sqlite's DDL
// (schemas flattened: core.users -> core_users), so every generated
// statement prepares against the generated schema.
package golang

import (
	"fmt"
	"go/format"
	"sort"
	"strings"

	"github.com/Piechutowski/not-an-orm/edbml/ast"
	"github.com/Piechutowski/not-an-orm/edbml/check"
)

// GenerateQueries renders the queries file for one checked DBML file. It
// is the sibling of Generate: same package, second file, models assumed
// present.
func GenerateQueries(f *ast.File, info *check.Info, opts Options) ([]byte, error) {
	if opts.Package == "" {
		return nil, fmt.Errorf("no package name")
	}
	p, err := buildPlan(f, info)
	if err != nil {
		return nil, err
	}
	e := &queryEmitter{plan: p, opts: opts}
	e.run()
	src, err := format.Source([]byte(e.out.String()))
	if err != nil {
		// unreachable if the emitter is correct; surfaced loudly if not
		return nil, fmt.Errorf("generated code does not parse: %w\n%s", err, e.out.String())
	}
	return src, nil
}

/* ===== the plan: everything decided before a line is emitted ===== */

type plan struct {
	tables  []*tableModel
	imports map[string]bool
}

// tableModel is one table resolved into Go and SQL naming.
type tableModel struct {
	ti      *check.TableInfo
	model   string       // Go model type, e.g. "User"
	sqlName string       // flattened SQLite table name, e.g. "core_users"
	fields  []*fieldPlan // effective columns in definition order
	pk      []*fieldPlan // identity columns in key order; empty = no pk
}

// fieldPlan is one column resolved into Go and SQL naming.
type fieldPlan struct {
	col     *ast.Column
	colName string // DBML/SQL column name
	goField string // exported struct field, e.g. "EditorID"
	goType  string // full field type, e.g. "rt.Null[int64]"
	param   string // SQL parameter name, e.g. "editor_id"
	arg     string // positional Go argument name, e.g. "editorID"

	increment  bool
	hasDefault bool
	unique     bool // column-level [unique] or a single-column unique index
}

func (t *tableModel) field(colName string) *fieldPlan {
	for _, f := range t.fields {
		if f.colName == colName {
			return f
		}
	}
	return nil
}

// nonPK returns the data columns: everything outside the primary key.
func (t *tableModel) nonPK() []*fieldPlan {
	var out []*fieldPlan
	for _, f := range t.fields {
		if !t.isPK(f) {
			out = append(out, f)
		}
	}
	return out
}

func (t *tableModel) isPK(f *fieldPlan) bool {
	for _, p := range t.pk {
		if p == f {
			return true
		}
	}
	return false
}

// createFields returns the caller-supplied Create columns (D16): the
// auto-increment key and defaulted columns are the database's job.
func (t *tableModel) createFields() []*fieldPlan {
	var out []*fieldPlan
	for _, f := range t.fields {
		if f.increment || f.hasDefault {
			continue
		}
		out = append(out, f)
	}
	return out
}

func buildPlan(f *ast.File, info *check.Info) (*plan, error) {
	g := &generator{f: f, info: info, imports: map[string]bool{}}
	if err := g.collectEnumTypes(); err != nil {
		return nil, err
	}

	p := &plan{imports: g.imports}
	typeNames := map[string]string{"Queries": "the generated Queries type"}
	for _, e := range info.Enums {
		typeNames[g.enumTypes[e.Key]] = "enum " + e.Decl.Name.String()
	}
	sqlNames := map[string]string{}

	for _, ti := range info.Tables {
		tm, err := buildTable(g, ti, typeNames, sqlNames)
		if err != nil {
			return nil, err
		}
		p.tables = append(p.tables, tm)
	}
	return p, nil
}

func buildTable(g *generator, ti *check.TableInfo, typeNames, sqlNames map[string]string) (*tableModel, error) {
	model, err := modelName(ti.Decl)
	if err != nil {
		return nil, fmt.Errorf("table %s: %w", ti.Decl.Name.String(), err)
	}
	if prev, dup := typeNames[model]; dup {
		return nil, fmt.Errorf("table %s and %s both map to Go type %s", ti.Decl.Name.String(), prev, model)
	}
	typeNames[model] = "table " + ti.Decl.Name.String()

	sqlName := sqlTableName(ti.Decl.Name)
	if prev, dup := sqlNames[sqlName]; dup {
		return nil, fmt.Errorf("tables %s and %s both flatten to SQLite name %q", prev, ti.Decl.Name.String(), sqlName)
	}
	sqlNames[sqlName] = ti.Decl.Name.String()

	tm := &tableModel{ti: ti, model: model, sqlName: sqlName}
	pkFromIndex := compositePKColumns(ti)
	goFields := map[string]string{}
	params := map[string]string{}
	for _, cd := range ti.Columns {
		fp, err := buildField(g, cd, pkFromIndex, goFields, params)
		if err != nil {
			return nil, fmt.Errorf("table %s: %w", ti.Decl.Name.String(), err)
		}
		tm.fields = append(tm.fields, fp)
	}

	// identity: column-level pk settings in definition order, then pk-index
	// columns in index order
	for _, f := range tm.fields {
		if hasSetting(f.col, "pk") || hasSetting(f.col, "primary key") || hasLegacyFlag(f.col, "pk") {
			tm.pk = append(tm.pk, f)
		}
	}
	if len(tm.pk) == 0 {
		for _, ix := range ti.Indexes {
			if ix.Settings.Get("pk") == nil {
				continue
			}
			for _, atom := range ix.Key {
				if id, ok := atom.(*ast.Ident); ok {
					if f := tm.field(id.Name()); f != nil {
						tm.pk = append(tm.pk, f)
					}
				}
			}
		}
	}

	// single-column unique indexes mark their column unique, like [unique]
	for _, ix := range ti.Indexes {
		if ix.Settings.Get("unique") == nil || len(ix.Key) != 1 {
			continue
		}
		if id, ok := ix.Key[0].(*ast.Ident); ok {
			if f := tm.field(id.Name()); f != nil {
				f.unique = true
			}
		}
	}
	return tm, nil
}

func buildField(g *generator, cd *check.ColumnDef, pkFromIndex map[string]bool, goFields, params map[string]string) (*fieldPlan, error) {
	col := cd.Col
	colName := col.Name.Name()

	goField, err := goName(colName)
	if err != nil {
		return nil, fmt.Errorf("column %q: %w", colName, err)
	}
	if prev, dup := goFields[goField]; dup {
		return nil, fmt.Errorf("columns %q and %q both map to Go field %s", prev, colName, goField)
	}
	goFields[goField] = colName

	typ, err := resolveType(col.Type.Name.Schema(), col.Type.Name.Base(), g.enumTypes)
	if err != nil {
		return nil, fmt.Errorf("column %q: %w", colName, err)
	}
	if typ.imp != "" {
		g.imports[typ.imp] = true
	}
	goType := typ.name
	if isNullable(col) && !pkFromIndex[colName] && !typ.nilable {
		goType = "rt.Null[" + goType + "]"
	}

	param := sqlParamName(colName)
	if prev, dup := params[param]; dup {
		return nil, fmt.Errorf("columns %q and %q both map to SQL parameter name %q", prev, colName, param)
	}
	params[param] = colName

	return &fieldPlan{
		col:        col,
		colName:    colName,
		goField:    goField,
		goType:     goType,
		param:      param,
		arg:        argName(goField),
		increment:  hasSetting(col, "increment"),
		hasDefault: col.Settings.Get("default") != nil,
		unique:     hasSetting(col, "unique"),
	}, nil
}

/* ===== naming helpers ===== */

// sqlTableName flattens the schema qualifier the same way gen/sqlite does:
// core.users -> core_users. The generated SQL must prepare against the
// generated DDL.
func sqlTableName(q *ast.QualName) string {
	if s := q.Schema(); s != "" && s != "public" {
		return s + "_" + q.Base()
	}
	return q.Base()
}

// sqlParamName derives a named-placeholder name from a column name.
// SQLite parameter names are ASCII alphanumerics plus underscore; anything
// else becomes an underscore ("full name" -> :full_name). Collisions are
// a generation error, caught by the caller.
func sqlParamName(colName string) string {
	var b strings.Builder
	for _, r := range colName {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	s := b.String()
	if s == "" || (s[0] >= '0' && s[0] <= '9') {
		s = "p" + s
	}
	return s
}

var goKeywords = map[string]bool{
	"break": true, "case": true, "chan": true, "const": true, "continue": true,
	"default": true, "defer": true, "else": true, "fallthrough": true,
	"for": true, "func": true, "go": true, "goto": true, "if": true,
	"import": true, "interface": true, "map": true, "package": true,
	"range": true, "return": true, "select": true, "struct": true,
	"switch": true, "type": true, "var": true,
}

// argName lowers an exported Go name into a positional-argument name:
// UserID -> userID, APIKey -> apiKey, ID -> id. Keywords and the receiver
// vocabulary get a trailing underscore.
func argName(exported string) string {
	runes := []rune(exported)
	upper := 0
	for upper < len(runes) && runes[upper] >= 'A' && runes[upper] <= 'Z' {
		upper++
	}
	switch {
	case upper == 0:
		// exported names start upper; non-ASCII uppercase falls here
		if len(runes) > 0 {
			runes[0] = []rune(strings.ToLower(string(runes[0])))[0]
		}
	case upper == len(runes) || upper == 1:
		for i := 0; i < upper; i++ {
			runes[i] = runes[i] - 'A' + 'a'
		}
	default:
		// leading initialism followed by a word: APIKey -> apiKey
		for i := 0; i < upper-1; i++ {
			runes[i] = runes[i] - 'A' + 'a'
		}
	}
	s := string(runes)
	if goKeywords[s] || s == "ctx" || s == "arg" || s == "q" {
		s += "_"
	}
	return s
}

// quoteSQLIdent double-quotes an identifier for SQLite.
func quoteSQLIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

func hasSetting(col *ast.Column, name string) bool {
	return col.Settings.Get(name) != nil
}

func hasLegacyFlag(col *ast.Column, name string) bool {
	for _, f := range col.LegacyFlags {
		if strings.EqualFold(f.Name(), name) {
			return true
		}
	}
	return false
}

/* ===== SQL rendering (shared by the emitter and the tests) ===== */

func (t *tableModel) columnList() string {
	names := make([]string, len(t.fields))
	for i, f := range t.fields {
		names[i] = quoteSQLIdent(f.colName)
	}
	return strings.Join(names, ", ")
}

func (t *tableModel) pkWhere() string {
	conds := make([]string, len(t.pk))
	for i, f := range t.pk {
		conds[i] = quoteSQLIdent(f.colName) + " = :" + f.param
	}
	return strings.Join(conds, " AND ")
}

func (t *tableModel) getSQL() string {
	return "SELECT " + t.columnList() + " FROM " + quoteSQLIdent(t.sqlName) + " WHERE " + t.pkWhere()
}

// getManySQL renders the IN-query prefix; the caller appends the
// placeholder list "(?, ?, ...)" at run time.
func (t *tableModel) getManySQL() string {
	return "SELECT " + t.columnList() + " FROM " + quoteSQLIdent(t.sqlName) + " WHERE " + quoteSQLIdent(t.pk[0].colName) + " IN"
}

func (t *tableModel) getBySQL(f *fieldPlan) string {
	return "SELECT " + t.columnList() + " FROM " + quoteSQLIdent(t.sqlName) + " WHERE " + quoteSQLIdent(f.colName) + " = :" + f.param
}

func (t *tableModel) listSQL() string {
	return "SELECT " + t.columnList() + " FROM " + quoteSQLIdent(t.sqlName)
}

func (t *tableModel) countSQL() string {
	return "SELECT count(*) FROM " + quoteSQLIdent(t.sqlName)
}

func (t *tableModel) createSQL() string {
	fields := t.createFields()
	if len(fields) == 0 {
		return "INSERT INTO " + quoteSQLIdent(t.sqlName) + " DEFAULT VALUES RETURNING " + t.columnList()
	}
	cols := make([]string, len(fields))
	vals := make([]string, len(fields))
	for i, f := range fields {
		cols[i] = quoteSQLIdent(f.colName)
		vals[i] = ":" + f.param
	}
	return "INSERT INTO " + quoteSQLIdent(t.sqlName) + " (" + strings.Join(cols, ", ") + ") VALUES (" +
		strings.Join(vals, ", ") + ") RETURNING " + t.columnList()
}

func (t *tableModel) updateSQL() string {
	sets := make([]string, 0, len(t.fields))
	for _, f := range t.nonPK() {
		sets = append(sets, quoteSQLIdent(f.colName)+" = :"+f.param)
	}
	return "UPDATE " + quoteSQLIdent(t.sqlName) + " SET " + strings.Join(sets, ", ") +
		" WHERE " + t.pkWhere() + " RETURNING " + t.columnList()
}

func (t *tableModel) deleteSQL() string {
	return "DELETE FROM " + quoteSQLIdent(t.sqlName) + " WHERE " + t.pkWhere()
}

// uniqueFields returns the columns served by a GetBy method: unique
// columns that are not already the (whole) primary key.
func (t *tableModel) uniqueFields() []*fieldPlan {
	var out []*fieldPlan
	for _, f := range t.fields {
		if !f.unique {
			continue
		}
		if len(t.pk) == 1 && t.pk[0] == f {
			continue // identical to Get
		}
		out = append(out, f)
	}
	return out
}

/* ===== the emitter ===== */

type queryEmitter struct {
	plan *plan
	opts Options
	out  strings.Builder
	body strings.Builder

	needStrings bool
}

func (e *queryEmitter) run() {
	for _, t := range e.plan.tables {
		e.emitTable(t)
	}
	e.header()
	e.prologue()
	e.out.WriteString(e.body.String())
}

func (e *queryEmitter) header() {
	fmt.Fprintf(&e.out, "// Code generated by nao gen from %s; DO NOT EDIT.\n", e.opts.Source)
	e.out.WriteString("//\n// CRUD methods over the models in nao_models.go. Identity arguments\n")
	e.out.WriteString("// are positional, data arguments ride in params structs, and the SQL\n")
	e.out.WriteString("// binds named placeholders — see docs/decisions.md D15-D17.\n\n")
	fmt.Fprintf(&e.out, "package %s\n\n", e.opts.Package)

	imports := map[string]bool{"context": true, "database/sql": true, rtImport: true}
	if e.needStrings {
		imports["strings"] = true
	}
	for imp := range e.plan.imports {
		imports[imp] = true
	}
	paths := make([]string, 0, len(imports))
	for p := range imports {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	e.out.WriteString("import (\n")
	for _, p := range paths {
		fmt.Fprintf(&e.out, "\t%q\n", p)
	}
	e.out.WriteString(")\n\n")
}

func (e *queryEmitter) prologue() {
	e.out.WriteString(`// Queries bundles the generated CRUD over one database handle: a
// *sql.DB, a *sql.Tx, or anything else satisfying rt.DBTX.
type Queries struct {
	db rt.DBTX
}

// New returns Queries running on db.
func New(db rt.DBTX) *Queries { return &Queries{db: db} }

// WithTx returns a copy of q running on tx.
func (q *Queries) WithTx(tx *sql.Tx) *Queries { return &Queries{db: tx} }

// Tx runs fn inside a transaction: the Queries handed to fn joins it, a
// nil error commits, anything else rolls back. q must be backed by a
// handle that can begin a transaction (a *sql.DB).
func (q *Queries) Tx(ctx context.Context, fn func(*Queries) error) error {
	return rt.Tx(ctx, q.db, func(tx *sql.Tx) error { return fn(q.WithTx(tx)) })
}

// rowScanner is the shared surface of *sql.Row and *sql.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

`)
}

func (e *queryEmitter) emitTable(t *tableModel) {
	if len(t.fields) == 0 {
		return // a columnless table has no queryable shape
	}
	b := &e.body
	lower := argName(t.model)
	tbl := t.ti.Decl.Name.String()

	// scan helper
	fmt.Fprintf(b, "// %sScan reads one %s row in generated column order.\n", lower, tbl)
	fmt.Fprintf(b, "func %sScan(r rowScanner) (%s, error) {\n\tvar v %s\n\terr := r.Scan(", lower, t.model, t.model)
	for i, f := range t.fields {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString("&v." + f.goField)
	}
	b.WriteString(")\n\treturn v, err\n}\n\n")

	if len(t.pk) > 0 {
		e.emitGet(t, lower, tbl)
		if len(t.pk) == 1 {
			e.emitGetMany(t, lower, tbl)
		}
	}
	for _, f := range t.uniqueFields() {
		e.emitGetBy(t, f, lower, tbl)
	}
	e.emitList(t, lower, tbl)
	e.emitCount(t, lower, tbl)
	e.emitCreate(t, lower, tbl)
	if len(t.pk) > 0 {
		if len(t.nonPK()) > 0 {
			e.emitUpdate(t, lower, tbl)
		}
		e.emitDelete(t, lower, tbl)
	}
}

// identityParams renders "id int64" / "userID int64, roleID int32".
func identityParams(t *tableModel) string {
	parts := make([]string, len(t.pk))
	for i, f := range t.pk {
		parts[i] = f.arg + " " + f.goType
	}
	return strings.Join(parts, ", ")
}

// identityArgs renders the sql.Named bindings for the key columns.
func identityArgs(t *tableModel) string {
	parts := make([]string, len(t.pk))
	for i, f := range t.pk {
		parts[i] = fmt.Sprintf("sql.Named(%q, %s)", f.param, f.arg)
	}
	return strings.Join(parts, ", ")
}

func (e *queryEmitter) emitGet(t *tableModel, lower, tbl string) {
	b := &e.body
	fmt.Fprintf(b, "const %sGetSQL = `%s`\n\n", lower, t.getSQL())
	fmt.Fprintf(b, "// %sGet fetches the %s row with the given primary key, or rt.ErrNotFound.\n", t.model, tbl)
	fmt.Fprintf(b, "func (q *Queries) %sGet(ctx context.Context, %s) (%s, error) {\n", t.model, identityParams(t), t.model)
	fmt.Fprintf(b, "\treturn %sScan(q.db.QueryRowContext(ctx, %sGetSQL, %s))\n}\n\n", lower, lower, identityArgs(t))
}

func (e *queryEmitter) emitGetMany(t *tableModel, lower, tbl string) {
	e.needStrings = true
	b := &e.body
	key := t.pk[0]
	fmt.Fprintf(b, "const %sGetManySQL = `%s`\n\n", lower, t.getManySQL())
	fmt.Fprintf(b, "// %sGetMany fetches the %s rows whose primary key is in %ss, in\n", t.model, tbl, key.arg)
	fmt.Fprintf(b, "// database order. Missing keys are not an error; they are just absent.\n")
	fmt.Fprintf(b, "func (q *Queries) %sGetMany(ctx context.Context, %ss []%s) ([]%s, error) {\n", t.model, key.arg, key.goType, t.model)
	fmt.Fprintf(b, "\tif len(%ss) == 0 {\n\t\treturn nil, nil\n\t}\n", key.arg)
	fmt.Fprintf(b, "\targs := make([]any, len(%ss))\n", key.arg)
	fmt.Fprintf(b, "\tfor i, k := range %ss {\n\t\targs[i] = k\n\t}\n", key.arg)
	fmt.Fprintf(b, "\tquery := %sGetManySQL + \" (?\" + strings.Repeat(\", ?\", len(%ss)-1) + \")\"\n", lower, key.arg)
	fmt.Fprintf(b, "\trows, err := q.db.QueryContext(ctx, query, args...)\n")
	e.scanRows(t, lower)
}

func (e *queryEmitter) emitGetBy(t *tableModel, f *fieldPlan, lower, tbl string) {
	b := &e.body
	fmt.Fprintf(b, "const %sGetBy%sSQL = `%s`\n\n", lower, f.goField, t.getBySQL(f))
	fmt.Fprintf(b, "// %sGetBy%s fetches the %s row with the given %s (a unique column),\n// or rt.ErrNotFound.\n", t.model, f.goField, tbl, f.colName)
	fmt.Fprintf(b, "func (q *Queries) %sGetBy%s(ctx context.Context, %s %s) (%s, error) {\n", t.model, f.goField, f.arg, f.goType, t.model)
	fmt.Fprintf(b, "\treturn %sScan(q.db.QueryRowContext(ctx, %sGetBy%sSQL, sql.Named(%q, %s)))\n}\n\n", lower, lower, f.goField, f.param, f.arg)
}

func (e *queryEmitter) emitList(t *tableModel, lower, tbl string) {
	b := &e.body
	fmt.Fprintf(b, "const %sListSQL = `%s`\n\n", lower, t.listSQL())
	fmt.Fprintf(b, "// %sList returns every %s row.\n", t.model, tbl)
	fmt.Fprintf(b, "func (q *Queries) %sList(ctx context.Context) ([]%s, error) {\n", t.model, t.model)
	fmt.Fprintf(b, "\trows, err := q.db.QueryContext(ctx, %sListSQL)\n", lower)
	e.scanRows(t, lower)
}

// scanRows emits the shared rows-loop tail of List and GetMany.
func (e *queryEmitter) scanRows(t *tableModel, lower string) {
	b := &e.body
	fmt.Fprintf(b, "\tif err != nil {\n\t\treturn nil, err\n\t}\n\tdefer rows.Close()\n")
	fmt.Fprintf(b, "\tvar out []%s\n\tfor rows.Next() {\n", t.model)
	fmt.Fprintf(b, "\t\tv, err := %sScan(rows)\n\t\tif err != nil {\n\t\t\treturn nil, err\n\t\t}\n\t\tout = append(out, v)\n\t}\n", lower)
	fmt.Fprintf(b, "\treturn out, rows.Err()\n}\n\n")
}

func (e *queryEmitter) emitCount(t *tableModel, lower, tbl string) {
	b := &e.body
	fmt.Fprintf(b, "const %sCountSQL = `%s`\n\n", lower, t.countSQL())
	fmt.Fprintf(b, "// %sCount reports the number of %s rows.\n", t.model, tbl)
	fmt.Fprintf(b, "func (q *Queries) %sCount(ctx context.Context) (int64, error) {\n", t.model)
	fmt.Fprintf(b, "\tvar n int64\n\terr := q.db.QueryRowContext(ctx, %sCountSQL).Scan(&n)\n\treturn n, err\n}\n\n", lower)
}

func (e *queryEmitter) emitCreate(t *tableModel, lower, tbl string) {
	b := &e.body
	fields := t.createFields()
	fmt.Fprintf(b, "const %sCreateSQL = `%s`\n\n", lower, t.createSQL())

	if len(fields) > 0 {
		fmt.Fprintf(b, "// %sCreateParams are the caller-supplied columns of %sCreate. The\n", t.model, t.model)
		fmt.Fprintf(b, "// auto-increment key and defaulted columns are the database's job (D16).\n")
		fmt.Fprintf(b, "type %sCreateParams struct {\n", t.model)
		e.paramFields(fields)
		b.WriteString("}\n\n")
	}

	fmt.Fprintf(b, "// %sCreate inserts a %s row and returns the row the database wrote,\n// generated keys and defaults filled in.\n", t.model, tbl)
	if len(fields) == 0 {
		fmt.Fprintf(b, "func (q *Queries) %sCreate(ctx context.Context) (%s, error) {\n", t.model, t.model)
		fmt.Fprintf(b, "\treturn %sScan(q.db.QueryRowContext(ctx, %sCreateSQL))\n}\n\n", lower, lower)
		return
	}
	fmt.Fprintf(b, "func (q *Queries) %sCreate(ctx context.Context, arg %sCreateParams) (%s, error) {\n", t.model, t.model, t.model)
	fmt.Fprintf(b, "\treturn %sScan(q.db.QueryRowContext(ctx, %sCreateSQL,\n", lower, lower)
	for _, f := range fields {
		fmt.Fprintf(b, "\t\tsql.Named(%q, arg.%s),\n", f.param, f.goField)
	}
	b.WriteString("\t))\n}\n\n")
}

func (e *queryEmitter) emitUpdate(t *tableModel, lower, tbl string) {
	b := &e.body
	fields := t.nonPK()
	fmt.Fprintf(b, "const %sUpdateSQL = `%s`\n\n", lower, t.updateSQL())

	fmt.Fprintf(b, "// %sUpdateParams are the data columns of %sUpdate: every column\n// outside the primary key.\n", t.model, t.model)
	fmt.Fprintf(b, "type %sUpdateParams struct {\n", t.model)
	e.paramFields(fields)
	b.WriteString("}\n\n")

	fmt.Fprintf(b, "// %sUpdate rewrites every non-key column of the identified %s row\n", t.model, tbl)
	fmt.Fprintf(b, "// and returns the result, or rt.ErrNotFound when no row matches.\n")
	fmt.Fprintf(b, "func (q *Queries) %sUpdate(ctx context.Context, %s, arg %sUpdateParams) (%s, error) {\n", t.model, identityParams(t), t.model, t.model)
	fmt.Fprintf(b, "\treturn %sScan(q.db.QueryRowContext(ctx, %sUpdateSQL,\n", lower, lower)
	for _, f := range t.pk {
		fmt.Fprintf(b, "\t\tsql.Named(%q, %s),\n", f.param, f.arg)
	}
	for _, f := range fields {
		fmt.Fprintf(b, "\t\tsql.Named(%q, arg.%s),\n", f.param, f.goField)
	}
	b.WriteString("\t))\n}\n\n")
}

func (e *queryEmitter) emitDelete(t *tableModel, lower, tbl string) {
	b := &e.body
	fmt.Fprintf(b, "const %sDeleteSQL = `%s`\n\n", lower, t.deleteSQL())
	fmt.Fprintf(b, "// %sDelete removes the identified %s row; rt.ErrNotFound reports\n// that nothing matched.\n", t.model, tbl)
	fmt.Fprintf(b, "func (q *Queries) %sDelete(ctx context.Context, %s) error {\n", t.model, identityParams(t))
	fmt.Fprintf(b, "\tres, err := q.db.ExecContext(ctx, %sDeleteSQL, %s)\n", lower, identityArgs(t))
	fmt.Fprintf(b, "\tif err != nil {\n\t\treturn err\n\t}\n")
	fmt.Fprintf(b, "\tn, err := res.RowsAffected()\n\tif err != nil {\n\t\treturn err\n\t}\n")
	fmt.Fprintf(b, "\tif n == 0 {\n\t\treturn rt.ErrNotFound\n\t}\n\treturn nil\n}\n\n")
}

// paramFields renders the fields of a params struct, notes as doc comments.
func (e *queryEmitter) paramFields(fields []*fieldPlan) {
	b := &e.body
	for _, f := range fields {
		if note := settingNote(f.col.Settings); note != "" {
			writeCommentIndent(b, note)
		}
		fmt.Fprintf(b, "\t%s %s `db:%q json:%q`\n", f.goField, f.goType, f.colName, f.colName)
	}
}
