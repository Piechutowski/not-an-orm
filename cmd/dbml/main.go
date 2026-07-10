// Command dbml is the CLI front end over the scanner/parser/check/vet
// packages, built on urfave/cli:
//
//	dbml parse [--json] file.dbml...    syntax only
//	dbml check [--json] file.dbml...    syntax + semantic errors
//	dbml vet [--json] [--enable a,b] [--werror] file.dbml...
//	                                    syntax + semantics + warnings
//	dbml analyzers                      list vet analyzers
//
// Exit status: 0 clean (warnings do not fail), 1 errors found (or any
// finding under --werror), 2 usage or I/O problems.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/urfave/cli/v3"

	"github.com/Piechutowski/not-an-orm/check"
	"github.com/Piechutowski/not-an-orm/diag"
	golanggen "github.com/Piechutowski/not-an-orm/gen/golang"
	sqlitegen "github.com/Piechutowski/not-an-orm/gen/sqlite"
	"github.com/Piechutowski/not-an-orm/parser"
	"github.com/Piechutowski/not-an-orm/vet"
)

func main() {
	jsonFlag := &cli.BoolFlag{Name: "json", Usage: "emit diagnostics as JSON"}

	app := &cli.Command{
		EnableShellCompletion: true,
		Name:  "dbml",
		Usage: "parse, check and lint DBML schema files",
		Commands: []*cli.Command{
			{
				Name:      "parse",
				Usage:     "tokenize and parse; report syntax errors only",
				ArgsUsage: "file.dbml...",
				Flags:     []cli.Flag{jsonFlag},
				Action: func(_ context.Context, c *cli.Command) error {
					return run(c, "parse")
				},
			},
			{
				Name:      "check",
				Usage:     "parse and run semantic analysis (spec §4-§8)",
				ArgsUsage: "file.dbml...",
				Flags:     []cli.Flag{jsonFlag},
				Action: func(_ context.Context, c *cli.Command) error {
					return run(c, "check")
				},
			},
			{
				Name:      "vet",
				Usage:     "check plus warnings for legal-but-suspicious DBML",
				ArgsUsage: "file.dbml...",
				Flags: []cli.Flag{
					jsonFlag,
					&cli.StringFlag{Name: "enable", Usage: "comma-separated analyzer `names` to run (default all; see 'dbml analyzers')"},
					&cli.BoolFlag{Name: "werror", Usage: "treat warnings as errors in the exit status"},
				},
				Action: func(_ context.Context, c *cli.Command) error {
					return run(c, "vet")
				},
			},
			{
				Name:  "gen",
				Usage: "generate code from a DBML file",
				Commands: []*cli.Command{
					{
						Name:      "go",
						Usage:     "generate Go model structs and CRUD (dbml_models.go, dbml_queries.go)",
						ArgsUsage: "file.dbml",
						Flags: []cli.Flag{
							&cli.StringFlag{Name: "out", Required: true, Usage: "output `directory` for the generated .go files"},
							&cli.StringFlag{Name: "package", Usage: "package `name` (default: sanitized output directory name)"},
						},
						Action: func(_ context.Context, c *cli.Command) error {
							return runGen(c, "go")
						},
					},
					{
						Name:      "sqlite",
						Usage:     "generate SQLite DDL and seed inserts (dbml_schema.sql)",
						ArgsUsage: "file.dbml",
						Flags: []cli.Flag{
							&cli.StringFlag{Name: "out", Required: true, Usage: "output `directory` for dbml_schema.sql"},
						},
						Action: func(_ context.Context, c *cli.Command) error {
							return runGen(c, "sqlite")
						},
					},
				},
			},
			{
				Name:  "analyzers",
				Usage: "list vet analyzers (documented in vet/RULES.md)",
				Action: func(_ context.Context, _ *cli.Command) error {
					for _, a := range vet.All() {
						fmt.Printf("%-18s %s\n", a.Name, a.Doc)
					}
					return nil
				},
			},
		},
	}

	if err := app.Run(context.Background(), os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}

func run(c *cli.Command, mode string) error {
	files := c.Args().Slice()
	if len(files) == 0 {
		return cli.Exit("no input files", 2)
	}
	analyzers, err := selectedAnalyzers(c, mode)
	if err != nil {
		return err
	}

	var all []diag.Diagnostic
	for _, file := range files {
		src, err := os.ReadFile(file)
		if err != nil {
			return cli.Exit(err.Error(), 2)
		}
		all = append(all, analyze(mode, file, string(src), analyzers)...)
	}

	if c.Bool("json") {
		if err := printJSON(all); err != nil {
			return cli.Exit(err.Error(), 2)
		}
	} else {
		for _, d := range all {
			fmt.Println(d)
		}
	}

	if diag.HasErrors(all) || (mode == "vet" && c.Bool("werror") && len(all) > 0) {
		return cli.Exit("", 1)
	}
	return nil
}

func selectedAnalyzers(c *cli.Command, mode string) ([]*vet.Analyzer, error) {
	if mode != "vet" || c.String("enable") == "" {
		return nil, nil // nil means "all" for vet.Run
	}
	var out []*vet.Analyzer
	for _, n := range strings.Split(c.String("enable"), ",") {
		a := vet.ByName(strings.TrimSpace(n))
		if a == nil {
			return nil, cli.Exit(fmt.Sprintf("unknown analyzer %q (see 'dbml analyzers')", n), 2)
		}
		out = append(out, a)
	}
	return out, nil
}

func analyze(mode, name, src string, analyzers []*vet.Analyzer) []diag.Diagnostic {
	f, diags := parser.ParseFile(name, src)
	if mode == "parse" {
		return diags
	}
	info, semDiags := check.File(f)
	diags = append(diags, semDiags...)
	if mode == "vet" {
		diags = append(diags, vet.Run(f, info, analyzers...)...)
	}
	diag.Sort(diags)
	return diags
}

func printJSON(all []diag.Diagnostic) error {
	type jsonDiag struct {
		File     string `json:"file"`
		Line     int    `json:"line"`
		Column   int    `json:"column"`
		Severity string `json:"severity"`
		Code     string `json:"code"`
		Message  string `json:"message"`
	}
	out := make([]jsonDiag, 0, len(all))
	for _, d := range all {
		out = append(out, jsonDiag{
			File: d.Pos.Filename, Line: d.Pos.Line, Column: d.Pos.Column,
			Severity: d.Severity.String(), Code: d.Code, Message: d.Msg,
		})
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// runGen implements 'dbml gen <lang>': parse + check one DBML file,
// generate into --out, refusing to clobber non-generated files.
func runGen(c *cli.Command, lang string) error {
	if c.Args().Len() != 1 {
		return cli.Exit("gen takes exactly one DBML file", 2)
	}
	file := c.Args().First()
	src, err := os.ReadFile(file)
	if err != nil {
		return cli.Exit(err.Error(), 2)
	}
	f, diags := parser.ParseFile(file, string(src))
	info, semDiags := check.File(f)
	diags = append(diags, semDiags...)
	if diag.HasErrors(diags) {
		diag.Sort(diags)
		for _, d := range diags {
			fmt.Println(d)
		}
		return cli.Exit("gen: input has errors; fix them first (see 'dbml check')", 1)
	}

	outDir := c.String("out")
	type output struct {
		name   string
		code   []byte
		marker string
	}
	var outputs []output
	switch lang {
	case "go":
		pkg := c.String("package")
		if pkg == "" {
			abs, err := filepath.Abs(outDir)
			if err != nil {
				return cli.Exit(err.Error(), 2)
			}
			pkg = packageName(filepath.Base(abs))
		}
		opts := golanggen.Options{Package: pkg, Source: filepath.Base(file)}
		models, err := golanggen.Generate(f, info, opts)
		if err != nil {
			return cli.Exit("gen: "+err.Error(), 1)
		}
		queries, err := golanggen.GenerateQueries(f, info, opts)
		if err != nil {
			return cli.Exit("gen: "+err.Error(), 1)
		}
		outputs = []output{
			{"dbml_models.go", models, "// Code generated "},
			{"dbml_queries.go", queries, "// Code generated "},
		}
	case "sqlite":
		code, err := sqlitegen.Generate(f, info, sqlitegen.Options{Source: filepath.Base(file)})
		if err != nil {
			return cli.Exit("gen: "+err.Error(), 1)
		}
		outputs = []output{{"dbml_schema.sql", code, "-- Code generated "}}
	}

	// Refuse every clobber before writing anything: all or nothing.
	for _, o := range outputs {
		target := filepath.Join(outDir, o.name)
		if old, err := os.ReadFile(target); err == nil {
			if !bytes.HasPrefix(old, []byte(o.marker)) {
				return cli.Exit(fmt.Sprintf("gen: refusing to overwrite %s: it lacks the generated-code header", target), 2)
			}
		}
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return cli.Exit(err.Error(), 2)
	}
	for _, o := range outputs {
		target := filepath.Join(outDir, o.name)
		if err := os.WriteFile(target, o.code, 0o644); err != nil {
			return cli.Exit(err.Error(), 2)
		}
		fmt.Println(target)
	}
	return nil
}

// packageName sanitizes a directory name into a Go package name.
func packageName(dir string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(dir) {
		if unicode.IsLetter(r) || (b.Len() > 0 && unicode.IsDigit(r)) {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "models"
	}
	return b.String()
}
