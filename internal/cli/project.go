package cli

import (
	"fmt"
	"strings"

	"github.com/byabasaija/indoc/internal/config"
	"github.com/byabasaija/indoc/internal/drift"
	"github.com/byabasaija/indoc/internal/pages"
)

// project is everything a command needs about the repository.
type project struct {
	paths config.Paths
	cfg   *config.Config
	repo  *drift.Repo
	pages []pages.Page
	lock  *drift.Lock
}

func openProject(configPath string) (*project, error) {
	paths := config.Resolve(configPath)
	cfg, err := config.Load(paths.Config)
	if err != nil {
		return nil, err
	}
	if len(cfg.Pages) == 0 {
		return nil, fmt.Errorf("no pages configured: add globs under `pages:` in %s (for example documentation/**/*.md)", paths.Config)
	}
	repo, err := drift.NewRepo(paths.Root, cfg.Exclude)
	if err != nil {
		return nil, err
	}
	files := pages.Match(repo.Files, cfg.Pages)
	if len(files) == 0 {
		return nil, fmt.Errorf("`pages:` in %s matches no Markdown files: %s", paths.Config, strings.Join(cfg.Pages, ", "))
	}
	ps, err := pages.Load(paths.Root, files)
	if err != nil {
		return nil, err
	}
	lock, err := drift.LoadLock(paths.Lock)
	if err != nil {
		return nil, err
	}
	return &project{paths: paths, cfg: cfg, repo: repo, pages: ps, lock: lock}, nil
}
