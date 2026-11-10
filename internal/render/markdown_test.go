package render

import (
	"strings"
	"testing"
)

func TestRenderHeadingsCodeAndTable(t *testing.T) {
	src := strings.Join([]string{
		"## Run it",
		"",
		"Start the API.",
		"",
		"```go",
		"func main() {}",
		"```",
		"",
		"| Method | Path |",
		"| --- | --- |",
		"| GET | /health |",
		"",
		"## Run it",
	}, "\n")

	html, headings, err := Render(src)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if len(headings) != 2 || headings[0].ID != "run-it" || headings[1].ID != "run-it-2" {
		t.Fatalf("headings = %+v", headings)
	}
	if !strings.Contains(html, `id="run-it"`) || !strings.Contains(html, `id="run-it-2"`) {
		t.Fatalf("html missing heading ids: %s", html)
	}
	if !strings.Contains(html, "<table>") || !strings.Contains(html, "language-go") {
		t.Fatalf("html missing table or code: %s", html)
	}
}
