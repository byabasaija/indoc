package reference

import (
	"fmt"
	"path"
	"strings"

	"github.com/byabasaija/indoc/internal/doc"
	"github.com/byabasaija/indoc/internal/lang/python"
)

// extractManagementCommands lists Django management commands:
// <app>/management/commands/<name>.py files that define class Command.
func extractManagementCommands(root string, files []string, cov *Coverage) []doc.RefEntry {
	py := newPyProject(root, nil)
	var out []doc.RefEntry
	for _, f := range files {
		dir, base := path.Split(f)
		if !strings.HasSuffix(dir, "/management/commands/") || !strings.HasSuffix(base, ".py") || strings.HasPrefix(base, "_") {
			continue
		}
		mod := py.load(f)
		cls := mod.Classes["Command"]
		if cls == nil {
			continue
		}
		name := strings.TrimSuffix(base, ".py")
		e := doc.RefEntry{
			Name:    name,
			Handler: "python manage.py " + name,
			Source:  fmt.Sprintf("%s:%d", f, cls.Line),
		}
		for _, a := range cls.Assigns {
			if s, ok := a.Value.(*python.Str); ok && a.Target == "help" {
				e.Description = strings.Join(strings.Fields(s.Value), " ")
			}
		}
		e.Detail = strings.Join(commandArguments(mod), " ")
		out = append(out, e)
		cov.Files++
	}
	return out
}

// commandArguments returns the names passed to parser.add_argument(...).
func commandArguments(m *python.Module) []string {
	var args []string
	for _, c := range m.AllCalls {
		if !strings.HasSuffix(c.FuncName(), ".add_argument") {
			continue
		}
		var names []string
		for _, a := range c.Args {
			if s, ok := a.(*python.Str); ok && s.Literal {
				names = append(names, s.Value)
			}
		}
		if len(names) == 0 {
			continue
		}
		arg := strings.Join(names, "/")
		if !strings.HasPrefix(names[0], "-") {
			arg = "<" + arg + ">"
		}
		args = append(args, arg)
	}
	return args
}
