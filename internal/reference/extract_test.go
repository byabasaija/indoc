package reference

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExtractRoutesAndConfig(t *testing.T) {
	dir := t.TempDir()
	py := filepath.Join(dir, "main.py")
	goFile := filepath.Join(dir, "server.go")
	js := filepath.Join(dir, "routes.js")

	pySrc := []byte(stringsJoin(
		"import os",
		"@app.get(\"/health\")",
		"def health():",
		"    return os.getenv(\"DATABASE_URL\")",
		"@app.post(",
		"    \"/visits\",",
		")",
		"def create():",
		"    pass",
	))
	goSrc := []byte(stringsJoin(
		"package main",
		"func main() {",
		"    http.HandleFunc(\"/ready\", ready)",
		"    r.GET(\"/patients\", list)",
		"    port := os.Getenv(\"PORT\")",
		"    _ = port",
		"}",
		"func (s *Server) Handle(w http.ResponseWriter, r *http.Request) {}",
	))
	jsSrc := []byte(stringsJoin(
		"app.get(\"/users\", list)",
		"axios.get(\"/ignore-me\")",
		"const key = process.env.API_KEY",
	))
	writeFile(t, py, pySrc)
	writeFile(t, goFile, goSrc)
	writeFile(t, js, jsSrc)

	sections, err := Extract([]SourceFile{
		{Path: py, Language: "Python"},
		{Path: goFile, Language: "Go"},
		{Path: js, Language: "JavaScript"},
	})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if len(sections) != 2 {
		t.Fatalf("sections = %+v", sections)
	}

	routes := sections[0].Entries
	wantRoutes := []string{"GET /health", "GET /patients", "ANY /ready", "GET /users", "POST /visits"}
	if len(routes) != len(wantRoutes) {
		t.Fatalf("routes = %+v", routes)
	}
	for i, want := range wantRoutes {
		got := routes[i].Detail + " " + routes[i].Name
		if got != want {
			t.Fatalf("route %d = %q, want %q", i, got, want)
		}
	}

	config := sections[1].Entries
	if len(config) != 3 || config[0].Name != "API_KEY" || config[1].Name != "DATABASE_URL" || config[2].Name != "PORT" {
		t.Fatalf("config = %+v", config)
	}
}

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
}

func stringsJoin(lines ...string) string {
	out := ""
	for i, line := range lines {
		if i > 0 {
			out += "\n"
		}
		out += line
	}
	return out + "\n"
}
