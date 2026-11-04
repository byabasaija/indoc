package scanner

import (
	"path/filepath"
	"strings"
)

// extensionMap maps file extensions to their canonical programming language name.
var extensionMap = map[string]string{
	// Systems & Backend
	".go":    "Go",
	".py":    "Python",
	".rs":    "Rust",
	".java":  "Java",
	".c":     "C",
	".h":     "C",
	".cpp":   "C++",
	".hpp":   "C++",
	".cc":    "C++",
	".cs":    "C#",
	".rb":    "Ruby",
	".php":   "PHP",
	".swift": "Swift",
	".kt":    "Kotlin",
	".scala": "Scala",

	// Web & Frontend
	".js":   "JavaScript",
	".jsx":  "JavaScript",
	".mjs":  "JavaScript",
	".ts":   "TypeScript",
	".tsx":  "TypeScript",
	".vue":  "Vue",
	".svelte": "Svelte",

	// Query, Schema & Config
	".sql":     "SQL",
	".proto":   "Protocol Buffers",
	".graphql": "GraphQL",
	".gql":     "GraphQL",
	".prisma":  "Prisma",
}

// DetectLanguage inspects a file's extension and returns its canonical language.
// If the extension is not a recognized source code language, it returns "", false.
func DetectLanguage(filePath string) (string, bool) {
	ext := strings.ToLower(filepath.Ext(filePath))
	lang, ok := extensionMap[ext]
	return lang, ok
}

// IsSupportedSourceFile returns true if the file extension is a recognized source language.
func IsSupportedSourceFile(filePath string) bool {
	_, ok := DetectLanguage(filePath)
	return ok
}

