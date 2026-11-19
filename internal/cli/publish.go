package cli

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/byabasaija/indoc/internal/config"
	"github.com/byabasaija/indoc/internal/drift"
	"github.com/byabasaija/indoc/internal/projectmap"
	"github.com/byabasaija/indoc/internal/version"
)

var publishTarget, publishPath, publishAccess string

var publishCmd = &cobra.Command{
	Use:   "publish",
	Short: "Choose where the docs are served and print how to wire it up",
	Long: `publish records where the site is served and explains how to get it there.

  --target pages   GitHub Pages: writes .github/workflows/indoc-pages.yml
  --target host    any static host (Netlify, Vercel, Cloudflare Pages, S3)
  --target app     served by this project's web app under --path, with
                   --access public or login; prints the code for the
                   framework it detects (Django, FastAPI, Flask, Express,
                   NestJS, Next.js, Astro, Go)
  --target none    build locally only

indoc writes only its own files (the config and, for pages, the workflow).
Code for your app is printed for you or your agent to add.

Without flags, publish prints the instructions for the current target.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPublish(cfgFile, cmd.OutOrStdout(), publishTarget, publishPath, publishAccess)
	},
}

func init() {
	publishCmd.Flags().StringVar(&publishTarget, "target", "", "pages, host, app, or none")
	publishCmd.Flags().StringVar(&publishPath, "path", "", "URL path in the app, such as /guide/ (target app)")
	publishCmd.Flags().StringVar(&publishAccess, "access", "", "public or login (target app)")
	rootCmd.AddCommand(publishCmd)
}

const pagesWorkflow = ".github/workflows/indoc-pages.yml"

func runPublish(configPath string, out io.Writer, target, urlPath, access string) error {
	paths := config.Resolve(configPath)
	cfg, err := config.Load(paths.Config)
	if err != nil {
		return err
	}
	repo, err := drift.NewRepo(paths.Root, cfg.Exclude)
	if err != nil {
		return err
	}
	fw := appFramework(projectmap.Frameworks(paths.Root, repo.Files), paths.Root)

	if target != "" {
		p := &config.Publish{Target: target}
		if target == "app" {
			p.Path = normalizePath(urlPath)
			if p.Path == "" {
				p.Path = "/guide/"
			}
			p.Access = access
			if p.Access == "" {
				p.Access = "public"
			}
			// Many app servers do not serve a folder's index.html, so link to
			// files. An explicit urls: setting is kept.
			if cfg.URLs == "" {
				cfg.URLs = "files"
			}
			if fw.publicDir && !strings.HasPrefix(cfg.OutputDir, "public/") {
				cfg.OutputDir = "public" + p.Path
				cfg.OutputDir = strings.TrimSuffix(cfg.OutputDir, "/")
			}
		}
		if target == "pages" && cfg.SiteURL == "" {
			if repoName := githubRepo(cfg.SourceURL); repoName != "" {
				owner, name, _ := strings.Cut(repoName, "/")
				cfg.SiteURL = "https://" + strings.ToLower(owner) + ".github.io/" + name + "/"
			}
		}
		cfg.Publish = p
		if err := cfg.Validate(); err != nil {
			return err
		}
		if err := cfg.Save(paths.Config); err != nil {
			return err
		}
		fmt.Fprintf(out, "Saved publish settings in %s.\n\n", paths.Config)
	}
	if cfg.Publish == nil {
		fmt.Fprintln(out, "No publish target is set. Choose one:")
		fmt.Fprintln(out, "  indoc publish --target pages                  GitHub Pages")
		fmt.Fprintln(out, "  indoc publish --target host                   Netlify, Vercel, Cloudflare Pages, S3")
		fmt.Fprintln(out, "  indoc publish --target app --path /guide/     served by this project's web app")
		fmt.Fprintln(out, "  indoc publish --target none                   build locally only")
		return nil
	}

	switch cfg.Publish.Target {
	case "pages":
		return publishPages(paths.Root, cfg, out)
	case "host":
		publishHost(cfg, out)
	case "app":
		publishApp(cfg, fw, out)
	case "none":
		fmt.Fprintf(out, "The site is built into %s and not published. Open its index.html, or run indoc preview.\n", cfg.OutputDir)
	}
	return nil
}

func normalizePath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	if !strings.HasSuffix(p, "/") {
		p += "/"
	}
	return p
}

func releaseTag() string {
	v := strings.TrimSuffix(strings.TrimPrefix(version.Version, "v"), "-dev")
	return "v" + v
}

func installLine() string {
	return "curl -fsSL https://github.com/byabasaija/indoc/releases/latest/download/install.sh | sh -s -- " + releaseTag()
}

func publishPages(root string, cfg *config.Config, out io.Writer) error {
	wf := filepath.Join(root, filepath.FromSlash(pagesWorkflow))
	if _, err := os.Stat(wf); err == nil {
		fmt.Fprintf(out, "%s already exists; left unchanged.\n", pagesWorkflow)
	} else {
		if err := os.MkdirAll(filepath.Dir(wf), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(wf, []byte(pagesWorkflowYAML(defaultBranch(root))), 0o644); err != nil {
			return err
		}
		fmt.Fprintf(out, "Wrote %s: builds the docs and deploys them to GitHub Pages on every push to the default branch.\n", pagesWorkflow)
	}
	fmt.Fprintln(out, "\nOne step on GitHub: Settings → Pages → Build and deployment → Source: GitHub Actions.")
	if cfg.SiteURL != "" {
		fmt.Fprintf(out, "The site will be at %s (site_url in the config; change it for a custom domain).\n", cfg.SiteURL)
	}
	fmt.Fprintf(out, "Commit %s and the config, then push.\n", pagesWorkflow)
	return nil
}

func pagesWorkflowYAML(branch string) string {
	return `# Written by indoc publish. Builds the docs and deploys them to GitHub Pages.
name: Docs

on:
  push:
    branches: [` + branch + `]
  workflow_dispatch:

permissions:
  contents: read
  pages: write
  id-token: write

concurrency:
  group: pages
  cancel-in-progress: true

jobs:
  deploy:
    runs-on: ubuntu-latest
    environment:
      name: github-pages
      url: ${{ steps.deployment.outputs.page_url }}
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0 # "Last updated" dates come from git history
      - uses: byabasaija/indoc@` + releaseTag() + `
        with:
          command: bundle
          out: _site
      - uses: actions/configure-pages@v5
      - uses: actions/upload-pages-artifact@v3
        with:
          path: _site
      - id: deployment
        uses: actions/deploy-pages@v4
`
}

// defaultBranch is the remote's default branch, or main.
func defaultBranch(root string) string {
	out, err := exec.Command("git", "-C", root, "symbolic-ref", "--short", "refs/remotes/origin/HEAD").Output()
	if err == nil {
		if b := strings.TrimPrefix(strings.TrimSpace(string(out)), "origin/"); b != "" {
			return b
		}
	}
	return "main"
}

func publishHost(cfg *config.Config, out io.Writer) {
	fmt.Fprintf(out, `Static host. Configure it with:

  build command:     %s && indoc bundle
  publish directory: %s

Netlify, Vercel, and Cloudflare Pages serve a folder's index.html, so pretty
URLs work. Amazon S3 website hosting and some CDNs do not: set "urls: files"
in the config for those.
`, installLine(), cfg.OutputDir)
	if cfg.SiteURL == "" {
		fmt.Fprintln(out, "\nSet site_url to the published address to get canonical links and sitemap.xml.")
	}
}

// framework is how a web framework serves a folder of files.
type framework struct {
	name      string
	publicDir bool // serves public/ as-is; the build goes there
}

func appFramework(found []string, root string) framework {
	for _, want := range []string{"Django", "FastAPI", "Flask", "NestJS", "Next.js", "Astro", "Express", "Gin", "Echo", "chi", "Fiber"} {
		for _, f := range found {
			if f == want {
				return framework{name: want, publicDir: want == "Next.js" || want == "Astro"}
			}
		}
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
		return framework{name: "Go"}
	}
	return framework{}
}

func publishApp(cfg *config.Config, fw framework, out io.Writer) {
	p := cfg.Publish
	dir := cfg.OutputDir
	route := strings.TrimSuffix(p.Path, "/")
	login := p.Access == "login"
	name := fw.name
	if name == "" {
		name = "your web app"
	}
	fmt.Fprintf(out, "Served by %s at %s (%s). The site is built into %s with file URLs (urls: %s).\n\n",
		name, p.Path, map[bool]string{true: "signed-in users only", false: "public"}[login], dir, cfg.URLs)

	fmt.Fprintln(out, "1. Add this to the app:")
	fmt.Fprintln(out)
	fmt.Fprintln(out, indent(appGlue(fw.name, route, dir, login)))

	fmt.Fprintln(out, "2. Build the docs wherever the app is built or deployed, before it starts:")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "     "+installLine())
	fmt.Fprintln(out, "     indoc bundle")
	fmt.Fprintln(out)
	switch fw.name {
	case "Django":
		fmt.Fprintln(out, "   For example in the same step that runs collectstatic.")
	case "Next.js", "Astro":
		fmt.Fprintln(out, `   For example as a "prebuild" script in package.json: "prebuild": "indoc bundle".`)
	}
	if !strings.HasPrefix(dir, ".indoc/") {
		fmt.Fprintf(out, "\n3. Add %s/ to .gitignore; it is rebuilt on every deploy.\n", dir)
	}
	fmt.Fprintf(out, "\nLocally: indoc bundle, start the app, and open %s.\n", p.Path)
}

func appGlue(fw, route, dir string, login bool) string {
	switch fw {
	case "Django":
		imp, check := "", ""
		if login {
			imp = "from django.contrib.auth.views import redirect_to_login\n"
			check = `    if not request.user.is_staff:  # staff only; sign-in at settings.LOGIN_URL
        return redirect_to_login(request.get_full_path())
`
		}
		return `# In your root URLconf (urls.py). BASE_DIR is the folder with manage.py.
from pathlib import Path

from django.conf import settings
` + imp + `from django.shortcuts import redirect
from django.urls import re_path
from django.views.static import serve

DOCS_DIR = Path(settings.BASE_DIR) / "` + dir + `"

def docs(request, path):
` + check + `    if path == "" or path.endswith("/"):
        path += "index.html"
    elif (DOCS_DIR / path).is_dir():
        return redirect(request.path + "/")
    return serve(request, path, document_root=DOCS_DIR)

urlpatterns += [re_path(r"^` + strings.TrimPrefix(route, "/") + `/(?P<path>.*)$", docs)]`
	case "FastAPI":
		s := `# Where the FastAPI app is created. Paths are relative to the working directory.
from fastapi.staticfiles import StaticFiles

app.mount("` + route + `", StaticFiles(directory="` + dir + `", html=True), name="docs")`
		if login {
			s += "\n\n# StaticFiles has no authentication: require sign-in for " + route + "/ in\n# your auth middleware or reverse proxy."
		}
		return s
	case "Flask":
		guard := ""
		if login {
			guard = "@login_required  # from flask_login, or your own decorator\n"
		}
		return `# Where the Flask app is created.
import os

from flask import send_from_directory

DOCS_DIR = os.path.join(os.getcwd(), "` + dir + `")

@app.route("` + route + `/", defaults={"path": "index.html"})
@app.route("` + route + `/<path:path>")
` + guard + `def docs(path):
    if path.endswith("/"):
        path += "index.html"
    return send_from_directory(DOCS_DIR, path)`
	case "Express":
		mw := ""
		if login {
			mw = "requireLogin, "
		}
		s := `// Where the Express app is created.
const path = require("path");

app.use("` + route + `", ` + mw + `express.static(path.join(process.cwd(), "` + dir + `")));`
		if login {
			s += "\n// requireLogin is your own middleware that rejects signed-out users."
		}
		return s
	case "NestJS":
		s := `// npm install @nestjs/serve-static, then in your root module:
import { ServeStaticModule } from "@nestjs/serve-static";
import { join } from "path";

@Module({
  imports: [
    ServeStaticModule.forRoot({
      rootPath: join(process.cwd(), "` + dir + `"),
      serveRoot: "` + route + `",
    }),
  ],
})`
		if login {
			s += "\n// ServeStaticModule has no guards: require sign-in for " + route + " in middleware."
		}
		return s
	case "Next.js", "Astro":
		s := `// Nothing to add: files in public/ are served as they are, so the docs
// are at ` + route + `/index.html once ` + dir + ` is built.`
		if login && fw == "Next.js" {
			s += `
// To require sign-in, add a middleware.ts with:
//   export const config = { matcher: ["` + route + `/:path*"] };
// and redirect signed-out users there.`
		}
		return s
	case "Gin":
		return `r.Static("` + route + `", "` + dir + `")`
	case "Echo":
		return `e.Static("` + route + `", "` + dir + `")`
	case "chi":
		return `r.Handle("` + route + `/*", http.StripPrefix("` + route + `/", http.FileServer(http.Dir("` + dir + `"))))`
	case "Fiber":
		return `app.Static("` + route + `", "` + dir + `")`
	case "Go":
		return `http.Handle("` + route + `/", http.StripPrefix("` + route + `/", http.FileServer(http.Dir("` + dir + `"))))`
	}
	return `Serve the folder ` + dir + ` at ` + route + `/ as static files. With urls: files,
any static file server works: no folder index is needed.`
}

func indent(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if l != "" {
			lines[i] = "     " + l
		}
	}
	return strings.Join(lines, "\n") + "\n"
}
