package reference

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/byabasaija/indoc/internal/doc"
)

const maxFileBytes = 1 << 20

// SourceFile is a file the extractor may read.
type SourceFile struct {
	Path     string
	Language string
}

var (
	pyRoute     = regexp.MustCompile(`(?i)^\s*@[\w.]+\.(get|post|put|patch|delete|head|options)\(`)
	jsRoute     = regexp.MustCompile(`\b(?:app|router)\.(get|post|put|patch|delete)\(`)
	goRoute     = regexp.MustCompile(`\.(GET|POST|PUT|PATCH|DELETE|Get|Post|Put|Patch|Delete|HandleFunc|Handle)\(`)
	quotedPath  = regexp.MustCompile(`["'](/[^"']*)["']`)
	envPatterns = []*regexp.Regexp{
		regexp.MustCompile(`os\.Getenv\(\s*"([A-Z][A-Z0-9_]*)"`),
		regexp.MustCompile(`process\.env\.([A-Z][A-Z0-9_]+)`),
		regexp.MustCompile(`process\.env\[\s*["']([A-Z][A-Z0-9_]+)["']\s*\]`),
	}
)

// generic is the best-effort regex extractor: decorator-style routes
// (Flask/FastAPI, Express, Go) and environment reads in Go and JavaScript.
// Python environment reads are handled by extractPythonEnv.
func generic(root string, files []SourceFile) ([]doc.RefSection, error) {
	var routes []doc.RefEntry
	var config []doc.RefEntry
	seenRoutes := map[string]bool{}
	seenConfig := map[string]bool{}

	for _, file := range files {
		content, ok, err := readSource(filepath.Join(root, filepath.FromSlash(file.Path)))
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		path := filepath.ToSlash(file.Path)
		for _, entry := range routesIn(path, file.Language, content) {
			key := entry.Detail + " " + entry.Name + " " + doc.SourcePath(entry.Source)
			if seenRoutes[key] {
				continue
			}
			seenRoutes[key] = true
			routes = append(routes, entry)
		}
		if file.Language == "Python" {
			continue
		}
		for _, entry := range configIn(path, content) {
			if seenConfig[entry.Name] {
				continue
			}
			seenConfig[entry.Name] = true
			config = append(config, entry)
		}
	}

	sort.Slice(routes, func(i, j int) bool {
		if routes[i].Name == routes[j].Name {
			return routes[i].Detail < routes[j].Detail
		}
		return routes[i].Name < routes[j].Name
	})
	sort.Slice(config, func(i, j int) bool {
		return config[i].Name < config[j].Name
	})

	var sections []doc.RefSection
	if len(routes) > 0 {
		sections = append(sections, doc.RefSection{
			ID:      "routes",
			Title:   "Routes",
			Kind:    doc.KindRoutes,
			Entries: routes,
		})
	}
	if len(config) > 0 {
		sections = append(sections, doc.RefSection{
			ID:      "config",
			Title:   "Configuration",
			Kind:    doc.KindConfig,
			Entries: config,
		})
	}
	if sections == nil {
		return []doc.RefSection{}, nil
	}
	return sections, nil
}

func readSource(path string) (string, bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", false, fmt.Errorf("failed to read %s: %w", path, err)
	}
	if info.Size() > maxFileBytes {
		return "", false, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false, fmt.Errorf("failed to read %s: %w", path, err)
	}
	if bytesContainNUL(data) {
		return "", false, nil
	}
	return string(data), true, nil
}

func bytesContainNUL(data []byte) bool {
	for _, b := range data {
		if b == 0 {
			return true
		}
	}
	return false
}

func routesIn(path, language, content string) []doc.RefEntry {
	lines := strings.Split(content, "\n")
	var entries []doc.RefEntry
	for i, line := range lines {
		if language == "Go" && strings.Contains(line, "func ") {
			continue
		}
		method, ok := routeMethod(language, line)
		if !ok {
			continue
		}
		routePath := firstPath(window(lines, i))
		if routePath == "" {
			continue
		}
		entries = append(entries, doc.RefEntry{
			Name:   routePath,
			Detail: method,
			Source: fmt.Sprintf("%s:%d", path, i+1),
		})
	}
	return entries
}

func routeMethod(language, line string) (string, bool) {
	switch language {
	case "Python":
		if match := pyRoute.FindStringSubmatch(line); match != nil {
			return strings.ToUpper(match[1]), true
		}
	case "JavaScript", "TypeScript": // ":env" variants skip routes
		if match := jsRoute.FindStringSubmatch(line); match != nil {
			return strings.ToUpper(match[1]), true
		}
	case "Go":
		if match := goRoute.FindStringSubmatch(line); match != nil {
			return canonicalGoMethod(match[1]), true
		}
	}
	return "", false
}

func canonicalGoMethod(raw string) string {
	switch strings.ToLower(raw) {
	case "handle", "handlefunc":
		return "ANY"
	default:
		return strings.ToUpper(raw)
	}
}

func window(lines []string, start int) string {
	parts := []string{lines[start]}
	if firstPath(parts[0]) != "" {
		return parts[0]
	}
	for j := start + 1; j < len(lines) && j < start+8; j++ {
		if pyRoute.MatchString(lines[j]) || jsRoute.MatchString(lines[j]) || goRoute.MatchString(lines[j]) {
			break
		}
		parts = append(parts, lines[j])
		if firstPath(strings.Join(parts, "\n")) != "" {
			break
		}
	}
	return strings.Join(parts, "\n")
}

func firstPath(chunk string) string {
	match := quotedPath.FindStringSubmatch(chunk)
	if match == nil {
		return ""
	}
	return match[1]
}

func configIn(path, content string) []doc.RefEntry {
	lines := strings.Split(content, "\n")
	var entries []doc.RefEntry
	for i, line := range lines {
		for _, pattern := range envPatterns {
			match := pattern.FindStringSubmatch(line)
			if match == nil || len(match[1]) < 2 {
				continue
			}
			entries = append(entries, doc.RefEntry{
				Name:   match[1],
				Source: fmt.Sprintf("%s:%d", path, i+1),
			})
			break
		}
	}
	return entries
}
