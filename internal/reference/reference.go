package reference

import (
	"sort"

	"github.com/byabasaija/indoc/internal/doc"
)

// Coverage reports what one extractor found and, as importantly, what it
// could not resolve. A framework that is detected but yields nothing is a
// warning, never silence.
type Coverage struct {
	Extractor  string   `json:"extractor"`
	Evidence   []string `json:"evidence,omitempty"`
	Files      int      `json:"files"`
	Entries    int      `json:"entries"`
	Unresolved []string `json:"unresolved,omitempty"`
	Warnings   []string `json:"warnings,omitempty"`
}

// Report is the extracted reference plus per-extractor coverage.
type Report struct {
	Sections []doc.RefSection `json:"sections"`
	Coverage []Coverage       `json:"coverage"`
}

// Extract runs every applicable extractor over the repository files
// (slash-separated, relative to root).
func Extract(root string, files []string) (*Report, error) {
	sources := Sources(files)
	report := &Report{Sections: []doc.RefSection{}, Coverage: []Coverage{}}

	var routes, config, commands []doc.RefEntry

	djangoRoutes, djangoCov := extractDjango(root, nonTest(files))
	if djangoCov != nil {
		routes = append(routes, djangoRoutes...)
		commands = extractManagementCommands(root, files, djangoCov)
		djangoCov.Entries += len(commands)
		report.Coverage = append(report.Coverage, *djangoCov)
	}

	envEntries, envCov := extractPythonEnv(root, sources, files)
	if envCov != nil {
		config = append(config, envEntries...)
		report.Coverage = append(report.Coverage, *envCov)
	}

	genericSections, err := generic(root, sources)
	if err != nil {
		return nil, err
	}
	genericCov := Coverage{Extractor: "generic (best effort)", Files: len(sources)}
	for _, s := range genericSections {
		genericCov.Entries += len(s.Entries)
		switch s.Kind {
		case doc.KindRoutes:
			routes = append(routes, s.Entries...)
		case doc.KindConfig:
			config = appendNew(config, s.Entries)
		}
	}
	if genericCov.Entries > 0 {
		report.Coverage = append(report.Coverage, genericCov)
	}

	sort.SliceStable(routes, func(i, j int) bool {
		if routes[i].Name != routes[j].Name {
			return routes[i].Name < routes[j].Name
		}
		return routes[i].Detail < routes[j].Detail
	})
	sort.SliceStable(config, func(i, j int) bool { return config[i].Name < config[j].Name })
	sort.SliceStable(commands, func(i, j int) bool { return commands[i].Name < commands[j].Name })

	for _, s := range []doc.RefSection{
		{ID: "routes", Title: "Routes", Kind: doc.KindRoutes, Entries: routes},
		{ID: "config", Title: "Configuration", Kind: doc.KindConfig, Entries: config},
		{ID: "commands", Title: "Commands", Kind: doc.KindCommands, Entries: commands},
	} {
		if len(s.Entries) > 0 {
			report.Sections = append(report.Sections, s)
		}
	}
	return report, nil
}

func nonTest(files []string) []string {
	var out []string
	for _, f := range files {
		if !isTest(f) {
			out = append(out, f)
		}
	}
	return out
}

func appendNew(dst, src []doc.RefEntry) []doc.RefEntry {
	seen := map[string]bool{}
	for _, e := range dst {
		seen[e.Name] = true
	}
	for _, e := range src {
		if !seen[e.Name] {
			seen[e.Name] = true
			dst = append(dst, e)
		}
	}
	return dst
}
