package sqlite

import (
	"bytes"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/Piechutowski/not-an-orm/edbml/check"
	"github.com/Piechutowski/not-an-orm/edbml/diag"
	"github.com/Piechutowski/not-an-orm/edbml/parser"
)

var update = flag.Bool("update", false, "rewrite golden files")

func generate(t *testing.T, dbmlPath string) []byte {
	t.Helper()
	src, err := os.ReadFile(dbmlPath)
	if err != nil {
		t.Fatal(err)
	}
	f, diags := parser.ParseFile(filepath.Base(dbmlPath), string(src))
	info, semDiags := check.File(f)
	diags = append(diags, semDiags...)
	if diag.HasErrors(diags) {
		t.Fatalf("test input must be valid DBML: %v", diags)
	}
	out, err := Generate(f, info, Options{Source: filepath.Base(dbmlPath)})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	return out
}

// TestGolden byte-compares generated DDL for the shared corpus in
// gen/testdata against .sql.golden files. Run with -update to accept
// intentional changes.
func TestGolden(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "testdata", "*.dbml"))
	if err != nil {
		t.Fatal(err)
	}
	extended, err := filepath.Glob(filepath.Join("..", "testdata", "*.edbml"))
	if err != nil {
		t.Fatal(err)
	}
	files = append(files, extended...)
	sort.Strings(files)
	if len(files) == 0 {
		t.Fatal("no shared corpus schemas in gen/testdata")
	}
	for _, dbml := range files {
		dbml := dbml
		t.Run(filepath.Base(dbml), func(t *testing.T) {
			got := generate(t, dbml)
			golden := filepath.Join("testdata", strings.TrimSuffix(strings.TrimSuffix(filepath.Base(dbml), ".dbml"), ".edbml")+".sql.golden")
			if *update {
				if err := os.WriteFile(golden, got, 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("missing golden file (run 'go test ./gen/... -update'): %v", err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("output differs from %s:\n--- got ---\n%s", golden, got)
			}
		})
	}
}

// TestGoldenExecutes runs every golden schema through a real SQLite engine
// (via Python's sqlite3 module) with foreign keys enforced — the analogue
// of gen/golang's compile test. Host-dialect functions that appear in
// opaque backtick expressions (now, getdate, uuid_generate_v4) are
// registered as deterministic shims so DEFAULT clauses resolve.
func TestGoldenExecutes(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	const driver = `
import sqlite3, sys
con = sqlite3.connect(":memory:")
con.execute("PRAGMA foreign_keys = ON")
import datetime, uuid
def _now(): return datetime.datetime.now().isoformat(sep=" ")
con.create_function("now", 0, _now, deterministic=True)
con.create_function("getdate", 0, _now, deterministic=True)
con.create_function("uuid_generate_v4", 0, lambda: str(uuid.uuid4()), deterministic=True)
con.executescript(sys.stdin.read())
con.execute("PRAGMA foreign_key_check").fetchall() and sys.exit("foreign key violations")
`
	for _, golden := range goldenFiles(t) {
		golden := golden
		t.Run(filepath.Base(golden), func(t *testing.T) {
			src, err := os.ReadFile(golden)
			if err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command("python3", "-c", driver)
			cmd.Stdin = bytes.NewReader(src)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Errorf("SQLite rejected %s: %v\n%s", golden, err, out)
			}
		})
	}
}

// TestGeneratedHeader pins the generated-code marker on the first line.
func TestGeneratedHeader(t *testing.T) {
	re := regexp.MustCompile(`^-- Code generated .* DO NOT EDIT\.$`)
	for _, golden := range goldenFiles(t) {
		src, err := os.ReadFile(golden)
		if err != nil {
			t.Fatal(err)
		}
		first := strings.SplitN(string(src), "\n", 2)[0]
		if !re.MatchString(first) {
			t.Errorf("%s first line %q does not match the generated-code convention", golden, first)
		}
	}
}

func goldenFiles(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join("testdata", "*.sql.golden"))
	if err != nil || len(files) == 0 {
		t.Fatal("no golden files; run 'go test ./gen/... -update' first")
	}
	return files
}

// TestGenerationErrors pins the strict failure modes.
func TestGenerationErrors(t *testing.T) {
	cases := []struct {
		name, dbml, wantErr string
	}{
		{
			name:    "unknown type",
			dbml:    "Table t { id int [pk]\n loc geography }",
			wantErr: "no SQLite mapping",
		},
		{
			name:    "flattened table collision",
			dbml:    "Table core.users { id int }\nTable core_users { id int }",
			wantErr: "both flatten to SQLite name",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, diags := parser.ParseFile("t", tc.dbml)
			info, semDiags := check.File(f)
			if diag.HasErrors(append(diags, semDiags...)) {
				t.Fatalf("input unexpectedly invalid: %v %v", diags, semDiags)
			}
			_, err := Generate(f, info, Options{Source: "t"})
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("want error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestQuoting(t *testing.T) {
	cases := []struct{ in, want string }{
		{"users", "users"},
		{"order", `"order"`},         // keyword
		{"full name", `"full name"`}, // space
		{"2fa_codes", `"2fa_codes"`}, // digit-leading
		{"Users", `"Users"`},         // upper case is not plain
		{`we"ird`, `"we""ird"`},      // embedded quote doubled
	}
	for _, tc := range cases {
		if got := identQuote(tc.in); got != tc.want {
			t.Errorf("identQuote(%q) = %s, want %s", tc.in, got, tc.want)
		}
	}
	if got := stringQuote("it's"); got != "'it''s'" {
		t.Errorf("stringQuote = %s", got)
	}
}
