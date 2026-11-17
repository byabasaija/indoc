package reference

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/byabasaija/indoc/internal/doc"
	"github.com/byabasaija/indoc/internal/lang/python"
)

// django resolves a Django project's URL configuration statically: from
// ROOT_URLCONF through include() and DRF routers to each view, and infers
// HTTP methods from view classes, DRF viewsets, and @api_view.
type django struct {
	py       *pyProject
	settings []string // settings module files
	cov      *Coverage
	routes   []doc.RefEntry
	visiting map[string]bool
	read     map[string]bool
}

// httpMethods in display order.
var httpMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE"}

// knownViews maps framework view classes (by last name segment) to the
// methods they implement. Classes not listed contribute nothing; their
// subclasses' own handler methods still count.
var knownViews = map[string][]string{
	// Django
	"View": {}, "TemplateView": {"GET"}, "ListView": {"GET"}, "DetailView": {"GET"},
	"RedirectView": {"GET", "POST", "PUT", "PATCH", "DELETE"},
	"FormView":     {"GET", "POST", "PUT"}, "CreateView": {"GET", "POST", "PUT"}, "UpdateView": {"GET", "POST", "PUT"},
	"DeleteView": {"GET", "POST", "DELETE"}, "ArchiveIndexView": {"GET"}, "YearArchiveView": {"GET"},
	"MonthArchiveView": {"GET"}, "DayArchiveView": {"GET"}, "TodayArchiveView": {"GET"}, "DateDetailView": {"GET"},
	"LoginView": {"GET", "POST", "PUT"}, "PasswordChangeView": {"GET", "POST", "PUT"},
	"LogoutView": {"POST"}, // Django 5+; GET logout was removed
	// Django REST framework
	"APIView": {}, "GenericAPIView": {},
	"CreateAPIView": {"POST"}, "ListAPIView": {"GET"}, "RetrieveAPIView": {"GET"}, "DestroyAPIView": {"DELETE"},
	"UpdateAPIView": {"PUT", "PATCH"}, "ListCreateAPIView": {"GET", "POST"}, "RetrieveUpdateAPIView": {"GET", "PUT", "PATCH"},
	"RetrieveDestroyAPIView": {"GET", "DELETE"}, "RetrieveUpdateDestroyAPIView": {"GET", "PUT", "PATCH", "DELETE"},
	// Common third-party views
	"TokenObtainPairView": {"POST"}, "TokenRefreshView": {"POST"}, "TokenVerifyView": {"POST"}, "TokenBlacklistView": {"POST"},
	"SpectacularAPIView": {"GET"}, "SpectacularSwaggerView": {"GET"}, "SpectacularRedocView": {"GET"},
}

// viewsetActions maps DRF viewset base classes and mixins to the actions they provide.
var viewsetActions = map[string][]string{
	"ModelViewSet":         {"list", "create", "retrieve", "update", "partial_update", "destroy"},
	"ReadOnlyModelViewSet": {"list", "retrieve"},
	"ListModelMixin":       {"list"}, "CreateModelMixin": {"create"}, "RetrieveModelMixin": {"retrieve"},
	"UpdateModelMixin": {"update", "partial_update"}, "DestroyModelMixin": {"destroy"},
	"GenericViewSet": {}, "ViewSet": {}, "ViewSetMixin": {},
}

var listActions = map[string]string{"list": "GET", "create": "POST"}
var detailActions = map[string]string{"retrieve": "GET", "update": "PUT", "partial_update": "PATCH", "destroy": "DELETE"}

