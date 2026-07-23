package golang

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"

	"github.com/Piechutowski/not-an-orm/edbml/check"
	"github.com/Piechutowski/not-an-orm/edbml/diag"
	"github.com/Piechutowski/not-an-orm/edbml/parser"
	sqlitegen "github.com/Piechutowski/not-an-orm/gen/sqlite"
	"github.com/Piechutowski/not-an-orm/rt"
)

// stmtCase is one statement for the prepare test: named-parameter
// statements bind a NULL defaultdict (N = -1); positional statements
// bind exactly N NULLs.
type stmtCase struct {
	SQL string `json:"sql"`
	N   int    `json:"n"`
}

// planStatements re-derives every SQL statement the CRUD emitter writes,
// mirroring tableEmit's conditions (D17). Used by the prepare test below.
func planStatements(t *testing.T, p *plan) map[string]stmtCase {
	t.Helper()
	named := func(sql string) stmtCase { return stmtCase{SQL: sql, N: -1} }
	stmts := map[string]stmtCase{}
	for _, tm := range p.tables {
		if len(tm.fields) == 0 {
			continue
		}
		if len(tm.pk) > 0 {
			stmts[tm.model+"Get"] = named(tm.getSQL())
			if len(tm.pk) == 1 {
				// the emitter appends the placeholder list at run time
				stmts[tm.model+"GetMany"] = stmtCase{SQL: tm.getManySQL() + " (?)", N: 1}
			}
			if len(tm.nonPK()) > 0 {
				stmts[tm.model+"Update"] = named(tm.updateSQL())
			}
			stmts[tm.model+"Delete"] = named(tm.deleteSQL())
		}
		for _, f := range tm.uniqueFields() {
			stmts[tm.model+"GetBy"+f.goField] = named(tm.getBySQL(f))
		}
		stmts[tm.model+"List"] = named(tm.listSQL())
		stmts[tm.model+"Create"] = named(tm.createSQL())
	}
	return stmts
}

// planDynStatements renders, per table, what the dynamic layer would:
// a kitchen-sink SELECT touching every column as filter, order and
// keyset plus paging, and the Count/Exists/DeleteWhere/UpdateWhere
// statements. The phantom model type is irrelevant to rendering, so a
// throwaway struct instantiates the generics.
func planDynStatements(t *testing.T, p *plan) map[string]stmtCase {
	t.Helper()
	type m = struct{}
	stmts := map[string]stmtCase{}
	add := func(name, sql string, args []any, err error) {
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		stmts[name] = stmtCase{SQL: sql, N: len(args)}
	}
	for _, tm := range p.tables {
		if len(tm.fields) == 0 {
			continue
		}
		table := sqlIdentQuote(tm.sqlName)
		cols := tm.columnList()
		var preds []rt.Pred[m]
		var orders []rt.Order[m]
		var key []any
		for _, f := range tm.fields {
			c := rt.Column[m, any]{Name: f.colName}
			preds = append(preds, c.Eq(nil))
			orders = append(orders, c.Asc())
			key = append(key, nil)
		}
		first := rt.Column[m, any]{Name: tm.fields[0].colName}

		sql, args, err := rt.SelectRender(table, cols, []rt.Opt[m]{
			rt.And(preds...),
			rt.Or(first.IsNull(), rt.Not(first.Eq(nil)), first.In(nil, nil), first.EqCol(first)),
			rt.Distinct[m](),
			rt.OrderBy(orders...),
			rt.After[m](key...),
			rt.Limit[m](10),
			rt.Offset[m](5),
		})
		add(tm.model+"DynQuery", sql, args, err)

		sql, args, err = rt.CountRender(table, preds)
		add(tm.model+"DynCount", sql, args, err)
		sql, args, err = rt.ExistsRender(table, preds)
		add(tm.model+"DynExists", sql, args, err)
		sql, args, err = rt.DeleteRender(table, []rt.Pred[m]{first.Eq(nil)})
		add(tm.model+"DynDeleteWhere", sql, args, err)
		sql, args, err = rt.UpdateRender(table, []rt.Assign[m]{first.Set(nil)}, []rt.Pred[m]{first.Eq(nil)})
		add(tm.model+"DynUpdateWhere", sql, args, err)
	}
	return stmts
}

