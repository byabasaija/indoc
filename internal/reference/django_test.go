package reference

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/byabasaija/indoc/internal/code"
	"github.com/byabasaija/indoc/internal/doc"
)

// djangoFixture is a small project exercising each construct the extractor
// resolves. Expected results were checked against Django's resolver on real
// projects (see TestDjangoDifferential).
var djangoFixture = map[string]string{
	"manage.py":    "import os\nos.environ.setdefault('DJANGO_SETTINGS_MODULE', 'config.settings.base')\n",
	".env.example": "DATABASE_URL=postgres://localhost/app\nUNUSED_VAR=1\n",
	"config/settings/base.py": `import environ
env = environ.Env()
DEBUG = env.bool("DJANGO_DEBUG", False)
DATABASES = {"default": env.db()}
SECRET_KEY = env("DJANGO_SECRET_KEY", default="dev-only-not-secret-but-hidden")
BROKER_URL = env("BROKER_URL", default="amqp://guest:hunter2@localhost:5672//")
TIMEOUT = env.int(
    "API_TIMEOUT",
    default=30,
)
ROOT_URLCONF = "config.urls"
ADMIN_URL = "admin/"
`,
	"config/settings/production.py": `from .base import *
ADMIN_URL = env("DJANGO_ADMIN_URL")
`,
	"config/urls.py": `from django.conf import settings
from django.contrib import admin
from django.urls import include, path
from django.views.generic import TemplateView
from app import views

urlpatterns = [
    path("", TemplateView.as_view(template_name="home.html")),
    path(settings.ADMIN_URL, admin.site.urls),
    path("accounts/", include("allauth.urls")),
    path("api/", include("config.api_router")),
    path("app/", include(("app.urls", "app"), namespace="app")),
]
urlpatterns += [path("hook/", views.hook)]

if settings.DEBUG:
    urlpatterns.append(path("debug/", views.debug_page))
`,
	"config/api_router.py": `from rest_framework.routers import DefaultRouter, SimpleRouter
from django.conf import settings
from app.views import UserViewSet, ReadOnlyThing

router = DefaultRouter() if settings.DEBUG else SimpleRouter()
router.register("users", UserViewSet)
router.register(r"things", ReadOnlyThing, basename="thing")
urlpatterns = router.urls
`,
	"app/urls.py": `from django.urls import path, re_path
from . import views

app_name = "app"
urlpatterns = [
    path("items/", views.ItemList.as_view()),
    path("items/<int:pk>/", views.ItemDetail.as_view()),
    path("grant/", views.GrantView.as_view()),
    re_path(r"^legacy/(?P<id>\d+)/$", views.legacy),
    path("items/", views.Shadowed.as_view()),
]
`,
	"app/views.py": `from rest_framework import generics, mixins, viewsets
from rest_framework.decorators import action, api_view
from django.views import View


@api_view(["POST"])
def hook(request):
    pass


def debug_page(request):
    pass


def legacy(request, id):
    pass


class ItemList(generics.ListCreateAPIView):
    pass


class Base(View):
    def get(self, request):
        pass

    def post(self, request):
        pass


class ItemDetail(Base):
    def delete(self, request, pk):
        pass


class GrantView(Base):
    http_method_names = ["post"]


class Shadowed(View):
    def put(self, request):
        pass


class UserViewSet(mixins.ListModelMixin, mixins.RetrieveModelMixin, viewsets.GenericViewSet):
    lookup_field = "username"

    @action(detail=True, methods=["post"], url_path="set-password")
    def set_password(self, request, username=None):
        pass

    @action(detail=False)
    def recent(self, request):
        pass


class ReadOnlyThing(viewsets.ReadOnlyModelViewSet):
    pass
`,
	"app/management/commands/sync_items.py": `from django.core.management.base import BaseCommand


class Command(BaseCommand):
    help = "Sync items from the upstream catalogue."

    def add_arguments(self, parser):
        parser.add_argument("source")
        parser.add_argument("--dry-run", "-n", action="store_true")
`,
	"app/management/commands/_helpers.py": "class Command:\n    pass\n",
	"app/tests/test_views.py":             "from django.urls import path\nurlpatterns = [path('test-only/', None)]\n",
}

