package doc

import "testing"

func TestSourcePath(t *testing.T) {
	cases := map[string]string{
		"app/main.py:12":    "app/main.py",
		"C:/repo/main.go:3": "C:/repo/main.go",
		"app/main.py":       "app/main.py",
		"app/main.py:x":     "app/main.py:x",
	}
	for in, want := range cases {
		if got := SourcePath(in); got != want {
			t.Errorf("SourcePath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSlug(t *testing.T) {
	cases := map[string]string{"Getting Started!": "getting-started", "  ": "page", "API v2 — Users": "api-v2-users"}
	for in, want := range cases {
		if got := Slug(in); got != want {
			t.Errorf("Slug(%q) = %q, want %q", in, got, want)
		}
	}
}