func extractDjango(root string, files []string) ([]doc.RefEntry, *Coverage) {
	cov := &Coverage{Extractor: "django"}
	var pyFiles []string
	for _, f := range files {
		if strings.HasSuffix(f, ".py") {
			pyFiles = append(pyFiles, f)
		}
		if path.Base(f) == "manage.py" {
			cov.Evidence = append(cov.Evidence, f)
		}
	}
	py := newPyProject(root, pyFiles)

	var urlconfs []string
	seen := map[string]bool{}
	for _, f := range pyFiles {
		if !strings.Contains(path.Base(f), "settings") && !strings.Contains(f, "/settings/") {
			continue
		}
		for _, a := range py.load(f).Assigns {
			if a.Target != "ROOT_URLCONF" {
				continue
			}
			if s, ok := a.Value.(*python.Str); ok && s.Literal && !seen[s.Value] {
				seen[s.Value] = true
				urlconfs = append(urlconfs, s.Value)
				cov.Evidence = append(cov.Evidence, fmt.Sprintf("ROOT_URLCONF = %q (%s:%d)", s.Value, f, a.Line))
			}
		}
	}
	if len(cov.Evidence) == 0 {
		return nil, nil // not a Django project
	}
	if len(urlconfs) == 0 {
		cov.Warnings = append(cov.Warnings, "Django project detected but no literal ROOT_URLCONF found in settings; routes not extracted")
		return nil, cov
	}

	d := &django{py: py, cov: cov, visiting: map[string]bool{}, read: map[string]bool{}}
	for _, f := range pyFiles {
		if strings.Contains(f, "/settings/") || path.Base(f) == "settings.py" {
			d.settings = append(d.settings, f)
		}
	}
	for _, mod := range urlconfs {
		file, ok := py.moduleFile(mod)
		if !ok {
			cov.Warnings = append(cov.Warnings, fmt.Sprintf("ROOT_URLCONF %q does not resolve to a file in the repository", mod))
			continue
		}
		d.walkFile(file, "", false)
	}
	d.shadowed()
	cov.Files = len(d.read)
	cov.Entries = len(d.routes)
	if len(d.routes) == 0 {
		cov.Warnings = append(cov.Warnings, "Django project detected but no routes were extracted")
	}
	return d.routes, cov
}

func (d *django) note(file string, line int, format string, args ...any) {
	d.cov.Unresolved = append(d.cov.Unresolved, fmt.Sprintf("%s:%d: %s", file, line, fmt.Sprintf(format, args...)))
}

// element is one item of a urlpatterns list.
type element struct {
	expr python.Expr
	file string
	line int
	cond bool
}

// walkFile adds every route reachable from a URLconf module.
func (d *django) walkFile(file, prefix string, cond bool) {
	if d.visiting[file] {
		return
	}
	d.visiting[file] = true
	defer delete(d.visiting, file)
	d.read[file] = true

	m := d.py.load(file)
	type op struct {
		line   int
		assign *python.Assign
		call   *python.CallStmt
	}
	var ops []op
	for i := range m.Assigns {
		if m.Assigns[i].Target == "urlpatterns" {
			ops = append(ops, op{line: m.Assigns[i].Line, assign: &m.Assigns[i]})
		}
	}
	for i := range m.Calls {
		switch m.Calls[i].Call.FuncName() {
		case "urlpatterns.append", "urlpatterns.extend", "urlpatterns.insert":
			ops = append(ops, op{line: m.Calls[i].Line, call: &m.Calls[i]})
		}
	}
	sort.SliceStable(ops, func(i, j int) bool { return ops[i].line < ops[j].line })

	var patterns []element
	found := false
	for _, o := range ops {
		found = true
		if a := o.assign; a != nil {
			elems := d.expand(file, a.Value, patterns, a.Line, a.Cond)
			if a.Op == "+=" {
				patterns = append(patterns, elems...)
			} else {
				patterns = elems
			}
			continue
		}
		c := o.call
		switch c.Call.FuncName() {
		case "urlpatterns.append":
			if x := c.Call.Arg(0, ""); x != nil {
				patterns = append(patterns, element{expr: x, file: file, line: c.Line, cond: c.Cond})
			}
		case "urlpatterns.extend":
			if x := c.Call.Arg(0, ""); x != nil {
				patterns = append(patterns, d.expand(file, x, patterns, c.Line, c.Cond)...)
			}
		case "urlpatterns.insert":
			if x := c.Call.Arg(1, ""); x != nil {
				patterns = append([]element{{expr: x, file: file, line: c.Line, cond: c.Cond}}, patterns...)
			}
		}
	}
	if !found {
		d.note(file, 1, "no urlpatterns assignment")
	}
	for _, e := range patterns {
		d.element(e, prefix, cond || e.cond)
	}
}

