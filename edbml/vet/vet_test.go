package vet_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/Piechutowski/not-an-orm/edbml/check"
	"github.com/Piechutowski/not-an-orm/edbml/diag"
	"github.com/Piechutowski/not-an-orm/edbml/parser"
	"github.com/Piechutowski/not-an-orm/edbml/vet"
)

// The testdata format follows go/analysis's analysistest in miniature:
//
//	// analyzers: name1,name2     <- first line: which analyzers to run
//	Table t {                     //WANT name1
//
// Every //WANT marker must be matched by a warning of that analyzer on
// that line, and every produced warning must be matched by a marker.
var wantRE = regexp.MustCompile(`//WANT ([a-z,]+)`)

func TestAnalyzers(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("testdata", "*.dbml"))
	if err != nil || len(files) == 0 {
		t.Fatal("no testdata files")
	}
	for _, file := range files {
		file := file
		t.Run(filepath.Base(file), func(t *testing.T) {
			src, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(string(src), "\n")
			header := strings.TrimPrefix(strings.TrimSpace(lines[0]), "// analyzers:")
			var analyzers []*vet.Analyzer
			for _, name := range strings.Split(header, ",") {
				a := vet.ByName(strings.TrimSpace(name))
				if a == nil {
					t.Fatalf("unknown analyzer %q in header", name)
				}
				analyzers = append(analyzers, a)
			}

			// expectations: line -> analyzer names still unmatched
			want := map[int][]string{}
			for i, ln := range lines {
				if m := wantRE.FindStringSubmatch(ln); m != nil {
					for _, n := range strings.Split(m[1], ",") {
						want[i+1] = append(want[i+1], n)
					}
				}
			}

			f, diags := parser.ParseFile(file, string(src))
			if diag.HasErrors(diags) {
				t.Fatalf("test input must be valid DBML; parse errors: %v", diags)
			}
			info, semDiags := check.File(f)
			if diag.HasErrors(semDiags) {
				t.Fatalf("test input must be valid DBML; check errors: %v", semDiags)
			}

			for _, w := range vet.Run(f, info, analyzers...) {
				name := strings.TrimPrefix(w.Code, "vet/")
				matched := false
				rest := want[w.Pos.Line][:0]
				for _, n := range want[w.Pos.Line] {
					if !matched && n == name {
						matched = true
						continue
					}
					rest = append(rest, n)
				}
				want[w.Pos.Line] = rest
				if !matched {
					t.Errorf("unexpected warning at line %d: %s", w.Pos.Line, w)
				}
			}
			for line, names := range want {
				for _, n := range names {
					t.Errorf("line %d: expected %s warning, got none", line, n)
				}
			}
		})
	}
}

// TestCleanFile pins the zero-noise property: a well-formed schema should
// produce no warnings at all from any analyzer.
func TestCleanFile(t *testing.T) {
	src := `
Project shop { database_type: 'PostgreSQL' }
Enum status { active
  retired }
Table users as U {
  id int [pk, increment]
  status status [not null]
}
Table orders {
  id int [pk]
  user_id int [not null, ref: > U.id]
}
TableGroup commerce { users
  orders }
`
	f, diags := parser.ParseFile("clean.dbml", src)
	info, semDiags := check.File(f)
	diags = append(diags, semDiags...)
	if diag.HasErrors(diags) {
		t.Fatalf("clean file has errors: %v", diags)
	}
	if ws := vet.Run(f, info); len(ws) > 0 {
		t.Errorf("clean file produced warnings: %v", ws)
	}
}

func TestAnalyzerDocs(t *testing.T) {
	for _, a := range vet.All() {
		if a.Name == "" || a.Doc == "" || a.Run == nil {
			t.Errorf("analyzer %+v is missing name, doc or run", a)
		}
	}
}
