// Package golang extracts symbols from Go source using the standard library
// parser.
package golang

import (
	"strings"

	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"

	"github.com/byabasaija/indoc/internal/lang"
)

// Parse returns functions, methods, and type declarations in src. Methods are
// qualified by receiver type: "Scanner.Scan". Source that does not parse
// yields whatever declarations the parser recovered.
func Parse(src []byte) []lang.Symbol {
	fset := token.NewFileSet()
	file, _ := parser.ParseFile(fset, "", src, parser.SkipObjectResolution)
	if file == nil {
		return nil
	}
	off := func(p token.Pos) int { return fset.Position(p).Offset }
	line := func(p token.Pos) int { return fset.Position(p).Line }

	var syms []lang.Symbol
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			kind, q := lang.KindFunction, d.Name.Name
			if d.Recv != nil && len(d.Recv.List) > 0 {
				kind = lang.KindMethod
				if recv := receiverName(d.Recv.List[0].Type); recv != "" {
					q = recv + "." + d.Name.Name
				}
			}
			sigEnd := d.End()
			body := ""
			if d.Body != nil {
				sigEnd = d.Body.Lbrace
				body = lang.Hash(tokens(src[off(d.Body.Lbrace):off(d.Body.End())]))
			}
			syms = append(syms, lang.Symbol{
				Name:          d.Name.Name,
				Qual:          q,
				Kind:          kind,
				Line:          line(d.Pos()),
				DecoratorLine: line(d.Pos()),
				EndLine:       line(d.End()),
				Surface:       lang.Hash(tokens(src[off(d.Pos()):off(sigEnd)])),
				Body:          body,
				Private:       !exported(q),
			})
		case *ast.GenDecl:
			if d.Tok != token.TYPE {
				continue
			}
			for _, spec := range d.Specs {
				ts := spec.(*ast.TypeSpec)
				syms = append(syms, lang.Symbol{
					Name:          ts.Name.Name,
					Qual:          ts.Name.Name,
					Kind:          lang.KindType,
					Line:          line(ts.Pos()),
					DecoratorLine: line(ts.Pos()),
					EndLine:       line(ts.End()),
					// A type's definition (fields, methods of an interface) is its surface.
					Surface: lang.Hash(tokens(src[off(ts.Pos()):off(ts.End())])),
					Private: !ast.IsExported(ts.Name.Name),
				})
			}
		}
	}
	return syms
}

func receiverName(expr ast.Expr) string {
	for {
		switch e := expr.(type) {
		case *ast.StarExpr:
			expr = e.X
		case *ast.IndexExpr:
			expr = e.X
		case *ast.IndexListExpr:
			expr = e.X
		case *ast.ParenExpr:
			expr = e.X
		case *ast.Ident:
			return e.Name
		default:
			return ""
		}
	}
}

// tokens returns significant token texts in src. Comments and automatically
// inserted semicolons are dropped, so layout changes do not affect hashes.
func tokens(src []byte) []string {
	fset := token.NewFileSet()
	file := fset.AddFile("", -1, len(src))
	var s scanner.Scanner
	s.Init(file, src, nil, 0)
	var out []string
	for {
		_, tok, lit := s.Scan()
		if tok == token.EOF {
			return out
		}
		if tok == token.SEMICOLON && lit == "\n" {
			continue
		}
		if lit != "" {
			out = append(out, lit)
		} else {
			out = append(out, tok.String())
		}
	}
}

// FileHash hashes the whole file's significant tokens.
func FileHash(src []byte) string {
	return lang.Hash(tokens(src))
}

// exported reports whether every segment of Recv.Method is exported.
func exported(qual string) bool {
	for _, seg := range strings.Split(qual, ".") {
		if !ast.IsExported(seg) {
			return false
		}
	}
	return true
}
