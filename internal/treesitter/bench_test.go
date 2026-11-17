package treesitter

import (
	"os"
	"strings"
	"testing"
)

func BenchmarkParseCorpus(b *testing.B) {
	list := os.Getenv("TS_BENCH_FILES")
	if list == "" {
		b.Skip("set TS_BENCH_FILES to a file of paths")
	}
	data, _ := os.ReadFile(list)
	var files [][]byte
	for _, p := range strings.Fields(string(data)) {
		if src, err := os.ReadFile(p); err == nil && len(src) < 1<<20 {
			files = append(files, src)
		}
	}
	lang := os.Getenv("TS_BENCH_LANG")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, src := range files {
			if _, err := Parse(lang, src); err != nil {
				b.Fatal(err)
			}
		}
	}
}