// expand evaluates a urlpatterns expression into elements.
func (d *django) expand(file string, x python.Expr, current []element, line int, cond bool) []element {
	switch v := x.(type) {
	case *python.Seq:
		var out []element
		for _, el := range v.Elems {
			if s, ok := el.(*python.Starred); ok {
				if n, ok := s.X.(*python.Dotted); ok && n.Path == "urlpatterns" {
					out = append(out, current...)
					continue
				}
				out = append(out, d.expand(file, s.X, nil, line, cond)...)
				continue
			}
			out = append(out, element{expr: el, file: file, line: line, cond: cond})
		}
		return out
	case *python.BinOp:
		if v.Op == "+" {
			return append(d.expand(file, v.L, current, line, cond), d.expand(file, v.R, current, line, cond)...)
		}
	case *python.Dotted:
		if v.Path == "urlpatterns" {
			return current
		}
		if strings.HasSuffix(v.Path, ".urls") {
			return []element{{expr: v, file: file, line: line, cond: cond}} // router.urls, resolved later
		}
		r := d.py.lookup(file, v.Path)
		if r.kind == "assign" {
			for _, a := range d.py.load(r.file).Assigns {
				if a.Target == r.name {
					return d.expand(r.file, a.Value, nil, a.Line, cond || a.Cond)
				}
			}
		}
	case *python.Call:
		name := v.FuncName()
		switch {
		case strings.HasSuffix(name, "format_suffix_patterns") && len(v.Args) > 0:
			return d.expand(file, v.Args[0], current, line, cond)
		case name == "static" || strings.HasSuffix(name, ".static"):
			return nil // media/static serving, empty unless DEBUG
		}
	}
	d.note(file, line, "urlpatterns item not resolved: %s", short(x.Text()))
	return nil
}

func short(s string) string {
	if len(s) > 80 {
		return s[:77] + "..."
	}
	return s
}

// element handles one urlpatterns item.
func (d *django) element(e element, prefix string, cond bool) {
	if n, ok := e.expr.(*python.Dotted); ok && strings.HasSuffix(n.Path, ".urls") {
		d.include(e.file, e.line, prefix, e.expr, cond)
		return
	}
	call, ok := e.expr.(*python.Call)
	if !ok {
		d.note(e.file, e.line, "urlpatterns item not resolved: %s", short(e.expr.Text()))
		return
	}
	fn := call.FuncName()
	last := fn[strings.LastIndex(fn, ".")+1:]
	if last != "path" && last != "re_path" && last != "url" {
		d.note(e.file, call.Line, "urlpatterns item not resolved: %s", short(call.Text()))
		return
	}
	route, overridden := "", ""
	switch r := call.Arg(0, "route").(type) {
	case *python.Str:
		route = r.Value
		if !r.Literal {
			d.note(e.file, call.Line, "route %s is not a literal", r.Text())
		}
	case nil:
		d.note(e.file, call.Line, "%s() without a route", fn)
		return
	default:
		route = "{" + r.Text() + "}"
		if v, why, ok := d.setting(r); ok {
			route = v
			if why != "" {
				d.note(e.file, call.Line, "route %s resolved to %q; %s", r.Text(), v, why)
				overridden = why
			}
		} else {
			d.note(e.file, call.Line, "route %s is not a literal", r.Text())
		}
	}
	full := prefix + route
	view := call.Arg(1, "view")
	if view == nil {
		d.note(e.file, call.Line, "%s() without a view", fn)
		return
	}
	before := len(d.routes)
	switch v := view.(type) {
	case *python.Call:
		if lastSeg(v.FuncName()) == "include" {
			d.include(e.file, call.Line, full, v.Arg(0, "arg"), cond)
			break
		}
		methods, handler := d.viewMethods(e.file, view, 0)
		d.add(full, methods, handler, e.file, call.Line, cond)
	case *python.Dotted:
		if strings.HasSuffix(v.Path, ".urls") {
			d.include(e.file, call.Line, full, v, cond)
			break
		}
		methods, handler := d.viewMethods(e.file, view, 0)
		d.add(full, methods, handler, e.file, call.Line, cond)
	default:
		methods, handler := d.viewMethods(e.file, view, 0)
		d.add(full, methods, handler, e.file, call.Line, cond)
	}
	if overridden != "" {
		for i := before; i < len(d.routes); i++ {
			d.routes[i].Description = strings.TrimSpace(d.routes[i].Description + " Path prefix varies: " + overridden + ".")
		}
	}
}

