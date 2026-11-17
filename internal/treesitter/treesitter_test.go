package treesitter

import (
	"strings"
	"testing"
)

func sexp(n *Node, src []byte) string {
	var b strings.Builder
	var walk func(*Node)
	walk = func(n *Node) {
		if !n.Named {
			return
		}
		b.WriteString("(")
		if n.Field != "" {
			b.WriteString(n.Field + ": ")
		}
		b.WriteString(n.Type)
		for _, c := range n.Children {
			if c.Named {
				b.WriteString(" ")
				walk(c)
			}
		}
		b.WriteString(")")
	}
	walk(n)
	return b.String()
}

func TestParseTypeScript(t *testing.T) {
	src := []byte("// greet\nexport function greet(name: string): string {\n  return `hi ${name}`;\n}\n")
	root, err := Parse("typescript", src)
	if err != nil {
		t.Fatal(err)
	}
	got := sexp(root, src)
	for _, want := range []string{"(program (comment)", "(export_statement (declaration: function_declaration (name: identifier)", "(body: statement_block"} {
		if !strings.Contains(got, want) {
			t.Errorf("tree missing %q:\n%s", want, got)
		}
	}
	fn := root.Children[1].Child("declaration")
	if fn == nil || fn.Child("name").Text(src) != "greet" || fn.StartRow != 1 || fn.EndRow != 3 {
		t.Fatalf("function node wrong: %+v", fn)
	}
	if !root.Children[0].Extra {
		t.Error("comment should be marked extra")
	}
}

func TestEveryGrammarLoads(t *testing.T) {
	samples := map[string]string{
		"javascript": "const App = () => <div>{x}</div>;\n",
		"typescript": "interface A<T> { x: T }\n",
		"tsx":        "export const B = (p: P) => <A x={1} />;\n",
		"python":     "def f(x):\n    return f\"{x!r}\"\n",
	}
	for _, lang := range Languages {
		root, err := Parse(lang, []byte(samples[lang]))
		if err != nil {
			t.Errorf("%s: %v", lang, err)
			continue
		}
		var errs int
		root.Walk(func(n *Node) bool {
			if n.Error || n.Missing {
				errs++
			}
			return true
		})
		if errs > 0 {
			t.Errorf("%s: %d error nodes in %s", lang, errs, sexp(root, []byte(samples[lang])))
		}
	}
}

func TestSyntaxErrorsRecover(t *testing.T) {
	root, err := Parse("typescript", []byte("function f( {\nconst x = ;\n"))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	root.Walk(func(n *Node) bool {
		found = found || n.Error || n.Missing
		return true
	})
	if !found {
		t.Error("expected error recovery nodes")
	}
}

func TestConcurrentParses(t *testing.T) {
	done := make(chan error, 16)
	for i := 0; i < 16; i++ {
		go func() {
			_, err := Parse("javascript", []byte("function a() { return 1 }\n"))
			done <- err
		}()
	}
	for i := 0; i < 16; i++ {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
}
