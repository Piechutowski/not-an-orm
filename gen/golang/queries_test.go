package golang

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"

	"github.com/Piechutowski/not-an-orm/edbml/check"
	"github.com/Piechutowski/not-an-orm/edbml/diag"
	sqlitegen "github.com/Piechutowski/not-an-orm/gen/sqlite"
	"github.com/Piechutowski/not-an-orm/edbml/parser"
)

// planStatements re-derives every SQL statement the emitter writes,
// mirroring tableEmit's conditions (D17). Used by the prepare test below.
func planStatements(t *testing.T, p *plan) map[string]string {
	t.Helper()
	stmts := map[string]string{}
	for _, tm := range p.tables {
		if len(tm.fields) == 0 {
			continue
		}
		if len(tm.pk) > 0 {
			stmts[tm.model+"Get"] = tm.getSQL()
			if len(tm.pk) == 1 {
				// the emitter appends the placeholder list at run time
				stmts[tm.model+"GetMany"] = tm.getManySQL() + " (?)"
			}
			if len(tm.nonPK()) > 0 {
				stmts[tm.model+"Update"] = tm.updateSQL()
			}
			stmts[tm.model+"Delete"] = tm.deleteSQL()
		}
		for _, f := range tm.uniqueFields() {
			stmts[tm.model+"GetBy"+f.goField] = tm.getBySQL(f)
		}
		stmts[tm.model+"List"] = tm.listSQL()
		stmts[tm.model+"Count"] = tm.countSQL()
		stmts[tm.model+"Create"] = tm.createSQL()
	}
	return stmts
}

// TestQueriesPrepare proves cross-generator coherence (D02, D31): every
// statement gen/golang emits must prepare on a real SQLite loaded with the
// DDL gen/sqlite emits from the same schema. EXPLAIN compiles a statement
// without running it; unbound parameters are bound NULL via a defaultdict.
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
for name, stmt in sorted(inp["stmts"].items()):
    # GetMany appends a positional "(?)" list; everything else binds :names
    params = [None] if "?" in stmt else named
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
			input, err := json.Marshal(map[string]any{
				"ddl":   string(ddl),
				"stmts": planStatements(t, p),
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
