package reference

import "testing"

func TestSources(t *testing.T) {
	got := Sources([]string{
		"app/main.py", "app/tests/test_api.py", "app/test_models.py", "conftest.py",
		"cmd/main.go", "cmd/main_test.go", "web/app.ts", "web/app.spec.ts", "README.md",
	})
	want := []string{"app/main.py", "cmd/main.go", "web/app.ts"}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for i, w := range want {
		if got[i].Path != w {
			t.Errorf("got[%d] = %s, want %s", i, got[i].Path, w)
		}
	}
}
