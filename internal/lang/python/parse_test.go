package python

import (
	"testing"

	"github.com/byabasaija/indoc/internal/lang"
)

func find(t *testing.T, syms []lang.Symbol, qual string) lang.Symbol {
	t.Helper()
	for _, s := range syms {
		if s.Qual == qual {
			return s
		}
	}
	t.Fatalf("symbol %q not found in %+v", qual, syms)
	return lang.Symbol{}
}

func TestParseStructure(t *testing.T) {
	src := `import os

@decorator(
    arg=1,
)
class Dispatcher(Base):
    """Routes sessions."""

    def dispatch(self, session):
        if session.cardpay:
            def inner():
                return 1
            return inner()
        return None  # trailing comment

    # comment between methods
    async def close(self): pass

if TYPE_CHECKING:
    def typed() -> int:
        ...
`
	syms := Parse(src)
	want := []struct {
		qual, kind      string
		line, deco, end int
	}{
		{"Dispatcher", lang.KindClass, 6, 3, 17},
		{"Dispatcher.dispatch", lang.KindMethod, 9, 9, 14},
		{"Dispatcher.dispatch.inner", lang.KindFunction, 11, 11, 12},
		{"Dispatcher.close", lang.KindMethod, 17, 17, 17},
		{"typed", lang.KindFunction, 20, 20, 21},
	}
	if len(syms) != len(want) {
		t.Fatalf("got %d symbols, want %d: %+v", len(syms), len(want), syms)
	}
	for i, w := range want {
		s := syms[i]
		if s.Qual != w.qual || s.Kind != w.kind || s.Line != w.line || s.DecoratorLine != w.deco || s.EndLine != w.end {
			t.Errorf("symbol %d = %s %s %d/%d-%d, want %s %s %d/%d-%d",
				i, s.Qual, s.Kind, s.DecoratorLine, s.Line, s.EndLine, w.qual, w.kind, w.deco, w.line, w.end)
		}
	}
}

// Regression: a newline inside brackets must not trigger indentation
// processing when the bracket closes mid-line (`):`).
func TestParseMultilineHeaderInBlock(t *testing.T) {
	src := `def accept(prefix):
    if prefix in (
        # comment with a ' quote
        b"mif1",
    ):
        return True
    return False
`
	if s := find(t, Parse(src), "accept"); s.EndLine != 7 {
		t.Fatalf("accept ends at %d, want 7", s.EndLine)
	}
}

// Regression: a backslash does not escape braces in f-strings, so `\{{` is a
// backslash followed by an escaped brace, not the start of a field.
func TestParseFStringBackslashBrace(t *testing.T) {
	src := `class Lexer:
    tokens = [rf'(\{{%)(-?\s*)({ident})', f"\N{BULLET} {x!r:>{width}}"]

    def after(self):
        return 1
`
	if s := find(t, Parse(src), "Lexer.after"); s.Line != 4 {
		t.Fatalf("Lexer.after at line %d, want 4", s.Line)
	}
}

func TestParseFStringNestedQuotes(t *testing.T) { // PEP 701
	src := "def f(d):\n    return f\"{d[\"key\"]} {f'{d[\"a\"]}'}\"\n\ndef g():\n    pass\n"
	if s := find(t, Parse(src), "g"); s.Line != 4 {
		t.Fatalf("g at line %d, want 4", s.Line)
	}
}

func TestHashesIgnoreFormattingAndComments(t *testing.T) {
	a := find(t, Parse("def f(a, b):\n    return a + b\n"), "f")
	b := find(t, Parse("def f(a,\n      b):  # sum\n    # add them\n    return a+b\n"), "f")
	if a.Surface != b.Surface || a.Body != b.Body {
		t.Fatalf("formatting changed hashes: %+v vs %+v", a, b)
	}
}

func TestHashesSeparateSurfaceFromBody(t *testing.T) {
	base := find(t, Parse("@cached\ndef f(a):\n    return a\n"), "f")
	body := find(t, Parse("@cached\ndef f(a):\n    return a * 2\n"), "f")
	sig := find(t, Parse("@cached\ndef f(a, b=1):\n    return a\n"), "f")
	deco := find(t, Parse("def f(a):\n    return a\n"), "f")

	if body.Surface != base.Surface || body.Body == base.Body {
		t.Error("body edit should change only the body hash")
	}
	if sig.Surface == base.Surface || sig.Body != base.Body {
		t.Error("signature edit should change only the surface hash")
	}
	if deco.Surface == base.Surface {
		t.Error("removing a decorator should change the surface hash")
	}
}

func TestHashesSeeBlockStructure(t *testing.T) {
	a := find(t, Parse("def f(x):\n    if x:\n        a()\n        b()\n"), "f")
	b := find(t, Parse("def f(x):\n    if x:\n        a()\n    b()\n"), "f")
	if a.Body == b.Body {
		t.Fatal("moving a statement out of a block must change the body hash")
	}
}

func TestParseNeverPanicsOnMalformedInput(t *testing.T) {
	for _, src := range []string{
		"def", "def f(", "class :", "@dec\n", "def f():\n  x\n    y\n z\n",
		"s = '''unterminated", "f'{", "x = (\n", "\tdef f(): pass\n",
	} {
		Parse(src)
	}
}
