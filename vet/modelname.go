// Model-name analyzer — decision D10: Go models are the singular of the
// table name, derived by the deterministic inflector in the inflect
// package. When the inflector is only guessing, the schema should say the
// name out loud with the [model:] extension setting.
package vet

import "github.com/Piechutowski/not-an-orm/inflect"

func init() { register(modelName) }

var modelName = &Analyzer{
	Name: "modelname",
	Doc:  "reports tables whose generated Go model name is an inflector guess not pinned by [model:]",
	Run: func(p *Pass) {
		for _, ti := range p.Info.Tables {
			t := ti.Decl
			if t.Settings.Get("model") != nil {
				continue
			}
			singular, ok := inflect.SingularLast(t.Name.Base())
			if ok {
				continue
			}
			p.Reportf(t.Pos(), "cannot confidently singularize %q for its Go model name (would use %q); pin it with [model: '...']",
				t.Name.Base(), singular)
		}
	},
}
