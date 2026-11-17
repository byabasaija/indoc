package projectmap

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/byabasaija/indoc/internal/code"
)

func TestBuild(t *testing.T) {
	root := t.TempDir()
	for p, c := range map[string]string{
		"manage.py":       "import sys\n",
		"pyproject.toml":  "[project]\nname = \"app\"\ndependencies = [\"django>=5\", \"djangorestframework\"]\n\n[project.scripts]\napp-sync = \"app.cli:main\"\n",
		"justfile":        "set dotenv-load\nport := \"8000\"\n\n# Start the dev server\nserve:\n    python manage.py runserver\n\ntest *args:\n    pytest {{args}}\n",
		"Makefile":        ".PHONY: lint\n# Lint everything\nlint:\n\truff check .\n%.o: %.c\n\tcc $<\n",
		"app/models.py":   "class Item:\n    pass\n",
		"app/views.py":    "from .models import Item\nfrom app import services\n",
		"app/services.py": "from app.models import Item\n",
		"docs/notes.md":   "# notes\n",
		"docs/index.md":   "# index\n",
		"LICENSE.md":      "MIT\n",
	} {
		full := filepath.Join(root, p)
		os.MkdirAll(filepath.Dir(full), 0o755)
		os.WriteFile(full, []byte(c), 0o644)
	}
	files, _ := code.ListFiles(root, nil)
	m := Build(root, files, []string{"docs/index.md"})

	if !reflect.DeepEqual(m.Frameworks, []string{"Django REST framework", "Django"}) {
		t.Errorf("frameworks = %v", m.Frameworks)
	}
	if !reflect.DeepEqual(m.EntryPoints, []string{"manage.py"}) {
		t.Errorf("entry points = %v", m.EntryPoints)
	}
	var tasks []string
	for _, tk := range m.Tasks {
		tasks = append(tasks, tk.Command+" # "+tk.Comment)
	}
	want := []string{"just serve # Start the dev server", "just test # ", "make lint # Lint everything", "app-sync # app.cli:main"}
	if !reflect.DeepEqual(tasks, want) {
		t.Errorf("tasks = %q", tasks)
	}
	if !reflect.DeepEqual(m.Docs, []string{"docs/notes.md"}) {
		t.Errorf("docs = %v", m.Docs)
	}
	if len(m.Central) == 0 || m.Central[0].Path != "app/models.py" || m.Central[0].Count != 2 {
		t.Errorf("central = %+v", m.Central)
	}
	if m.Languages[0].Name != "Python" || m.Languages[0].Files != 4 || !m.Languages[0].Parsed {
		t.Errorf("languages = %+v", m.Languages)
	}
}
