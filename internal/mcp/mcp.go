// Package mcp is a minimal Model Context Protocol server over stdio that
// exposes tools. It speaks both protocol eras: the initialize handshake
// (2024-11-05 through 2025-11-25) and the per-request `_meta` model with
// server/discover (2026-07-28).
//
// Messages are newline-delimited JSON-RPC 2.0. Nothing but protocol messages
// is ever written to the output; use stderr for logs.
package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"sync"
)

// Versions this server speaks, newest first.
var Versions = []string{"2026-07-28", "2025-11-25", "2025-06-18", "2025-03-26", "2024-11-05"}

const (
	modern       = "2026-07-28"
	latestLegacy = "2025-11-25"
	metaVersion  = "io.modelcontextprotocol/protocolVersion"
)

// Tool is one callable tool.
type Tool struct {
	Name        string
	Title       string
	Description string
	InputSchema map[string]any
	ReadOnly    bool
	Idempotent  bool
	// Handler runs the tool. text is shown to the model; structured, if
	// non-nil, is also returned as structuredContent. A tool failure the
	// model can act on returns isError.
	Handler func(args json.RawMessage) (text string, structured any, isError bool)
}

// Server serves tools.
type Server struct {
	Name, Version, Instructions string
	Tools                       []Tool
}

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// Serve reads requests from in and writes responses to out until in closes.
func (s *Server) Serve(in io.Reader, out io.Writer) error {
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 64*1024), 32<<20)
	var mu sync.Mutex
	write := func(r response) {
		r.JSONRPC = "2.0"
		data, err := json.Marshal(r) // escapes newlines inside strings: one line per message
		if err != nil {
			data, _ = json.Marshal(response{JSONRPC: "2.0", ID: r.ID, Error: &rpcError{Code: -32603, Message: err.Error()}})
		}
		mu.Lock()
		out.Write(append(data, '\n'))
		mu.Unlock()
	}
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var req request
		if err := json.Unmarshal(line, &req); err != nil {
			write(response{ID: json.RawMessage("null"), Error: &rpcError{Code: -32700, Message: "parse error"}})
			continue
		}
		if len(req.ID) == 0 {
			continue // notification: initialized, cancelled, ...
		}
		result, rerr := s.handle(req)
		if rerr != nil {
			write(response{ID: req.ID, Error: rerr})
		} else {
			write(response{ID: req.ID, Result: result})
		}
	}
	return sc.Err()
}

func (s *Server) handle(req request) (result any, rerr *rpcError) {
	defer func() {
		if r := recover(); r != nil {
			result, rerr = nil, &rpcError{Code: -32603, Message: fmt.Sprint("internal error: ", r)}
		}
	}()

	// Modern requests carry the protocol version in params._meta.
	var p struct {
		Meta map[string]any `json:"_meta"`
	}
	json.Unmarshal(req.Params, &p)
	version, isModern := p.Meta[metaVersion].(string)
	if isModern && !supported(version) {
		return nil, &rpcError{Code: -32022, Message: "Unsupported protocol version",
			Data: map[string]any{"supported": Versions, "requested": version}}
	}
	complete := func(m map[string]any) map[string]any {
		if isModern {
			m["resultType"] = "complete"
		}
		return m
	}

	switch req.Method {
	case "initialize":
		var params struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		json.Unmarshal(req.Params, &params)
		negotiated := latestLegacy
		if supported(params.ProtocolVersion) && params.ProtocolVersion != modern {
			negotiated = params.ProtocolVersion
		}
		return map[string]any{
			"protocolVersion": negotiated,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": s.Name, "version": s.Version},
			"instructions":    s.Instructions,
		}, nil
	case "server/discover":
		return map[string]any{
			"resultType":        "complete",
			"supportedVersions": Versions,
			"capabilities":      map[string]any{"tools": map[string]any{}},
			"instructions":      s.Instructions,
			"_meta": map[string]any{
				"io.modelcontextprotocol/serverInfo": map[string]any{"name": s.Name, "version": s.Version},
			},
		}, nil
	case "ping":
		return complete(map[string]any{}), nil
	case "tools/list":
		tools := make([]map[string]any, len(s.Tools))
		for i, t := range s.Tools {
			tools[i] = map[string]any{
				"name":        t.Name,
				"title":       t.Title,
				"description": t.Description,
				"inputSchema": t.InputSchema,
				"annotations": map[string]any{
					"title":           t.Title,
					"readOnlyHint":    t.ReadOnly,
					"destructiveHint": false,
					"idempotentHint":  t.Idempotent,
					"openWorldHint":   false,
				},
			}
		}
		return complete(map[string]any{"tools": tools}), nil
	case "tools/call":
		var params struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return nil, &rpcError{Code: -32602, Message: "invalid params"}
		}
		for _, t := range s.Tools {
			if t.Name != params.Name {
				continue
			}
			args := params.Arguments
			if len(args) == 0 || string(args) == "null" {
				args = json.RawMessage("{}")
			}
			text, structured, isError := t.Handler(args)
			res := map[string]any{
				"content": []map[string]any{{"type": "text", "text": text}},
				"isError": isError,
			}
			if structured != nil {
				res["structuredContent"] = structured
			}
			return complete(res), nil
		}
		return nil, &rpcError{Code: -32602, Message: "Unknown tool: " + params.Name}
	}
	return nil, &rpcError{Code: -32601, Message: "Method not found: " + req.Method}
}

func supported(v string) bool {
	for _, s := range Versions {
		if s == v {
			return true
		}
	}
	return false
}
