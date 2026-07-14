// Command nao ("not an ORM") is the one binary of the project (D41): the
// CLI front end over the edbml/ language packages plus the generators,
// built on urfave/cli:
//
//	nao parse [--json] file.edbml...    syntax only
//	nao check [--json] file.edbml...    syntax + semantic errors
//	nao vet [--json] [--enable a,b] [--werror] file.edbml...
//	                                    syntax + semantics + warnings
//	nao analyzers                       list vet analyzers
//	nao gen go     [-i in.edbml] [-o dir] [-p pkg] [-m]  Go models (+CRUD)
//	nao gen sqlite [-i in.edbml] [-o dir]                SQLite DDL + seeds
//	nao lsp                             language server on stdin/stdout
//
// gen go -m/--models-only emits just nao_models.go (structs/enums), for
// sharing the row types across processes without the CRUD layer.
//
// gen defaults: input ./schema.edbml, output the current directory, Go
// package "main".
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

	"github.com/urfave/cli/v3"

	"github.com/Piechutowski/not-an-orm/edbml/check"
	"github.com/Piechutowski/not-an-orm/edbml/diag"
	golanggen "github.com/Piechutowski/not-an-orm/gen/golang"
	sqlitegen "github.com/Piechutowski/not-an-orm/gen/sqlite"
	"github.com/Piechutowski/not-an-orm/edbml/parser"
	"github.com/Piechutowski/not-an-orm/edbml/lsp"
	"github.com/Piechutowski/not-an-orm/edbml/vet"
)

func main() {
	jsonFlag := &cli.BoolFlag{Name: "json", Usage: "emit diagnostics as JSON"}

	app := &cli.Command{
		EnableShellCompletion: true,
		Name:  "nao",
		Usage: "not an ORM: parse, check, lint, generate from and serve EDBML schema files",
		Commands: []*cli.Command{
			{
				Name:      "parse",
				Usage:     "tokenize and parse; report syntax errors only",
				ArgsUsage: "file.edbml...",
				Flags:     []cli.Flag{jsonFlag},
				Action: func(_ context.Context, c *cli.Command) error {
					return run(c, "parse")
				},
			},
			{
				Name:      "check",
				Usage:     "parse and run semantic analysis (spec §4-§8)",
				ArgsUsage: "file.edbml...",
				Flags:     []cli.Flag{jsonFlag},
				Action: func(_ context.Context, c *cli.Command) error {
					return run(c, "check")
				},
			},
			{
				Name:      "vet",
				Usage:     "check plus warnings for legal-but-suspicious DBML",
				ArgsUsage: "file.edbml...",
				Flags: []cli.Flag{
					jsonFlag,
					&cli.StringFlag{Name: "enable", Usage: "comma-separated analyzer `names` to run (default all; see 'nao analyzers')"},
					&cli.BoolFlag{Name: "werror", Usage: "treat warnings as errors in the exit status"},
				},
				Action: func(_ context.Context, c *cli.Command) error {
					return run(c, "vet")
				},
			},
			{
				Name:  "gen",
				Usage: "generate code from an EDBML file",
				Commands: []*cli.Command{
					{
						Name:      "go",
						Usage:     "generate Go model structs and CRUD (nao_models.go, nao_queries.go)",
						ArgsUsage: "[file.edbml]",
						Flags: []cli.Flag{
							&cli.StringFlag{Name: "input", Aliases: []string{"i"}, Usage: "input `file.edbml` (default: ./schema.edbml)"},
							&cli.StringFlag{Name: "out", Aliases: []string{"o"}, Value: ".", Usage: "output `directory` for the generated .go files"},
							&cli.StringFlag{Name: "package", Aliases: []string{"p"}, Value: "main", Usage: "package `name`"},
							&cli.BoolFlag{Name: "models-only", Aliases: []string{"m"}, Usage: "emit only nao_models.go (structs/enums); skip the CRUD queries"},
						},
						Action: func(_ context.Context, c *cli.Command) error {
							return runGen(c, "go")
						},
					},
					{
						Name:      "sqlite",
						Usage:     "generate SQLite DDL and seed inserts (nao_schema.sql)",
						ArgsUsage: "[file.edbml]",
						Flags: []cli.Flag{
							&cli.StringFlag{Name: "input", Aliases: []string{"i"}, Usage: "input `file.edbml` (default: ./schema.edbml)"},
							&cli.StringFlag{Name: "out", Aliases: []string{"o"}, Value: ".", Usage: "output `directory` for nao_schema.sql"},
						},
						Action: func(_ context.Context, c *cli.Command) error {
							return runGen(c, "sqlite")
						},
					},
				},
			},
			{
				Name:  "lsp",
				Usage: "run the EDBML language server (LSP over stdin/stdout)",
				Action: func(_ context.Context, _ *cli.Command) error {
					return lsp.NewServer().RunStdio()
				},
			},
			{
				Name:  "analyzers",
				Usage: "list vet analyzers (documented in edbml/vet/RULES.md)",
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

// runGen implements 'nao gen <lang>': parse + check one EDBML file,
// generate into --out, refusing to clobber non-generated files.
func runGen(c *cli.Command, lang string) error {
	file, err := genInput(c)
	if err != nil {
		return err
	}
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
		return cli.Exit("gen: input has errors; fix them first (see 'nao check')", 1)
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
		opts := golanggen.Options{Package: c.String("package"), Source: filepath.Base(file)}
		models, err := golanggen.Generate(f, info, opts)
		if err != nil {
			return cli.Exit("gen: "+err.Error(), 1)
		}
		outputs = []output{{"nao_models.go", models, "// Code generated "}}
		if !c.Bool("models-only") {
			queries, err := golanggen.GenerateQueries(f, info, opts)
			if err != nil {
				return cli.Exit("gen: "+err.Error(), 1)
			}
			outputs = append(outputs, output{"nao_queries.go", queries, "// Code generated "})
		}
	case "sqlite":
		code, err := sqlitegen.Generate(f, info, sqlitegen.Options{Source: filepath.Base(file)})
		if err != nil {
			return cli.Exit("gen: "+err.Error(), 1)
		}
		outputs = []output{{"nao_schema.sql", code, "-- Code generated "}}
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

// genInput resolves the EDBML input for 'nao gen': the --input/-i flag if
// set, else a single positional arg, else the ./schema.edbml convention.
// Passing both the flag and a positional arg is an error, not a silent pick.
func genInput(c *cli.Command) (string, error) {
	flagFile := c.String("input")
	switch {
	case flagFile != "" && c.Args().Len() > 0:
		return "", cli.Exit("gen: pass the input via -i/--input or positionally, not both", 2)
	case flagFile != "":
		return flagFile, nil
	case c.Args().Len() == 1:
		return c.Args().First(), nil
	case c.Args().Len() > 1:
		return "", cli.Exit("gen takes exactly one EDBML file", 2)
	default:
		return "schema.edbml", nil
	}
}
