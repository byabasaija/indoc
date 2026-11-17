// Package lang defines the language-neutral symbol model used for staleness
// detection and citations. Each language package produces []Symbol from source.
package lang

import (
	"crypto/sha256"
	"encoding/hex"
)

// Symbol kinds.
const (
	KindFunction = "function"
	KindMethod   = "method"
	KindClass    = "class"
	KindType     = "type"
)

// Symbol is one named definition in a source file.
//
// Surface hashes what callers and readers depend on: decorators, name,
// parameters, return annotation, bases. Body hashes the implementation.
// Both are computed over tokens with comments and layout removed, so
// formatting-only edits change neither.
type Symbol struct {
	Name          string `json:"name"`
	Qual          string `json:"qual"` // dotted path within the file, e.g. "Dispatcher.dispatch"
	Kind          string `json:"kind"`
	Line          int    `json:"line"`           // line of the def/class/func keyword
	DecoratorLine int    `json:"decorator_line"` // first decorator line, or Line when undecorated
	EndLine       int    `json:"end_line"`
	Surface       string `json:"surface"`
	Body          string `json:"body"`
	// Private marks symbols outside the documented interface, by the
	// language's rule (unexported in JS/TS and Go, _name in Python).
	Private bool `json:"private,omitempty"`
}

// Hash returns a short stable digest of a token sequence.
func Hash(tokens []string) string {
	h := sha256.New()
	for _, t := range tokens {
		h.Write([]byte(t))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}