// setting resolves `settings.NAME` to the literal assigned in the settings
// modules. why is non-empty when other settings files assign it differently.
func (d *django) setting(x python.Expr) (value, why string, ok bool) {
	n, isName := x.(*python.Dotted)
	if !isName || !strings.HasPrefix(n.Path, "settings.") {
		return "", "", false
	}
	name := strings.TrimPrefix(n.Path, "settings.")
	var literal, literalFile string
	var others []string
	for _, f := range d.settings {
		for _, a := range d.py.load(f).Assigns {
			if a.Target != name {
				continue
			}
			if s, isStr := a.Value.(*python.Str); isStr && s.Literal && literal == "" {
				literal, literalFile = s.Value, f
				continue
			}
			others = append(others, fmt.Sprintf("%s sets it to %s", f, short(a.Value.Text())))
		}
	}
	if literalFile == "" {
		return "", "", false
	}
	return literal, strings.Join(others, "; "), true
}

func lastSeg(s string) string { return s[strings.LastIndex(s, ".")+1:] }

// include follows include(target) with the accumulated prefix.
func (d *django) include(file string, line int, prefix string, target python.Expr, cond bool) {
	if seq, ok := target.(*python.Seq); ok && seq.Tuple && len(seq.Elems) > 0 {
		target = seq.Elems[0] // include(("module.urls", "namespace"))
	}
	switch t := target.(type) {
	case *python.Str:
		if f, ok := d.py.moduleFile(t.Value); ok {
			d.walkFile(f, prefix, cond)
			return
		}
		d.external(prefix, t.Value, file, line, cond)
	case *python.Seq:
		for _, el := range d.expand(file, t, nil, line, cond) {
			d.element(el, prefix, cond)
		}
	case *python.Dotted:
		if strings.HasSuffix(t.Path, ".urls") {
			base := strings.TrimSuffix(t.Path, ".urls")
			r := d.py.lookup(file, base)
			switch {
			case r.kind == "assign" && d.router(r.file, r.name, prefix, cond):
				return
			case r.kind == "module":
				d.walkFile(r.file, prefix, cond)
				return
			case r.kind == "external" || r.kind == "unknown" && r.external != "":
				d.external(prefix, r.external+".urls", file, line, cond)
				return
			case r.kind == "unknown" && strings.Contains(base, "."):
				d.external(prefix, t.Path, file, line, cond)
				return
			}
		}
		r := d.py.lookup(file, t.Path)
		switch r.kind {
		case "module":
			d.walkFile(r.file, prefix, cond)
		case "assign":
			for _, a := range d.py.load(r.file).Assigns {
				if a.Target == r.name {
					for _, el := range d.expand(r.file, a.Value, nil, a.Line, cond) {
						d.element(el, prefix, cond)
					}
					return
				}
			}
		case "external":
			d.external(prefix, r.external, file, line, cond)
		default:
			d.note(file, line, "include(%s) not resolved", t.Path)
		}
	default:
		if target == nil {
			d.note(file, line, "include() without a target")
			return
		}
		d.note(file, line, "include(%s) not resolved", short(target.Text()))
	}
}