func writeFixture(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for p, content := range files {
		full := filepath.Join(root, p)
		os.MkdirAll(filepath.Dir(full), 0o755)
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func section(r *Report, kind string) []doc.RefEntry {
	for _, s := range r.Sections {
		if s.Kind == kind {
			return s.Entries
		}
	}
	return nil
}

func TestDjangoExtraction(t *testing.T) {
	root := writeFixture(t, djangoFixture)
	files, err := code.ListFiles(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	r, err := Extract(root, files)
	if err != nil {
		t.Fatal(err)
	}

	var got []string
	for _, e := range section(r, doc.KindRoutes) {
		line := e.Detail + " " + e.Name + " " + e.Handler
		if strings.HasPrefix(e.Description, "Conditional") {
			line += " (conditional)"
		}
		got = append(got, line)
	}
	want := []string{
		"GET / TemplateView",
		"INCLUDE /accounts/* allauth.urls",
		"INCLUDE /admin/* django.contrib.admin.site.urls",
		"GET /api/ api-root (conditional)",
		"GET /api/things/ ReadOnlyThing",
		"GET /api/things/<pk>/ ReadOnlyThing",
		"GET /api/users/ UserViewSet",
		"GET /api/users/<username>/ UserViewSet",
		"POST /api/users/<username>/set-password/ UserViewSet.set_password",
		"GET /api/users/recent/ UserViewSet.recent",
		"ANY /app/^legacy/(?P<id>\\d+)/$ views.legacy",
		"POST /app/grant/ views.GrantView",
		"GET, POST /app/items/ views.ItemList",
		"PUT /app/items/ views.Shadowed",
		"GET, POST, DELETE /app/items/<int:pk>/ views.ItemDetail",
		"ANY /debug/ views.debug_page (conditional)",
		"POST /hook/ views.hook",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("routes:\n%s\n\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	var cov Coverage
	for _, c := range r.Coverage {
		if c.Extractor == "django" {
			cov = c
		}
	}
	joined := strings.Join(append(cov.Unresolved, cov.Warnings...), "\n")
	for _, want := range []string{
		`route settings.ADMIN_URL resolved to "admin/"; config/settings/production.py sets it to env("DJANGO_ADMIN_URL")`,
		"/accounts/* is provided by allauth.urls",
		"PUT /app/items/ (views.Shadowed, app/urls.py:10) is unreachable: views.ItemList (app/urls.py:6) matches the same path first",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("coverage missing %q in:\n%s", want, joined)
		}
	}

	config := map[string]doc.RefEntry{}
	for _, e := range section(r, doc.KindConfig) {
		config[e.Name] = e
	}
	for name, detail := range map[string]string{
		"DJANGO_DEBUG":      "bool, default False",
		"DATABASE_URL":      "database URL, required",
		"DJANGO_SECRET_KEY": "default (set in code; not shown)",
		"BROKER_URL":        `default "amqp://guest:***@localhost:5672//"`,
		"API_TIMEOUT":       "int, default 30",
		"DJANGO_ADMIN_URL":  "required",
	} {
		if config[name].Detail != detail {
			t.Errorf("%s detail = %q, want %q", name, config[name].Detail, detail)
		}
	}
	if !strings.Contains(config["API_TIMEOUT"].Description, "Not in .env.example") || config["DATABASE_URL"].Description != "" {
		t.Errorf(".env.example cross-check wrong: %+v / %+v", config["API_TIMEOUT"], config["DATABASE_URL"])
	}

	cmds := section(r, doc.KindCommands)
	if len(cmds) != 1 || cmds[0].Name != "sync_items" || cmds[0].Detail != "<source> --dry-run/-n" ||
		cmds[0].Description != "Sync items from the upstream catalogue." {
		t.Errorf("commands = %+v", cmds)
	}
}

func TestDjangoDetectedWithoutURLconfWarns(t *testing.T) {
	root := writeFixture(t, map[string]string{"manage.py": "", "app/models.py": "x = 1\n"})
	files, _ := code.ListFiles(root, nil)
	r, err := Extract(root, files)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Coverage) == 0 || len(r.Coverage[0].Warnings) == 0 ||
		!strings.Contains(r.Coverage[0].Warnings[0], "no literal ROOT_URLCONF") {
		t.Fatalf("want a loud warning, got %+v", r.Coverage)
	}
}

func TestRedactDefault(t *testing.T) {
	cases := map[[2]string]string{
		{"SMS_SERVICE_PASSWORD", `"T3stUs3r!"`}:    "(set in code; not shown)",
		{"STRIPE_API_KEY", `"sk_live_x"`}:          "(set in code; not shown)",
		{"KEYBOARD_LAYOUT", `"us"`}:                `"us"`,
		{"DJANGO_SECRET_KEY", `""`}:                `""`,
		{"DATABASE_URL", `"postgres://u:pw@h/db"`}: `"postgres://u:***@h/db"`,
		{"REDIS_URL", `"redis://localhost:6379"`}:  `"redis://localhost:6379"`,
	}
	for in, want := range cases {
		if got := redactDefault(in[0], in[1]); got != want {
			t.Errorf("redactDefault(%s, %s) = %s, want %s", in[0], in[1], got, want)
		}
	}
}
