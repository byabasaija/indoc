package python

import (
	"testing"
)

const urlsSrc = `from django.conf import settings
from django.urls import include, path
from . import views
from .views import (
    HomeView,
    detail as show,
)
import debug_toolbar
import a.b as ab

router = DefaultRouter() if settings.DEBUG else SimpleRouter()
router.register(r"users", views.UserViewSet, basename="user")

urlpatterns = [
    path("", HomeView.as_view(), name="home"),
    path("api/", include(("app.api.urls", "api"), namespace="api")),
    path(settings.ADMIN_URL, admin.site.urls),
    *static(settings.MEDIA_URL, document_root=settings.MEDIA_ROOT),
]
urlpatterns += router.urls

if settings.DEBUG:
    urlpatterns = [path("__debug__/", include(debug_toolbar.urls)), *urlpatterns]


@api_view(["GET", "POST"])
def hook(request):
    x = 1  # not module level
    return x


class UserViewSet(mixins.ListModelMixin, viewsets.GenericViewSet):
    lookup_field = "username"

    @action(detail=True, methods=["post"], url_path="set-password")
    def set_password(self, request, username=None):
        pass

    def list(self, request):
        pass
`

func TestParseModule(t *testing.T) {
	m := ParseModule(urlsSrc)

	imports := map[string]Import{}
	for _, imp := range m.Imports {
		imports[imp.Local] = imp
	}
	for local, want := range map[string]Import{
		"settings":      {Module: "django.conf", Name: "settings"},
		"views":         {Module: "", Name: "views", Level: 1},
		"HomeView":      {Module: "views", Name: "HomeView", Level: 1},
		"show":          {Module: "views", Name: "detail", Level: 1},
		"debug_toolbar": {Module: "debug_toolbar"},
		"ab":            {Module: "a.b"},
	} {
		got := imports[local]
		if got.Module != want.Module || got.Name != want.Name || got.Level != want.Level {
			t.Errorf("import %s = %+v, want %+v", local, got, want)
		}
	}

	var ops []string
	for _, a := range m.Assigns {
		if a.Target == "urlpatterns" {
			ops = append(ops, a.Op)
			if a.Op == "=" && a.Cond {
				seq, ok := a.Value.(*Seq)
				if !ok || len(seq.Elems) != 2 {
					t.Errorf("conditional urlpatterns = %T %s", a.Value, a.Value.Text())
				} else if _, star := seq.Elems[1].(*Starred); !star {
					t.Errorf("want *urlpatterns spread, got %s", seq.Elems[1].Text())
				}
			}
		}
	}
	if len(ops) != 3 || ops[0] != "=" || ops[1] != "+=" || ops[2] != "=" {
		t.Errorf("urlpatterns ops = %v", ops)
	}

	var router Assign
	for _, a := range m.Assigns {
		if a.Target == "router" {
			router = a
		}
	}
	cond, ok := router.Value.(*Cond)
	if !ok || cond.Then.(*Call).FuncName() != "DefaultRouter" || cond.Else.(*Call).FuncName() != "SimpleRouter" {
		t.Errorf("router = %#v", router.Value)
	}

	if len(m.Calls) != 1 || m.Calls[0].Call.FuncName() != "router.register" {
		t.Fatalf("calls = %+v", m.Calls)
	}
	reg := m.Calls[0].Call
	if s, ok := reg.Arg(0, "prefix").(*Str); !ok || s.Value != "users" || !s.Literal {
		t.Errorf("register prefix = %#v", reg.Arg(0, ""))
	}
	if n, ok := reg.Arg(1, "viewset").(*Dotted); !ok || n.Path != "views.UserViewSet" {
		t.Errorf("register viewset = %#v", reg.Arg(1, ""))
	}

	var first *Call
	for _, a := range m.Assigns {
		if a.Target == "urlpatterns" {
			first = a.Value.(*Seq).Elems[1].(*Call)
			break
		}
	}
	inc := first.Arg(1, "view").(*Call)
	if inc.FuncName() != "include" {
		t.Fatalf("include = %s", inc.Text())
	}
	if tuple, ok := inc.Args[0].(*Seq); !ok || !tuple.Tuple || tuple.Elems[0].(*Str).Value != "app.api.urls" {
		t.Errorf("include arg = %s", inc.Args[0].Text())
	}

	hook := m.Funcs["hook"]
	if hook == nil || len(hook.Decorators) != 1 {
		t.Fatalf("hook = %+v", hook)
	}
	if d := hook.Decorators[0].(*Call); d.FuncName() != "api_view" || len(d.Args[0].(*Seq).Elems) != 2 {
		t.Errorf("decorator = %s", hook.Decorators[0].Text())
	}

	vs := m.Classes["UserViewSet"]
	if vs == nil || len(vs.Bases) != 2 || vs.Bases[1].Text() != "viewsets.GenericViewSet" {
		t.Fatalf("class = %+v", vs)
	}
	if vs.Methods["list"] == nil || vs.Methods["set_password"] == nil {
		t.Errorf("methods = %v", vs.Methods)
	}
	if d := vs.Methods["set_password"].Decorators; len(d) != 1 || d[0].(*Call).Kw("url_path").(*Str).Value != "set-password" {
		t.Errorf("action decorator = %v", d)
	}
	if len(vs.Assigns) != 1 || vs.Assigns[0].Target != "lookup_field" {
		t.Errorf("class assigns = %+v", vs.Assigns)
	}
	for _, a := range m.Assigns {
		if a.Target == "x" {
			t.Error("function-body assignment leaked to module level")
		}
	}
}

func TestDecodeString(t *testing.T) {
	cases := []struct {
		tok, want string
		literal   bool
	}{
		{`"plain"`, "plain", true},
		{`r"^users/(?P<pk>\d+)/$"`, `^users/(?P<pk>\d+)/$`, true},
		{`'it\'s'`, "it's", true},
		{`"""triple"""`, "triple", true},
		{`f"{x}/"`, "{x}/", false},
		{`b"bytes"`, "bytes", true},
	}
	for _, c := range cases {
		got, lit := decodeString(c.tok)
		if got != c.want || lit != c.literal {
			t.Errorf("decodeString(%s) = %q %v, want %q %v", c.tok, got, lit, c.want, c.literal)
		}
	}
}

func TestAnnotatedAssignments(t *testing.T) {
	m := ParseModule("class Settings(BaseSettings):\n    database_url: str\n    timeout: int = 30\n    tags: list[str] = Field(default_factory=list)\n")
	c := m.Classes["Settings"]
	if c == nil || len(c.Assigns) != 3 {
		t.Fatalf("assigns = %+v", c)
	}
	a := c.Assigns
	if a[0].Target != "database_url" || a[0].Op != ":" || a[0].Value != nil || a[0].Annotation.Text() != "str" {
		t.Errorf("bare annotation = %+v", a[0])
	}
	if a[1].Op != "=" || a[1].Value.Text() != "30" || a[1].Annotation.Text() != "int" {
		t.Errorf("annotated with value = %+v", a[1])
	}
	if a[2].Annotation.Text() != "list[str]" || a[2].Value.(*Call).FuncName() != "Field" {
		t.Errorf("Field default = %+v", a[2])
	}
}
