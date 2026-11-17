// Package treesitter parses source code with tree-sitter grammars compiled to
// WebAssembly and run by wazero, a pure-Go runtime. indoc stays a single
// static binary: no cgo, no external tools.
//
// The gzipped .wasm files in wasm/ are built from pinned sources by
// `make grammars` (see build/). Each holds the tree-sitter runtime and one
// grammar.
package treesitter

import (
	"compress/gzip"
	"context"
	"embed"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

//go:embed wasm/*.wasm.gz
var grammars embed.FS

// Languages lists the embedded grammars.
var Languages = []string{"javascript", "typescript", "tsx", "python"}

// Node is one syntax tree node. Children are in source order.
type Node struct {
	Type     string // grammar symbol: "function_declaration", "(", "identifier"
	Field    string // field name in the parent: "name", "body", or ""
	Named    bool   // named node (anonymous nodes are punctuation and keywords)
	Extra    bool   // comments and other extras
	Error    bool   // ERROR node from error recovery
	Missing  bool   // inserted by error recovery
	Start    int    // byte offsets
	End      int
	StartRow int // 0-based
	EndRow   int
	Children []*Node
	Parent   *Node
}

// Child returns the first child with the given field name, or nil.
func (n *Node) Child(field string) *Node {
	for _, c := range n.Children {
		if c.Field == field {
			return c
		}
	}
	return nil
}

// Text returns the node's source text.
func (n *Node) Text(src []byte) string { return string(src[n.Start:n.End]) }

// Walk visits n and its descendants in pre-order; returning false skips children.
func (n *Node) Walk(visit func(*Node) bool) {
	if visit(n) {
		for _, c := range n.Children {
			c.Walk(visit)
		}
	}
}

var (
	runtimeOnce sync.Once
	rt          wazero.Runtime
	rtErr       error

	mu      sync.Mutex
	loaded  = map[string]*language{}
	runtime = context.Background()
)

// language is a compiled grammar with a pool of instances; an instance is
// single-threaded.
type language struct {
	compiled wazero.CompiledModule
	symbols  []string
	fields   []string
	pool     sync.Pool
}

type instance struct {
	mod                             api.Module
	alloc, dealloc, parse, outWords api.Function
}

func engine() (wazero.Runtime, error) {
	runtimeOnce.Do(func() {
		cfg := wazero.NewRuntimeConfig()
		if dir, err := os.UserCacheDir(); err == nil {
			// Compiled machine code is cached across runs; the first run
			// compiles, later runs load in milliseconds.
			if cache, err := wazero.NewCompilationCacheWithDir(filepath.Join(dir, "indoc", "wazero")); err == nil {
				cfg = cfg.WithCompilationCache(cache)
			}
		}
		rt = wazero.NewRuntimeWithConfig(runtime, cfg)
		_, rtErr = wasi_snapshot_preview1.Instantiate(runtime, rt)
	})
	return rt, rtErr
}

func load(name string) (*language, error) {
	mu.Lock()
	defer mu.Unlock()
	if l, ok := loaded[name]; ok {
		return l, nil
	}
	r, err := engine()
	if err != nil {
		return nil, err
	}
	gz, err := grammars.Open("wasm/" + name + ".wasm.gz")
	if err != nil {
		return nil, fmt.Errorf("no tree-sitter grammar for %s", name)
	}
	defer gz.Close()
	zr, err := gzip.NewReader(gz)
	if err != nil {
		return nil, fmt.Errorf("read %s grammar: %w", name, err)
	}
	bin, err := io.ReadAll(zr)
	if err != nil {
		return nil, fmt.Errorf("read %s grammar: %w", name, err)
	}
	compiled, err := r.CompileModule(runtime, bin)
	if err != nil {
		return nil, fmt.Errorf("compile %s grammar: %w", name, err)
	}
	l := &language{compiled: compiled}
	inst, err := l.instantiate()
	if err != nil {
		return nil, err
	}
	if l.symbols, err = inst.names("symbol_count", "symbol_name"); err != nil {
		return nil, err
	}
	if l.fields, err = inst.names("field_count", "field_name"); err != nil {
		return nil, err
	}
	l.pool.Put(inst)
	loaded[name] = l
	return l, nil
}

func (l *language) instantiate() (*instance, error) {
	r, _ := engine()
	mod, err := r.InstantiateModule(runtime, l.compiled, wazero.NewModuleConfig().WithName(""))
	if err != nil {
		return nil, err
	}
	if init := mod.ExportedFunction("_initialize"); init != nil {
		if _, err := init.Call(runtime); err != nil {
			return nil, err
		}
	}
	inst := &instance{
		mod:      mod,
		alloc:    mod.ExportedFunction("alloc"),
		dealloc:  mod.ExportedFunction("dealloc"),
		parse:    mod.ExportedFunction("parse"),
		outWords: mod.ExportedFunction("out_words"),
	}
	if inst.alloc == nil || inst.parse == nil || inst.outWords == nil || inst.dealloc == nil {
		return nil, errors.New("grammar module is missing indoc exports")
	}
	return inst, nil
}

// names reads a symbol or field name table.
func (in *instance) names(count, name string) ([]string, error) {
	res, err := in.mod.ExportedFunction(count).Call(runtime)
	if err != nil {
		return nil, err
	}
	n := int(res[0])
	out := make([]string, n+1)
	fn := in.mod.ExportedFunction(name)
	mem := in.mod.Memory()
	for i := 0; i <= n; i++ {
		r, err := fn.Call(runtime, uint64(i))
		if err != nil {
			return nil, err
		}
		out[i] = cString(mem, uint32(r[0]))
	}
	return out, nil
}

func cString(mem api.Memory, ptr uint32) string {
	if ptr == 0 {
		return ""
	}
	var b []byte
	for {
		c, ok := mem.ReadByte(ptr)
		if !ok || c == 0 {
			return string(b)
		}
		b = append(b, c)
		ptr++
	}
}

// Parse parses src with the named grammar. Syntax errors do not fail: the
// tree contains ERROR or missing nodes where recovery happened.
func Parse(lang string, src []byte) (*Node, error) {
	l, err := load(lang)
	if err != nil {
		return nil, err
	}
	inst, _ := l.pool.Get().(*instance)
	if inst == nil {
		if inst, err = l.instantiate(); err != nil {
			return nil, err
		}
	}
	root, err := inst.run(l, src)
	if err != nil {
		inst.mod.Close(runtime) // a failed instance is not reused
		return nil, err
	}
	l.pool.Put(inst)
	return root, nil
}

const wordsPerNode = 7

func (in *instance) run(l *language, src []byte) (*Node, error) {
	res, err := in.alloc.Call(runtime, uint64(len(src)+1))
	if err != nil || res[0] == 0 {
		return nil, fmt.Errorf("allocate %d bytes: %v", len(src), err)
	}
	ptr := uint32(res[0])
	defer in.dealloc.Call(runtime, uint64(ptr))
	mem := in.mod.Memory()
	if !mem.Write(ptr, src) {
		return nil, errors.New("write source")
	}
	res, err = in.parse.Call(runtime, uint64(ptr), uint64(len(src)))
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	outPtr := uint32(res[0])
	if outPtr == 0 {
		return nil, errors.New("parse failed: out of memory")
	}
	res, err = in.outWords.Call(runtime)
	if err != nil {
		return nil, err
	}
	words := uint32(res[0])
	raw, ok := mem.Read(outPtr, words*4)
	if !ok {
		return nil, errors.New("read tree")
	}
	return decode(l, raw)
}

// decode rebuilds the tree from the pre-order dump.
func decode(l *language, raw []byte) (*Node, error) {
	n := len(raw) / 4 / wordsPerNode
	if n == 0 {
		return nil, errors.New("empty tree")
	}
	nodes := make([]Node, n)
	word := func(i, k int) uint32 { return binary.LittleEndian.Uint32(raw[(i*wordsPerNode+k)*4:]) }
	type frame struct {
		node      *Node
		remaining uint32
	}
	stack := make([]frame, 0, 64)
	for i := 0; i < n; i++ {
		head, flags := word(i, 0), word(i, 1)
		sym, field := int(head>>16), int(head&0xffff)
		nd := &nodes[i]
		switch {
		case sym == 0xFFFF: // ts_builtin_sym_error
			nd.Type = "ERROR"
		case sym < len(l.symbols):
			nd.Type = l.symbols[sym]
		}
		if field < len(l.fields) {
			nd.Field = l.fields[field]
		}
		nd.Named, nd.Extra, nd.Error, nd.Missing = flags&1 != 0, flags&2 != 0, flags&4 != 0, flags&8 != 0
		nd.Start, nd.End = int(word(i, 2)), int(word(i, 3))
		nd.StartRow, nd.EndRow = int(word(i, 4)), int(word(i, 5))
		children := word(i, 6)
		if len(stack) > 0 {
			top := &stack[len(stack)-1]
			nd.Parent = top.node
			top.node.Children = append(top.node.Children, nd)
			top.remaining--
		}
		for len(stack) > 0 && stack[len(stack)-1].remaining == 0 {
			stack = stack[:len(stack)-1]
		}
		if children > 0 {
			nd.Children = make([]*Node, 0, children)
			stack = append(stack, frame{nd, children})
		}
	}
	return &nodes[0], nil
}
