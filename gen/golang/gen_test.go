package golang

import (
	"bytes"
	"flag"
	"go/format"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/Piechutowski/not-an-orm/edbml/ast"
	"github.com/Piechutowski/not-an-orm/edbml/check"
	"github.com/Piechutowski/not-an-orm/edbml/diag"
	"github.com/Piechutowski/not-an-orm/edbml/parser"
)

var update = flag.Bool("update", false, "rewrite golden files")

func parseChecked(t *testing.T, dbmlPath string) (*ast.File, *check.Info) {
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
	return f, info
}

func generate(t *testing.T, dbmlPath string) []byte {
	t.Helper()
	f, info := parseChecked(t, dbmlPath)
	out, err := Generate(f, info, Options{Package: "models", Source: filepath.Base(dbmlPath)})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	return out
}

func generateQueries(t *testing.T, dbmlPath string) []byte {
	t.Helper()
	f, info := parseChecked(t, dbmlPath)
	out, err := GenerateQueries(f, info, Options{Package: "models", Source: filepath.Base(dbmlPath)})
	if err != nil {
		t.Fatalf("GenerateQueries: %v", err)
	}
	return out
}

func compareGolden(t *testing.T, got []byte, golden string) {
	t.Helper()
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
}

// TestGolden regenerates every testdata schema — models and queries — and
// byte-compares against the .go.golden files. Run with -update to accept
// intentional changes.
func TestGolden(t *testing.T) {
	for _, dbml := range corpusSchemas(t) {
		dbml := dbml
		name := strings.TrimSuffix(filepath.Base(dbml), ".dbml")
		t.Run(name, func(t *testing.T) {
			compareGolden(t, generate(t, dbml), filepath.Join("testdata", name+".go.golden"))
			compareGolden(t, generateQueries(t, dbml), filepath.Join("testdata", name+"_queries.go.golden"))
		})
	}
}

func corpusSchemas(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join("..", "testdata", "*.dbml"))
	if err != nil || len(files) == 0 {
		t.Fatal("no shared corpus schemas in gen/testdata")
	}
	return files
}

// TestGoldenGofmtStable proves the generated code is gofmt-clean: applying
// gofmt to a golden file must be the identity function.
func TestGoldenGofmtStable(t *testing.T) {
	for _, golden := range goldenFiles(t) {
		src, err := os.ReadFile(golden)
		if err != nil {
			t.Fatal(err)
		}
		formatted, err := format.Source(src)
		if err != nil {
			t.Errorf("%s does not parse: %v", golden, err)
			continue
		}
		if !bytes.Equal(src, formatted) {
			t.Errorf("%s is not gofmt-stable", golden)
		}
	}
}

// TestGoldenCompiles builds every golden file with the real Go toolchain:
// generated code must be valid, compilable Go, not merely parseable.
func TestGoldenCompiles(t *testing.T) {
	dir := t.TempDir()
	// Generated code imports the rt runtime package (D03/D13); point the
	// throwaway module back at this repository for it.
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	gomod := "module goldencheck\n\ngo 1.24\n\n" +
		"require github.com/Piechutowski/not-an-orm v0.0.0\n\n" +
		"replace github.com/Piechutowski/not-an-orm => " + repoRoot + "\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(gomod), 0o644); err != nil {
		t.Fatal(err)
	}
	// One package per schema, holding both generated files: the queries
	// reference the model structs, so they must compile together.
	for _, golden := range goldenFiles(t) {
		src, err := os.ReadFile(golden)
		if err != nil {
			t.Fatal(err)
		}
		base := filepath.Base(strings.TrimSuffix(golden, ".go.golden"))
		outName := "dbml_models.go"
		if strings.HasSuffix(base, "_queries") {
			base, outName = strings.TrimSuffix(base, "_queries"), "dbml_queries.go"
		}
		pkgDir := filepath.Join(dir, "p", base)
		if err := os.MkdirAll(pkgDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(pkgDir, outName), src, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command("go", "build", "./...")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("golden files do not compile:\n%s", out)
	}
}

// TestGeneratedHeader pins the machine-readable generated-code marker
// (https://golang.org/s/generatedcode) on the first line of every golden.
func TestGeneratedHeader(t *testing.T) {
	re := regexp.MustCompile(`^// Code generated .* DO NOT EDIT\.$`)
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
	files, err := filepath.Glob(filepath.Join("testdata", "*.go.golden"))
	if err != nil || len(files) == 0 {
		t.Fatal("no golden files; run 'go test ./gen/... -update' first")
	}
	return files
}

// TestGenerationErrors pins the strict failure modes: unknown types and Go
// name collisions must fail loudly, never guess.
func TestGenerationErrors(t *testing.T) {
	cases := []struct {
		name, dbml, wantErr string
	}{
		{
			name:    "unknown type",
			dbml:    "Table t { id int [pk]\n loc geography }",
			wantErr: "no Go mapping",
		},
		{
			name:    "field collision",
			dbml:    "Table t { user_id int\n \"user__id\" int }",
			wantErr: "both map to Go field",
		},
		{
			name:    "struct collision",
			dbml:    "Table user_roles { id int }\nTable \"user roles\" { id int }",
			wantErr: "both map to Go type",
		},
		{
			name:    "unusable name",
			dbml:    "Table t { \"++\" int }",
			wantErr: "no characters usable",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, diags := parser.ParseFile("t", tc.dbml)
			info, semDiags := check.File(f)
			if diag.HasErrors(append(diags, semDiags...)) {
				t.Fatalf("input unexpectedly invalid: %v %v", diags, semDiags)
			}
			_, err := Generate(f, info, Options{Package: "models", Source: "t"})
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("want error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestGoNames(t *testing.T) {
	cases := []struct{ in, want string }{
		{"user_id", "UserID"},
		{"api_key", "APIKey"},
		{"http_status_url", "HTTPStatusURL"},
		{"uuid", "UUID"},
		{"full name", "FullName"},
		{"2fa_codes", "X2faCodes"},
		{"created_at", "CreatedAt"},
		{"id", "ID"},
		{"żółć", "Żółć"},
	}
	for _, tc := range cases {
		got, err := goName(tc.in)
		if err != nil {
			t.Errorf("goName(%q): %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("goName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
	if _, err := goName("__"); err == nil {
		t.Error("goName(__): want error")
	}
}
