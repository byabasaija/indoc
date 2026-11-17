// Command tsbench measures tree-sitter parsing: startup and throughput.
//
//	find DIR -name '*.ts' | go run ./tools/tsbench -lang typescript
//
// Development tool only.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/byabasaija/indoc/internal/treesitter"
)

func main() {
	lang := flag.String("lang", "typescript", "grammar")
	jobs := flag.Int("j", 1, "parallel parses")
	flag.Parse()

	var files [][]byte
	var bytes int
	sc := bufio.NewScanner(os.Stdin)
	for sc.Scan() {
		data, err := os.ReadFile(sc.Text())
		if err != nil || len(data) > 1<<20 {
			continue
		}
		files = append(files, data)
		bytes += len(data)
	}

	start := time.Now()
	if _, err := treesitter.Parse(*lang, []byte("x")); err != nil {
		fmt.Println("load:", err)
		os.Exit(1)
	}
	load := time.Since(start)

	start = time.Now()
	var nodes, errs atomic.Int64
	var wg sync.WaitGroup
	work := make(chan []byte)
	for w := 0; w < *jobs; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for src := range work {
				root, err := treesitter.Parse(*lang, src)
				if err != nil {
					errs.Add(1)
					continue
				}
				n := int64(0)
				root.Walk(func(*treesitter.Node) bool { n++; return true })
				nodes.Add(n)
			}
		}()
	}
	for _, src := range files {
		work <- src
	}
	close(work)
	wg.Wait()
	parse := time.Since(start)
	fmt.Printf("%-10s load %6.0f ms | %5d files %6.1f MB in %6.2f s = %6.0f files/s, %5.1f MB/s | %d nodes, %d failures\n",
		*lang, ms(load), len(files), float64(bytes)/1e6, parse.Seconds(), float64(len(files))/parse.Seconds(),
		float64(bytes)/1e6/parse.Seconds(), nodes.Load(), errs.Load())

}

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }
