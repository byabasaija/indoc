package mcp

import (
	"bufio"
	"encoding/json"
	"strings"
	"testing"
)

func server() *Server {
	return &Server{
		Name: "indoc", Version: "test", Instructions: "Use the tools.",
		Tools: []Tool{{
			Name: "echo", Title: "Echo", Description: "Echoes text.", ReadOnly: true,
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{"text": map[string]any{"type": "string"}}},
			Handler: func(args json.RawMessage) (string, any, bool) {
				var a struct{ Text string }
				json.Unmarshal(args, &a)
				if a.Text == "fail" {
					return "it failed\nwith detail", nil, true
				}
				return "said: " + a.Text, map[string]any{"text": a.Text}, false
			},
		}},
	}
}

// exchange sends request lines and returns decoded responses by id.
func exchange(t *testing.T, lines ...string) map[string]map[string]any {
	t.Helper()
	var out strings.Builder
	if err := server().Serve(strings.NewReader(strings.Join(lines, "\n")+"\n"), &out); err != nil {
		t.Fatal(err)
	}
	got := map[string]map[string]any{}
	sc := bufio.NewScanner(strings.NewReader(out.String()))
	for sc.Scan() {
		var m map[string]any
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			t.Fatalf("output line is not JSON: %q", sc.Text())
		}
		got[string(mustJSON(m["id"]))] = m
	}
	return got
}

func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }

func TestLegacyHandshake(t *testing.T) {
	got := exchange(t,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"c","version":"1"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"echo","arguments":{"text":"hi"}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"echo","arguments":{"text":"fail"}}}`,
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"nope"}}`,
		`{"jsonrpc":"2.0","id":6,"method":"ping"}`,
		`{"jsonrpc":"2.0","id":7,"method":"resources/list"}`,
		`{"jsonrpc":"2.0","id":8,"method":"initialize","params":{"protocolVersion":"1999-01-01"}}`,
	)
	if len(got) != 8 {
		t.Fatalf("want 8 responses (notification gets none), got %d", len(got))
	}
	init := got["1"]["result"].(map[string]any)
	if init["protocolVersion"] != "2025-06-18" || init["instructions"] != "Use the tools." {
		t.Errorf("initialize = %v", init)
	}
	if got["8"]["result"].(map[string]any)["protocolVersion"] != "2025-11-25" {
		t.Error("unknown version should negotiate to the latest handshake version")
	}
	tools := got["2"]["result"].(map[string]any)["tools"].([]any)
	if tools[0].(map[string]any)["annotations"].(map[string]any)["readOnlyHint"] != true {
		t.Errorf("tools/list = %v", tools)
	}
	call := got["3"]["result"].(map[string]any)
	if call["isError"] != false || call["content"].([]any)[0].(map[string]any)["text"] != "said: hi" ||
		call["structuredContent"].(map[string]any)["text"] != "hi" {
		t.Errorf("tools/call = %v", call)
	}
	if _, modern := call["resultType"]; modern {
		t.Error("legacy results must not carry resultType")
	}
	if got["4"]["result"].(map[string]any)["isError"] != true {
		t.Error("tool failure must be a result with isError")
	}
	if got["5"]["error"].(map[string]any)["code"].(float64) != -32602 {
		t.Error("unknown tool must be -32602")
	}
	if got["7"]["error"].(map[string]any)["code"].(float64) != -32601 {
		t.Error("unknown method must be -32601")
	}
}

func TestModernRequests(t *testing.T) {
	meta := `"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}`
	got := exchange(t,
		`{"jsonrpc":"2.0","id":"d","method":"server/discover","params":{`+meta+`}}`,
		`{"jsonrpc":"2.0","id":"l","method":"tools/list","params":{`+meta+`}}`,
		`{"jsonrpc":"2.0","id":"c","method":"tools/call","params":{"name":"echo","arguments":{"text":"x"},`+meta+`}}`,
		`{"jsonrpc":"2.0","id":"v","method":"tools/list","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"1900-01-01"}}}`,
	)
	d := got[`"d"`]["result"].(map[string]any)
	if d["resultType"] != "complete" || d["supportedVersions"].([]any)[0] != "2026-07-28" {
		t.Errorf("discover = %v", d)
	}
	for _, id := range []string{`"l"`, `"c"`} {
		if got[id]["result"].(map[string]any)["resultType"] != "complete" {
			t.Errorf("%s missing resultType", id)
		}
	}
	if got[`"v"`]["error"].(map[string]any)["code"].(float64) != -32022 {
		t.Errorf("unsupported version = %v", got[`"v"`])
	}
}

func TestOutputIsOneLinePerMessage(t *testing.T) {
	var out strings.Builder
	server().Serve(strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"echo","arguments":{"text":"fail"}}}`+"\n"), &out)
	if strings.Count(out.String(), "\n") != 1 {
		t.Fatalf("embedded newline in output: %q", out.String())
	}
}
