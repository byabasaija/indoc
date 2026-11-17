package reference

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/byabasaija/indoc/internal/code"
	"github.com/byabasaija/indoc/internal/doc"
)

const page = "export default function Page() { return null }\n"

var nextFixture = map[string]string{
	"package.json":                     `{"name": "web", "dependencies": {"next": "*"}}`,
	"next.config.js":                   "module.exports = {}\n",
	"app/layout.tsx":                   "export default function L({ children }: { children: React.ReactNode }) { return <html><body>{children}</body></html> }\n",
	"app/page.tsx":                     page,
	"app/(marketing)/about/page.tsx":   page,
	"app/blog/[slug]/page.tsx":         page,
	"app/docs/[...path]/page.tsx":      page,
	"app/shop/[[...filters]]/page.tsx": page,
	"app/_internal/page.tsx":           page,
	"app/dashboard/loading.tsx":        page,
	"app/api/users/route.ts":           "export async function GET() { return Response.json([]) }\nexport const POST = async () => Response.json({})\n",
	"app/api/users/[id]/route.ts":      "const handler = async () => new Response(null)\nexport { handler as GET, handler as DELETE }\n",
	"pages/legacy.tsx":                 page,
	"pages/posts/[id].tsx":             page,
	"pages/api/hello.ts":               "export default function handler(req: any, res: any) { res.json({}) }\n",
	"pages/_app.tsx":                   "export default function App({ Component, pageProps }: any) { return <Component {...pageProps} /> }\n",
}

func TestNextExtraction(t *testing.T) {
	root := writeFixture(t, nextFixture)
	files, _ := code.ListFiles(root, nil)
	r, err := Extract(root, files)
	if err != nil {
		t.Fatal(err)
	}
	var got, paths []string
	for _, e := range section(r, doc.KindRoutes) {
		got = append(got, e.Detail+" "+e.Name)
		paths = append(paths, e.Name)
	}
	want := []string{
		"GET /", "GET /about", "GET /blog/[slug]", "GET /docs/[...path]", "GET /shop/[[...filters]]",
		"GET, POST /api/users", "GET, DELETE /api/users/[id]",
		"GET /legacy", "GET /posts/[id]", "ANY /api/hello",
	}
	sort.Strings(got)
	sort.Strings(want)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("routes:\n%s\n\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	// Against `next build`'s route manifests: INDOC_NEXT_ENV=<dir with node_modules/next>.
	env := os.Getenv("INDOC_NEXT_ENV")
	if env == "" {
		return
	}
	if err := os.Symlink(filepath.Join(env, "node_modules"), filepath.Join(root, "node_modules")); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("node", filepath.Join(root, "node_modules/next/dist/bin/next"), "build")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "NEXT_TELEMETRY_DISABLED=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("next build: %v\n%s", err, out)
	}
	truth := map[string]bool{}
	var appRoutes map[string]string
	readJSON(t, filepath.Join(root, ".next/app-path-routes-manifest.json"), &appRoutes)
	for _, route := range appRoutes {
		truth[route] = true
	}
	var pagesManifest map[string]string
	readJSON(t, filepath.Join(root, ".next/server/pages-manifest.json"), &pagesManifest)
	for route := range pagesManifest {
		truth[route] = true
	}
	for _, internal := range []string{"/_app", "/_document", "/_error", "/404", "/500", "/_not-found", "/favicon.ico"} {
		delete(truth, internal)
	}
	var want2 []string
	for p := range truth {
		want2 = append(want2, p)
	}
	sort.Strings(want2)
	sort.Strings(paths)
	if strings.Join(want2, "\n") != strings.Join(paths, "\n") {
		t.Errorf("next build disagrees:\nnext:\n%s\n\nindoc:\n%s", strings.Join(want2, "\n"), strings.Join(paths, "\n"))
	} else {
		t.Logf("matches next build: %d routes", len(paths))
	}
}

func readJSON(t *testing.T, path string, v any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		t.Fatal(err)
	}
}

func TestAstroExtraction(t *testing.T) {
	root := writeFixture(t, map[string]string{
		"astro.config.mjs":               "export default {}\n",
		"src/pages/index.astro":          "---\n---\n<h1/>\n",
		"src/pages/about.md":             "# About\n",
		"src/pages/blog/[slug].astro":    "---\n---\n",
		"src/pages/docs/[...path].astro": "---\n---\n",
		"src/pages/og/[slug].png.ts":     "export const GET = () => new Response()\n",
		"src/pages/api/orders/index.ts":  "export const POST = async () => new Response()\nexport async function GET() {}\n",
		"src/pages/api/any.ts":           "export const ALL = () => new Response()\n",
		"src/pages/_partials/nav.astro":  "---\n---\n",
		"src/pages/_draft.astro":         "---\n---\n",
		"src/pages/api/helpers.ts":       "export const util = 1\n",
		"src/components/Card.astro":      "---\n---\n",
	})
	files, _ := code.ListFiles(root, nil)
	r, err := Extract(root, files)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range section(r, doc.KindRoutes) {
		got = append(got, e.Detail+" "+e.Name)
	}
	want := []string{"GET /", "GET /about", "GET /blog/[slug]", "GET /docs/[...path]", "GET /og/[slug].png",
		"GET, POST /api/orders", "ANY /api/any"}
	sort.Strings(got)
	sort.Strings(want)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("routes:\n%s\n\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	var unresolved string
	for _, c := range r.Coverage {
		if c.Extractor == "astro" {
			unresolved = strings.Join(c.Unresolved, "\n")
		}
	}
	if !strings.Contains(unresolved, "src/pages/api/helpers.ts: endpoint exports no HTTP method handlers") {
		t.Errorf("missing note for a non-endpoint module: %q", unresolved)
	}
}