func (d *django) external(prefix, target, file string, line int, cond bool) {
	d.cov.Unresolved = append(d.cov.Unresolved, fmt.Sprintf("%s:%d: /%s* is provided by %s (outside the repository)", file, line, prefix, target))
	d.routes = append(d.routes, doc.RefEntry{
		Name:    "/" + prefix + "*",
		Detail:  "INCLUDE",
		Handler: target,
		Source:  fmt.Sprintf("%s:%d", file, line),
		Description: map[bool]string{
			true:  "Conditional (inside an if block). Routes provided by " + target + ".",
			false: "Routes provided by " + target + ".",
		}[cond],
	})
}

func (d *django) add(route string, methods []string, handler, file string, line int, cond bool) {
	detail := "ANY"
	if len(methods) > 0 {
		detail = strings.Join(methods, ", ")
	}
	e := doc.RefEntry{Name: "/" + route, Detail: detail, Handler: handler, Source: fmt.Sprintf("%s:%d", file, line)}
	if cond {
		e.Description = "Conditional (inside an if block)."
	}
	d.routes = append(d.routes, e)
}

// viewMethods infers the HTTP methods a view accepts; nil means unknown.
func (d *django) viewMethods(file string, view python.Expr, depth int) ([]string, string) {
	if depth > 3 {
		return nil, view.Text()
	}
	switch v := view.(type) {
	case *python.Call:
		fn := v.FuncName()
		if strings.HasSuffix(fn, ".as_view") {
			cls := strings.TrimSuffix(fn, ".as_view")
			methods, _ := d.classMethods(file, cls, 0)
			return methods, cls
		}
		if len(v.Args) > 0 { // decorator wrapper: csrf_exempt(view), login_required(view)
			return d.viewMethods(file, v.Args[0], depth+1)
		}
	case *python.Dotted:
		r := d.py.lookup(file, v.Path)
		if r.kind == "func" {
			fn := d.py.load(r.file).Funcs[r.name]
			for _, dec := range fn.Decorators {
				call, ok := dec.(*python.Call)
				if ok && lastSeg(call.FuncName()) == "api_view" {
					if seq, ok := call.Arg(0, "http_method_names").(*python.Seq); ok {
						return upperStrings(seq), v.Path
					}
					return []string{"GET"}, v.Path
				}
				if n, ok := dec.(*python.Dotted); ok && lastSeg(n.Path) == "api_view" {
					return []string{"GET"}, v.Path
				}
			}
		}
		return nil, v.Path
	}
	return nil, view.Text()
}

func upperStrings(seq *python.Seq) []string {
	set := map[string]bool{}
	for _, e := range seq.Elems {
		if s, ok := e.(*python.Str); ok {
			set[strings.ToUpper(s.Value)] = true
		}
	}
	return ordered(set)
}

func ordered(set map[string]bool) []string {
	var out []string
	for _, m := range httpMethods {
		if set[m] {
			out = append(out, m)
		}
	}
	return out
}

// classMethods returns the HTTP methods a class-based view handles, following
// project base classes and known framework bases.
func (d *django) classMethods(file, name string, depth int) ([]string, bool) {
	set := map[string]bool{}
	known := d.collectMethods(file, name, set, depth)
	if allowed := d.allowedMethods(file, name, 0); allowed != nil {
		for m := range set {
			if !allowed[m] {
				delete(set, m)
			}
		}
	}
	return ordered(set), known
}

