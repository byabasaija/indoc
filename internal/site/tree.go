package site

import (
	"path"
	"sort"
	"strings"
)

// folder is one section of the site: a folder of pages, possibly with an
// index page and sub-sections. The sidebar, breadcrumbs, and page order all
// come from this tree.
type folder struct {
	path      string // "guides/deploy"; "" for the root
	index     *built // the folder's own page (index.md, or guides.md next to guides/)
	pages     []*built
	generated []*built // reference pages indoc extracted, after the user's pages
	subs      []*folder
}

func (f *folder) child(seg string) *folder {
	p := seg
	if f.path != "" {
		p = f.path + "/" + seg
	}
	for _, s := range f.subs {
		if s.path == p {
			return s
		}
	}
	s := &folder{path: p}
	f.subs = append(f.subs, s)
	return s
}

// at returns the folder at dir, creating it and its parents.
func (f *folder) at(dir string) *folder {
	if dir == "" {
		return f
	}
	cur := f
	for _, seg := range strings.Split(dir, "/") {
		cur = cur.child(seg)
	}
	return cur
}

// contains reports whether dir is this folder or inside it.
func (f *folder) contains(dir string) bool {
	return f.path == "" || dir == f.path || strings.HasPrefix(dir, f.path+"/")
}

func (f *folder) title() string {
	if f.index != nil {
		return f.index.Title
	}
	if f.path == "reference" {
		return "Reference"
	}
	return humanize(path.Base(f.path))
}

// rank orders sections: the index page's order, else the lowest order of
// any page inside, else after every ordered section.
func (f *folder) rank() int {
	if f.index != nil && f.index.HasOrder {
		return f.index.Order
	}
	r := 1 << 30
	var walk func(*folder)
	walk = func(g *folder) {
		for _, p := range append([]*built{g.index}, g.pages...) {
			if p != nil && p.HasOrder && p.Order < r {
				r = p.Order
			}
		}
		for _, s := range g.subs {
			walk(s)
		}
	}
	walk(f)
	return r
}

func (f *folder) sort() {
	sort.SliceStable(f.pages, func(i, j int) bool { return pageLess(f.pages[i].Page, f.pages[j].Page) })
	sort.SliceStable(f.subs, func(i, j int) bool {
		a, c := f.subs[i], f.subs[j]
		if ra, rc := a.rank(), c.rank(); ra != rc {
			return ra < rc
		}
		return a.title() < c.title()
	})
	for _, s := range f.subs {
		s.sort()
	}
}

// flatten lists pages in reading order: a folder's own page, its pages,
// then its sub-sections. Previous/next links follow this order.
func (f *folder) flatten(out []*built) []*built {
	if f.index != nil {
		out = append(out, f.index)
	}
	out = append(out, f.pages...)
	out = append(out, f.generated...)
	for _, s := range f.subs {
		out = s.flatten(out)
	}
	return out
}

// sectionsOf returns the folders a page sits in, outermost first, excluding
// the root.
func (b *builder) sectionsOf(p *built) []*folder {
	var out []*folder
	cur := b.tree
	for cur != nil {
		var next *folder
		for _, s := range cur.subs {
			if s.contains(p.Dir) {
				next = s
				break
			}
		}
		if next == nil {
			break
		}
		out = append(out, next)
		cur = next
	}
	return out
}

// nav builds the sidebar: top-level pages, then one collapsible section per
// folder, nested as deep as the folders go. The sections holding the
// current page start open; on the home page, which sits in no section, the
// top-level ones do.
func (b *builder) nav(current *built) []navItem {
	return b.navItems(b.tree, current)
}

func (b *builder) navItems(f *folder, current *built) []navItem {
	home := current.Slug == ""
	var items []navItem
	for _, p := range append(append([]*built{}, f.pages...), f.generated...) {
		items = append(items, navItem{Title: p.Title, Href: b.rel(current.Slug, p.Slug), Active: p == current})
	}
	for _, s := range f.subs {
		it := navItem{Title: s.title(), Section: true, Open: (!home && s.contains(current.Dir)) || (home && f == b.tree)}
		if s.index != nil {
			it.Href = b.rel(current.Slug, s.index.Slug)
			it.Active = s.index == current
		}
		it.Children = b.navItems(s, current)
		items = append(items, it)
	}
	return items
}

// crumbs is the trail of sections above a page, linked where a section has
// its own page.
func (b *builder) crumbs(p *built) []navItem {
	var out []navItem
	for _, s := range b.sectionsOf(p) {
		if s.index == p {
			break
		}
		it := navItem{Title: s.title()}
		if s.index != nil {
			it.Href = b.rel(p.Slug, s.index.Slug)
		}
		out = append(out, it)
	}
	return out
}
