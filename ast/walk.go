package ast

// Inspect traverses the AST in depth-first order, calling f for each node.
// If f returns false, children of the node are not visited. It plays the
// role of go/ast's Inspect for the vet analyzers.
func Inspect(n Node, f func(Node) bool) {
	if n == nil || !f(n) {
		return
	}
	for _, c := range children(n) {
		Inspect(c, f)
	}
}

func children(n Node) []Node {
	var out []Node
	add := func(ns ...Node) {
		for _, c := range ns {
			if c != nil {
				out = append(out, c)
			}
		}
	}
	switch n := n.(type) {
	case *File:
		for _, d := range n.Decls {
			add(d)
		}
	case *Use:
		for _, it := range n.Items {
			add(it)
		}
		add(n.Path)
	case *UseItem:
		add(n.Kind, n.Name, n.Alias)
	case *Project:
		add(identOrNil(n.Name))
		for _, p := range n.Props {
			add(p)
		}
		for _, nt := range n.Notes {
			add(nt)
		}
	case *ProjectProp:
		add(n.Key, n.Value)
	case *Table:
		add(n.Name, identOrNil(n.Alias), settingsOrNil(n.Settings))
		for _, it := range n.Body {
			add(it)
		}
	case *TablePartial:
		add(n.Name, settingsOrNil(n.Settings))
		for _, it := range n.Body {
			add(it)
		}
	case *Column:
		add(n.Name, n.Type, settingsOrNil(n.Settings))
		for _, f := range n.LegacyFlags {
			add(f)
		}
	case *TypeRef:
		add(n.Name)
	case *IndexesBlock:
		for _, ix := range n.Indexes {
			add(ix)
		}
	case *Index:
		add(n.Key...)
		add(settingsOrNil(n.Settings))
	case *ChecksBlock:
		for _, c := range n.Checks {
			add(c)
		}
	case *Check:
		add(n.Expr, settingsOrNil(n.Settings))
	case *PartialRef:
		add(n.Name)
	case *Note:
		add(n.Text)
	case *Ref:
		add(identOrNil(n.Name), n.Left, n.Right, settingsOrNil(n.Settings))
	case *RefEndpoint:
		add(n.Table)
		for _, c := range n.Columns {
			add(c)
		}
	case *Enum:
		add(n.Name)
		for _, v := range n.Values {
			add(v)
		}
	case *EnumValue:
		add(n.Name, settingsOrNil(n.Settings))
	case *Records:
		add(n.Table)
		for _, c := range n.Columns {
			add(c)
		}
		for _, r := range n.Rows {
			add(r)
		}
	case *RecordRow:
		add(n.Values...)
	case *StickyNote:
		add(n.Name, settingsOrNil(n.Settings), n.Text)
	case *TableGroup:
		add(n.Name, settingsOrNil(n.Settings))
		for _, m := range n.Members {
			add(m)
		}
		for _, nt := range n.Notes {
			add(nt)
		}
	case *DiagramView:
		add(n.Name)
		for _, c := range n.Categories {
			add(c)
		}
	case *ViewCategory:
		add(n.Kind)
		for _, nm := range n.Names {
			add(nm)
		}
	case *SettingList:
		for _, s := range n.Settings {
			add(s)
		}
	case *Setting:
		add(n.Value)
	case *QualName:
		for _, p := range n.Parts {
			add(p)
		}
	case *NegNumber:
		add(n.Num)
	case *EnumConst:
		add(n.Enum, n.Value)
	case *RefValue:
		add(n.Endpoint)
	}
	return out
}

// identOrNil avoids the typed-nil-in-interface trap for optional fields.
func identOrNil(x *Ident) Node {
	if x == nil {
		return nil
	}
	return x
}

func settingsOrNil(x *SettingList) Node {
	if x == nil {
		return nil
	}
	return x
}