// TestQueriesPrepare proves cross-generator coherence (D02, D31): every
// statement gen/golang emits — CRUD and the dynamic layer's rendered
// shapes alike — must prepare on a real SQLite loaded with the DDL
// gen/sqlite emits from the same schema. EXPLAIN compiles a statement
// without running it; named parameters bind NULL via a defaultdict,
// positional ones bind exactly as many NULLs as the interpreter bound.
func TestQueriesPrepare(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	const driver = `
import sqlite3, sys, json, collections, datetime, uuid
inp = json.load(sys.stdin)
con = sqlite3.connect(":memory:")
con.execute("PRAGMA foreign_keys = ON")
def _now(): return datetime.datetime.now().isoformat(sep=" ")
con.create_function("now", 0, _now, deterministic=True)
con.create_function("getdate", 0, _now, deterministic=True)
con.create_function("uuid_generate_v4", 0, lambda: str(uuid.uuid4()), deterministic=True)
con.executescript(inp["ddl"])
named = collections.defaultdict(lambda: None)
for name, case in sorted(inp["stmts"].items()):
    stmt, n = case["sql"], case["n"]
    params = named if n < 0 else [None] * n
    try:
        con.execute("EXPLAIN " + stmt, params)
    except Exception as e:
        sys.exit(f"{name}: {e}\n  {stmt}")
`
	for _, dbml := range corpusSchemas(t) {
		dbml := dbml
		t.Run(strings.TrimSuffix(strings.TrimPrefix(dbml, "../testdata/"), ".dbml"), func(t *testing.T) {
			f, info := parseChecked(t, dbml)
			ddl, err := sqlitegen.Generate(f, info, sqlitegen.Options{Source: dbml})
			if err != nil {
				t.Fatalf("gen/sqlite: %v", err)
			}
			p, err := planBuild(f, info)
			if err != nil {
				t.Fatalf("planBuild: %v", err)
			}
			stmts := planStatements(t, p)
			for name, c := range planDynStatements(t, p) {
				stmts[name] = c
			}
			input, err := json.Marshal(map[string]any{
				"ddl":   string(ddl),
				"stmts": stmts,
			})
			if err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command("python3", "-c", driver)
			cmd.Stdin = strings.NewReader(string(input))
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Errorf("statement does not prepare against the generated DDL: %v\n%s", err, out)
			}
		})
	}
}

// TestQueriesGenerationErrors pins the loud failure modes specific to the
// queries generator.
func TestQueriesGenerationErrors(t *testing.T) {
	cases := []struct {
		name, dbml, wantErr string
	}{
		{
			name:    "model collision after singularization",
			dbml:    "Table users { id int [pk] }\nTable \"user\" { id int [pk] }",
			wantErr: "both map to Go type",
		},
		{
			name:    "reserved model name",
			dbml:    "Table jobs [model: 'Queries'] { id int [pk] }",
			wantErr: "Queries",
		},
		{
			name:    "sql parameter collision",
			dbml:    "Table t { \"żb\" int [pk]\n \"įb\" int }",
			wantErr: "SQL parameter name",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, diags := parser.ParseFile("t", tc.dbml)
			info, semDiags := check.File(f)
			if diag.HasErrors(append(diags, semDiags...)) {
				t.Fatalf("input unexpectedly invalid: %v %v", diags, semDiags)
			}
			_, err := GenerateQueries(f, info, Options{Package: "models", Source: "t"})
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("want error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestArgNames(t *testing.T) {
	cases := []struct{ in, want string }{
		{"ID", "id"},
		{"UserID", "userID"},
		{"APIKey", "apiKey"},
		{"HTTPStatus", "httpStatus"},
		{"Name", "name"},
		{"X2faCode", "x2faCode"},
		{"Type", "type_"}, // Go keyword
		{"Ctx", "ctx_"},   // receiver vocabulary
	}
	for _, tc := range cases {
		if got := argName(tc.in); got != tc.want {
			t.Errorf("argName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestSQLParamNames(t *testing.T) {
	cases := []struct{ in, want string }{
		{"email", "email"},
		{"full name", "full_name"},
		{"user_id", "user_id"},
		{"2fa", "p2fa"},
		{"żółć", "____"},
	}
	for _, tc := range cases {
		if got := sqlParamName(tc.in); got != tc.want {
			t.Errorf("sqlParamName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
