package glob

import "testing"

func TestMatch(t *testing.T) {
	cases := []struct {
		pattern, name string
		want          bool
	}{
		{"docs/**/*.md", "docs/a.md", true},
		{"docs/**/*.md", "docs/partner/a.md", true},
		{"docs/**/*.md", "docs/a/b/c.md", true},
		{"docs/**/*.md", "other/a.md", false},
		{"docs/*.md", "docs/partner/a.md", false},
		{"*.md", "README.md", true},
		{"*.md", "docs/a.md", false},
		{"**/*.md", "docs/a.md", true},
		{"**", "anything/at/all.py", true},
		{"shop/core/services/**", "shop/core/services/checkout/handler.py", true},
		{"shop/core/services/**", "shop/core/models.py", false},
		{"./docs/a.md", "docs/a.md", true},
		{"src/**/test_?.py", "src/x/test_a.py", true},
		{"[", "[", false},
	}
	for _, c := range cases {
		if got := Match(c.pattern, c.name); got != c.want {
			t.Errorf("Match(%q, %q) = %v, want %v", c.pattern, c.name, got, c.want)
		}
	}
}

func TestValid(t *testing.T) {
	if !Valid("docs/**/*.md") || Valid("docs/[") {
		t.Fatal("Valid misclassified a pattern")
	}
}
