package jsts

import (
	"strings"
	"testing"

	"github.com/byabasaija/indoc/internal/lang"
)

const fixture = `import express from "express";

/** Payment gateway. */
@Injectable()
export class Gateway {
  private secret = "x";
  constructor(private http: Http) {}

  @Get(":id")
  async charge(id: string): Promise<Receipt> {
    console.log("charging", id);
    return this.http.post(id);
  }

  private sign(body: string) {
    return body;
  }

  #nonce() { return 1 }

  onTimeout = () => this.retry();
}

export interface Receipt { id: string }
type Internal = { n: number };
export enum Status { Pending, Paid }

export const handler = async (req, res) => {
  function helper() {}
  res.send({ ok: true, run() { return 1 } });
};

function notExported() {}
export { notExported as renamedExport };

const config = { load() {} };
export default function main() {}
`

func TestParseSymbols(t *testing.T) {
	f, err := Parse("app/gateway.ts", []byte(fixture))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, s := range f.Symbols {
		priv := ""
		if s.Private {
			priv = " private"
		}
		got = append(got, s.Kind+" "+s.Qual+priv)
	}
	want := []string{
		"class Gateway",
		"method Gateway.constructor",
		"method Gateway.charge",
		"method Gateway.sign private",
		"method Gateway.#nonce private",
		"method Gateway.onTimeout",
		"type Receipt",
		"type Internal private",
		"type Status",
		"function handler",
		"function handler.helper private",
		"function notExported",
		"function main",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("symbols:\n%s\n\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	g := symbol(t, f.Symbols, "Gateway")
	if g.DecoratorLine != 4 || g.EndLine != 22 {
		t.Errorf("Gateway lines %d-%d, want decorator line 4 to 22", g.DecoratorLine, g.EndLine)
	}
	if c := symbol(t, f.Symbols, "Gateway.charge"); c.DecoratorLine != 9 {
		t.Errorf("charge starts at %d, want its decorator at 9", c.DecoratorLine)
	}
}

func symbol(t *testing.T, syms []lang.Symbol, qual string) lang.Symbol {
	t.Helper()
	for _, s := range syms {
		if s.Qual == qual {
			return s
		}
	}
	t.Fatalf("no symbol %q", qual)
	return lang.Symbol{}
}

func parseOne(t *testing.T, src, qual string) lang.Symbol {
	t.Helper()
	f, err := Parse("x.ts", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	return symbol(t, f.Symbols, qual)
}

func TestHashes(t *testing.T) {
	base := parseOne(t, "export function add(a: number, b: number): number {\n  return a + b;\n}\n", "add")
	formatted := parseOne(t, "// sum\nexport function add(\n  a: number,\n  b: number,\n): number {\n  console.log(a)\n  return a + b\n}\n", "add")
	body := parseOne(t, "export function add(a: number, b: number): number {\n  return a - b;\n}\n", "add")
	sig := parseOne(t, "export function add(a: number, b: string): number {\n  return a + b;\n}\n", "add")
	if formatted.Surface != base.Surface || formatted.Body != base.Body {
		t.Error("comments, logging, semicolons, and trailing commas changed hashes")
	}
	if body.Surface != base.Surface || body.Body == base.Body {
		t.Error("body edit should change only the body hash")
	}
	if sig.Surface == base.Surface || sig.Body != base.Body {
		t.Error("signature edit should change only the surface hash")
	}

	cls := "export class A {\n  run(x: number) {\n    return x;\n  }\n}\n"
	a := parseOne(t, cls, "A")
	methodBody := parseOne(t, strings.Replace(cls, "return x;", "return x * 2;", 1), "A")
	methodSig := parseOne(t, strings.Replace(cls, "run(x: number)", "run(x: string)", 1), "A")
	if a.Body != methodBody.Body {
		t.Error("a class's body hash must not see method bodies")
	}
	if a.Body == methodSig.Body {
		t.Error("a class's body hash must see member signatures")
	}
}

func TestJavaScriptCommonJSExports(t *testing.T) {
	f, err := Parse("lib.js", []byte("function a() {}\nfunction b() {}\nconst c = () => 1;\nmodule.exports = { a, renamed: c };\n"))
	if err != nil {
		t.Fatal(err)
	}
	priv := map[string]bool{}
	for _, s := range f.Symbols {
		priv[s.Qual] = s.Private
	}
	if priv["a"] || !priv["b"] || priv["c"] {
		t.Errorf("CommonJS exports: %v", priv)
	}
}