// allowedMethods returns the nearest http_method_names restriction, or nil.
func (d *django) allowedMethods(file, name string, depth int) map[string]bool {
	if depth > 6 {
		return nil
	}
	r := d.py.lookup(file, name)
	if r.kind != "class" {
		return nil
	}
	cls := d.py.load(r.file).Classes[r.name]
	for _, a := range cls.Assigns {
		if seq, ok := a.Value.(*python.Seq); ok && a.Target == "http_method_names" {
			set := map[string]bool{}
			for _, m := range upperStrings(seq) {
				set[m] = true
			}
			return set
		}
	}
	for _, b := range cls.Bases {
		if n, ok := b.(*python.Dotted); ok {
			if set := d.allowedMethods(r.file, n.Path, depth+1); set != nil {
				return set
			}
		}
	}
	return nil
}

func (d *django) collectMethods(file, name string, set map[string]bool, depth int) bool {
	if depth > 6 {
		return false
	}
	r := d.py.lookup(file, name)
	if r.kind != "class" {
		if ms, ok := knownViews[lastSeg(name)]; ok {
			for _, m := range ms {
				set[m] = true
			}
			return true
		}
		return false
	}
	cls := d.py.load(r.file).Classes[r.name]
	for _, m := range httpMethods {
		if cls.Methods[strings.ToLower(m)] != nil {
			set[m] = true
		}
	}
	known := true
	for _, b := range cls.Bases {
		if n, ok := b.(*python.Dotted); ok {
			known = d.collectMethods(r.file, n.Path, set, depth+1) && known
		}
	}
	return known
}

// router emits routes for a DRF router variable; false if it is not a router.
func (d *django) router(file, variable, prefix string, cond bool) bool {
	m := d.py.load(file)
	var ctor *python.Call
	chosen := false
	for _, a := range m.Assigns {
		if a.Target != variable {
			continue
		}
		v := a.Value
		if c, ok := v.(*python.Cond); ok {
			chosen = true
			// `DefaultRouter() if settings.DEBUG else SimpleRouter()`: document production.
			v = c.Then
			if strings.Contains(c.Test.Text(), "DEBUG") {
				v = c.Else
			}
		}
		if call, ok := v.(*python.Call); ok && strings.HasSuffix(call.FuncName(), "Router") {
			ctor = call
		}
	}
	if ctor == nil {
		return false
	}
	d.read[file] = true
	slash := "/"
	if ts, ok := ctor.Kw("trailing_slash").(*python.Dotted); ok && ts.Path == "False" {
		slash = ""
	}
	for _, c := range m.Calls {
		if c.Call.FuncName() != variable+".register" {
			continue
		}
		p, ok := c.Call.Arg(0, "prefix").(*python.Str)
		if !ok {
			d.note(file, c.Line, "router prefix %s is not a literal", c.Call.Arg(0, "prefix"))
			continue
		}
		vs, ok := c.Call.Arg(1, "viewset").(*python.Dotted)
		if !ok {
			d.note(file, c.Line, "viewset for %q not resolved", p.Value)
			continue
		}
		d.viewset(file, c.Line, prefix, p.Value, slash, vs.Path, cond || c.Cond)
	}
	if lastSeg(ctor.FuncName()) == "DefaultRouter" || chosen {
		// DefaultRouter appends the browsable API root after the registered
		// routes. When the router class depends on a condition, so does the root.
		d.add(prefix, []string{"GET"}, "api-root", file, ctor.Line, cond || chosen)
	}
	return true
}

