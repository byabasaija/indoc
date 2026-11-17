package golang

import (
	"testing"

	"github.com/byabasaija/indoc/internal/lang"
)

const base = `package x

// Scanner walks files.
type Scanner[T any] struct {
	dirs []string
}

func New(dirs []string) *Scanner[string] {
	return &Scanner[string]{dirs: dirs}
}

func (s *Scanner[T]) Scan() error {
	return nil
}
`

func find(t *testing.T, src, qual string) lang.Symbol {
	t.Helper()
	for _, s := range Parse([]byte(src)) {
		if s.Qual == qual {
			return s
		}
	}
	t.Fatalf("symbol %q not found", qual)
	return lang.Symbol{}
}

func TestParse(t *testing.T) {
	syms := Parse([]byte(base))
	want := []struct {
		qual, kind string
		line, end  int
	}{
		{"Scanner", lang.KindType, 4, 6},
		{"New", lang.KindFunction, 8, 10},
		{"Scanner.Scan", lang.KindMethod, 12, 14},
	}
	if len(syms) != len(want) {
		t.Fatalf("got %d symbols: %+v", len(syms), syms)
	}
	for i, w := range want {
		s := syms[i]
		if s.Qual != w.qual || s.Kind != w.kind || s.Line != w.line || s.EndLine != w.end {
			t.Errorf("symbol %d = %s %s %d-%d, want %s %s %d-%d", i, s.Qual, s.Kind, s.Line, s.EndLine, w.qual, w.kind, w.line, w.end)
		}
	}
}

func TestHashes(t *testing.T) {
	a := find(t, base, "Scanner.Scan")
	reformatted := find(t, `package x
func (s *Scanner[T]) Scan() error { // comment
	return nil
}`, "Scanner.Scan")
	body := find(t, `package x
func (s *Scanner[T]) Scan() error {
	return errNope
}`, "Scanner.Scan")
	sig := find(t, `package x
func (s *Scanner[T]) Scan(deep bool) error {
	return nil
}`, "Scanner.Scan")

	if reformatted.Surface != a.Surface || reformatted.Body != a.Body {
		t.Error("formatting and comments must not change hashes")
	}
	if body.Surface != a.Surface || body.Body == a.Body {
		t.Error("body edit should change only the body hash")
	}
	if sig.Surface == a.Surface || sig.Body != a.Body {
		t.Error("signature edit should change only the surface hash")
	}
}
