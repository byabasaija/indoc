package reference

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/byabasaija/indoc/internal/doc"
)

// cobraCommand is one cobra.Command literal bound to a variable.
type cobraCommand struct {
	variable string // package-qualified: "cli.scanCmd"
	use      string
	short    string
	source   string
	parent   string
	flags    []string
	children []string
}

// extractCobra lists Cobra commands with their full command path, flags,
// and short description, from Go source.
func extractCobra(root string, files []string) ([]doc.RefEntry, *Coverage) {
	cov := &Coverage{Extractor: "cobra"}
	cmds := map[string]*cobraCommand{}
	var order []string
	type addCall struct{ parent, child string }
	var adds []addCall
	type flagCall struct {
		owner string
		flag  string
	}
	var flags []flagCall

	for _, f := range files {
		if !strings.HasSuffix(f, ".go") || strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(f)))
		if err != nil || !strings.Contains(string(src), "cobra") {
			continue
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, f, src, parser.SkipObjectResolution)
		if err != nil {
			continue
		}
		cobraName := ""
		for _, imp := range file.Imports {
			if p, _ := strconv.Unquote(imp.Path.Value); p == "github.com/spf13/cobra" {
				cobraName = "cobra"
				if imp.Name != nil {
					cobraName = imp.Name.Name
				}
			}
		}
		if cobraName == "" {
			continue
		}
		cov.Files++
		pkg := file.Name.Name
		qualify := func(name string) string { return pkg + "." + name }

		ast.Inspect(file, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.ValueSpec:
				for i, name := range x.Names {
					if i < len(x.Values) {
						if c := commandLiteral(x.Values[i], cobraName); c != nil {
							c.variable = qualify(name.Name)
							c.source = fmt.Sprintf("%s:%d", f, fset.Position(x.Pos()).Line)
							cmds[c.variable] = c
							order = append(order, c.variable)
						}
					}
				}
			case *ast.AssignStmt:
				for i, lhs := range x.Lhs {
					id, ok := lhs.(*ast.Ident)
					if !ok || i >= len(x.Rhs) {
						continue
					}
					if c := commandLiteral(x.Rhs[i], cobraName); c != nil {
						c.variable = qualify(id.Name)
						c.source = fmt.Sprintf("%s:%d", f, fset.Position(x.Pos()).Line)
						cmds[c.variable] = c
						order = append(order, c.variable)
					}
				}
			case *ast.CallExpr:
				sel, ok := x.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				switch {
				case sel.Sel.Name == "AddCommand":
					if parent, ok := sel.X.(*ast.Ident); ok {
						for _, arg := range x.Args {
							if child, ok := arg.(*ast.Ident); ok {
								adds = append(adds, addCall{qualify(parent.Name), qualify(child.Name)})
							}
						}
					}
				default:
					if owner, flag, ok := flagDefinition(x); ok {
						flags = append(flags, flagCall{qualify(owner), flag})
					}
				}
			}
			return true
		})
	}
	if len(cmds) == 0 {
		return nil, nil
	}
	cov.Evidence = append(cov.Evidence, "github.com/spf13/cobra")
	for _, a := range adds {
		if p, c := cmds[a.parent], cmds[a.child]; p != nil && c != nil {
			c.parent = a.parent
			p.children = append(p.children, a.child)
		} else {
			cov.Unresolved = append(cov.Unresolved, fmt.Sprintf("%s.AddCommand(%s): command not found", a.parent, a.child))
		}
	}
	for _, fl := range flags {
		if c := cmds[fl.owner]; c != nil {
			c.flags = append(c.flags, fl.flag)
		}
	}

	var out []doc.RefEntry
	for _, v := range order {
		c := cmds[v]
		if c.parent == "" && len(c.children) == 0 && len(cmds) > 1 {
			cov.Unresolved = append(cov.Unresolved, fmt.Sprintf("%s (%s) is not attached to any command", c.variable, c.source))
			continue
		}
		e := doc.RefEntry{Name: commandPath(cmds, c), Detail: strings.Join(c.flags, " "), Description: c.short, Source: c.source}
		if rest := strings.TrimSpace(strings.TrimPrefix(c.use, firstWord(c.use))); rest != "" {
			e.Handler = "args: " + rest
		}
		out = append(out, e)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	cov.Entries = len(out)
	return out, cov
}

func firstWord(s string) string {
	if f := strings.Fields(s); len(f) > 0 {
		return f[0]
	}
	return ""
}

func commandPath(cmds map[string]*cobraCommand, c *cobraCommand) string {
	var parts []string
	for seen := 0; c != nil && seen < 16; seen++ {
		parts = append([]string{firstWord(c.use)}, parts...)
		c = cmds[c.parent]
	}
	return strings.Join(parts, " ")
}

// commandLiteral returns the fields of &cobra.Command{...} or cobra.Command{...}.
func commandLiteral(x ast.Expr, cobraName string) *cobraCommand {
	if u, ok := x.(*ast.UnaryExpr); ok && u.Op == token.AND {
		x = u.X
	}
	lit, ok := x.(*ast.CompositeLit)
	if !ok {
		return nil
	}
	sel, ok := lit.Type.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Command" {
		return nil
	}
	if pkg, ok := sel.X.(*ast.Ident); !ok || pkg.Name != cobraName {
		return nil
	}
	c := &cobraCommand{}
	for _, el := range lit.Elts {
		kv, ok := el.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, _ := kv.Key.(*ast.Ident)
		val := stringValue(kv.Value)
		switch key.Name {
		case "Use":
			c.use = val
		case "Short":
			c.short = val
		}
	}
	return c
}

func stringValue(x ast.Expr) string {
	switch v := x.(type) {
	case *ast.BasicLit:
		if s, err := strconv.Unquote(v.Value); err == nil {
			return s
		}
	case *ast.BinaryExpr:
		return stringValue(v.X) + stringValue(v.Y)
	}
	return ""
}

// flagDefinition recognizes cmd.Flags().StringVarP(&v, "name", "n", ...),
// cmd.PersistentFlags().Bool("name", ...), and their variants.
func flagDefinition(call *ast.CallExpr) (owner, flag string, ok bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return "", "", false
	}
	inner, ok := sel.X.(*ast.CallExpr)
	if !ok {
		return "", "", false
	}
	flagsSel, ok := inner.Fun.(*ast.SelectorExpr)
	if !ok || (flagsSel.Sel.Name != "Flags" && flagsSel.Sel.Name != "PersistentFlags") {
		return "", "", false
	}
	id, ok := flagsSel.X.(*ast.Ident)
	if !ok {
		return "", "", false
	}
	method := sel.Sel.Name
	nameAt := 0
	if strings.Contains(method, "Var") {
		nameAt = 1
	}
	if nameAt >= len(call.Args) {
		return "", "", false
	}
	name := stringValue(call.Args[nameAt])
	if name == "" {
		return "", "", false
	}
	flag = "--" + name
	if strings.HasSuffix(method, "P") && nameAt+1 < len(call.Args) {
		if short := stringValue(call.Args[nameAt+1]); short != "" {
			flag = "-" + short + "/--" + name
		}
	}
	if flagsSel.Sel.Name == "PersistentFlags" {
		flag += " (global)"
	}
	return id.Name, flag, true
}