func (d *django) viewset(file string, line int, prefix, regPrefix, slash, name string, cond bool) {
	// DRF joins prefix and routes with "/", dropping the leading "/" when the
	// registered prefix is empty.
	base := prefix + regPrefix
	sep := "/"
	if regPrefix == "" {
		sep = ""
	}
	actions := map[string]bool{}
	var extras []*python.Func
	lookup := "pk"
	known := d.collectActions(file, name, actions, &extras, &lookup, 0)
	if !known {
		d.note(file, line, "viewset %s has bases outside the repository; actions may be incomplete", name)
	}
	listM, detailM := map[string]bool{}, map[string]bool{}
	for a := range actions {
		if m, ok := listActions[a]; ok {
			listM[m] = true
		}
		if m, ok := detailActions[a]; ok {
			detailM[m] = true
		}
	}
	src := fmt.Sprintf("%s:%d", file, line)
	listPath := base + slash
	if regPrefix == "" {
		listPath = prefix
	}
	if ms := ordered(listM); len(ms) > 0 {
		d.add(listPath, ms, name, file, line, cond)
	}
	if ms := ordered(detailM); len(ms) > 0 {
		d.add(base+sep+"<"+lookup+">"+slash, ms, name, file, line, cond)
	}
	sort.Slice(extras, func(i, j int) bool { return extras[i].Name < extras[j].Name })
	for _, fn := range extras {
		call := actionDecorator(fn)
		urlPath := fn.Name
		if s, ok := call.Kw("url_path").(*python.Str); ok {
			urlPath = s.Value
		}
		methods := []string{"GET"}
		if seq, ok := call.Kw("methods").(*python.Seq); ok {
			methods = upperStrings(seq)
		}
		route := base + sep + urlPath + slash
		if detail, ok := call.Kw("detail").(*python.Dotted); ok && detail.Path == "True" {
			route = base + sep + "<" + lookup + ">/" + urlPath + slash
		}
		d.routes = append(d.routes, doc.RefEntry{Name: "/" + route, Detail: strings.Join(methods, ", "), Handler: name + "." + fn.Name, Source: src})
	}
}

func actionDecorator(fn *python.Func) *python.Call {
	for _, dec := range fn.Decorators {
		if c, ok := dec.(*python.Call); ok && lastSeg(c.FuncName()) == "action" {
			return c
		}
	}
	return nil
}

func (d *django) collectActions(file, name string, actions map[string]bool, extras *[]*python.Func, lookup *string, depth int) bool {
	if depth > 6 {
		return false
	}
	r := d.py.lookup(file, name)
	if r.kind != "class" {
		as, ok := viewsetActions[lastSeg(name)]
		for _, a := range as {
			actions[a] = true
		}
		return ok
	}
	cls := d.py.load(r.file).Classes[r.name]
	for mname, fn := range cls.Methods {
		if _, ok := listActions[mname]; ok {
			actions[mname] = true
		}
		if _, ok := detailActions[mname]; ok {
			actions[mname] = true
		}
		if actionDecorator(fn) != nil && depth == 0 {
			*extras = append(*extras, fn)
		}
	}
	for _, a := range cls.Assigns {
		if s, ok := a.Value.(*python.Str); ok && (a.Target == "lookup_url_kwarg" || a.Target == "lookup_field" && *lookup == "pk") {
			*lookup = s.Value
		}
	}
	known := true
	for _, b := range cls.Bases {
		if n, ok := b.(*python.Dotted); ok {
			known = d.collectActions(r.file, n.Path, actions, extras, lookup, depth+1) && known
		}
	}
	return known
}

// shadowed warns about routes Django can never reach because an earlier
// pattern has the same path. Routes are still listed; the warning is the point.
func (d *django) shadowed() {
	first := map[string]doc.RefEntry{}
	for _, r := range d.routes {
		if r.Detail == "INCLUDE" || strings.HasPrefix(r.Description, "Conditional") {
			continue
		}
		if prev, ok := first[r.Name]; ok {
			if prev.Handler != r.Handler {
				d.cov.Warnings = append(d.cov.Warnings, fmt.Sprintf(
					"%s %s (%s, %s) is unreachable: %s (%s) matches the same path first",
					r.Detail, r.Name, r.Handler, r.Source, prev.Handler, prev.Source))
			}
			continue
		}
		first[r.Name] = r
	}
}
