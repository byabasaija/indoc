package reference

import (
	"bufio"
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

var fastapiFixture = map[string]string{
	"app/__init__.py": "",
	"app/main.py": `from fastapi import FastAPI
from fastapi.staticfiles import StaticFiles
from app.api import items
from app.api.users import router as users_router
from app.admin import admin_app

app = FastAPI(title="Shop", docs_url="/api-docs", redoc_url=None)


@app.get("/health")
def health():
    return {"ok": True}


app.include_router(users_router, prefix="/v1")
app.include_router(items.router)
app.mount("/admin", admin_app)
app.mount("/static", StaticFiles(directory="static", check_dir=False), name="static")


@app.api_route("/ping", methods=["GET", "POST"])
def ping():
    return "pong"


def legacy():
    return "old"


app.add_api_route("/legacy", legacy, methods=["PUT"])


@app.websocket("/ws")
async def ws(websocket):
    pass
`,
	"app/api/__init__.py": "",
	"app/api/users.py": `from fastapi import APIRouter
from app.api.roles import router as roles_router

router = APIRouter(prefix="/users", tags=["users"])
router.include_router(roles_router, prefix="/{user_id}")


@router.get("")
def list_users():
    return []


@router.post("/")
def create_user():
    return {}


@router.delete("/{user_id}")
def delete_user(user_id: int):
    return {}
`,
	"app/api/roles.py": `from fastapi import APIRouter

router = APIRouter(prefix="/roles")


@router.get("/")
def list_roles(user_id: int):
    return []
`,
	"app/api/items.py": `from fastapi import APIRouter

router = APIRouter()


@router.get("/items/{item_id}")
def get_item(item_id: int):
    return {}


@router.get("/items/{item_id}")
def get_item_again(item_id: int):
    return {}
`,
	"app/admin.py": `from fastapi import FastAPI

admin_app = FastAPI(openapi_url=None)


@admin_app.get("/stats")
def stats():
    return {}
`,
	"app/config.py": `from pydantic import Field
from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    model_config = SettingsConfigDict(env_prefix="SHOP_")

    database_url: str
    debug: bool = False
    api_token: str = "dev-token"
    redis: str = Field("redis://localhost:6379", alias="REDIS_URL")
    _cache: dict = {}
`,
}

func TestFastAPIExtraction(t *testing.T) {
	root := writeFixture(t, fastapiFixture)
	files, _ := code.ListFiles(root, nil)
	r, err := Extract(root, files)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range section(r, doc.KindRoutes) {
		got = append(got, e.Detail+" "+e.Name+" "+e.Handler)
	}
	want := []string{
		"MOUNT /static/* StaticFiles(directory=\"static\", check_dir=False)",
		"GET /admin/stats stats",
		"GET /api-docs FastAPI Swagger UI",
		"GET /docs/oauth2-redirect FastAPI Swagger UI OAuth2 redirect",
		"GET /health health",
		"GET /items/{item_id} get_item",
		"GET /items/{item_id} get_item_again",
		"PUT /legacy legacy",
		"GET /openapi.json FastAPI OpenAPI schema",
		"GET, POST /ping ping",
		"GET /v1/users list_users",
		"POST /v1/users/ create_user",
		"GET /v1/users/{user_id}/roles/ list_roles",
		"DELETE /v1/users/{user_id} delete_user",
		"WS /ws ws",
	}
	sort.Strings(got)
	sort.Strings(want)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("routes:\n%s\n\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	var warnings string
	for _, c := range r.Coverage {
		if c.Extractor == "fastapi" {
			warnings = strings.Join(c.Warnings, "\n")
		}
	}
	if !strings.Contains(warnings, "GET /items/{item_id} (get_item_again") {
		t.Errorf("missing shadowed-route warning: %q", warnings)
	}

	config := map[string]string{}
	for _, e := range section(r, doc.KindConfig) {
		config[e.Name] = e.Detail
	}
	for name, detail := range map[string]string{
		"SHOP_DATABASE_URL": "str, required",
		"SHOP_DEBUG":        "bool, default False",
		"SHOP_API_TOKEN":    "str, default (set in code; not shown)",
		"REDIS_URL":         `str, default "redis://localhost:6379"`,
	} {
		if config[name] != detail {
			t.Errorf("%s = %q, want %q", name, config[name], detail)
		}
	}
	if _, ok := config["SHOP__CACHE"]; ok {
		t.Error("private field became an environment variable")
	}

	if py := os.Getenv("INDOC_FASTAPI_PYTHON"); py != "" {
		compareFastAPI(t, py, root, "app.main:app", r)
	}
}

// TestFastAPIDifferential compares against a real project's route table:
//
//	INDOC_FASTAPI_REPO=~/projects/your-fastapi-app INDOC_FASTAPI_APP=app.main:app \
//	INDOC_FASTAPI_PYTHON=~/projects/your-fastapi-app/.venv/bin/python go test ./internal/reference -run FastAPIDifferential -v
func TestFastAPIDifferential(t *testing.T) {
	repo, app, py := expandHome(os.Getenv("INDOC_FASTAPI_REPO")), os.Getenv("INDOC_FASTAPI_APP"), expandHome(os.Getenv("INDOC_FASTAPI_PYTHON"))
	if repo == "" || app == "" || py == "" {
		t.Skip("set INDOC_FASTAPI_REPO, INDOC_FASTAPI_APP, and INDOC_FASTAPI_PYTHON")
	}
	files, _ := code.ListFiles(repo, nil)
	r, err := Extract(repo, files)
	if err != nil {
		t.Fatal(err)
	}
	compareFastAPI(t, py, repo, app, r)
}

func compareFastAPI(t *testing.T, python, dir, app string, r *Report) {
	t.Helper()
	script, _ := filepath.Abs("testdata/fastapi_routes.py")
	cmd := exec.Command(python, script, app)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("fastapi route table: %v", err)
	}
	truth := map[string]int{}
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	for sc.Scan() {
		var row struct {
			Path    string   `json:"path"`
			Methods []string `json:"methods"`
		}
		if json.Unmarshal(sc.Bytes(), &row) == nil {
			truth[strings.Join(orderedOrAll(row.Methods), ", ")+" "+row.Path]++
		}
	}
	ours := map[string]int{}
	for _, e := range section(r, doc.KindRoutes) {
		ours[e.Detail+" "+e.Name]++
	}
	var diff []string
	for k, n := range truth {
		if ours[k] != n {
			diff = append(diff, "fastapi: "+k)
		}
	}
	for k, n := range ours {
		if truth[k] != n {
			diff = append(diff, "indoc:   "+k)
		}
	}
	sort.Strings(diff)
	t.Logf("fastapi routes: %d, indoc routes: %d", len(truth), len(ours))
	if len(diff) > 0 {
		t.Errorf("route tables differ:\n%s", strings.Join(diff, "\n"))
	}
}

// orderedOrAll orders HTTP methods like indoc and keeps others (WS, MOUNT).
func orderedOrAll(ms []string) []string {
	set := map[string]bool{}
	var other []string
	for _, m := range ms {
		set[m] = true
		if indexOf(httpMethods, m) < 0 {
			other = append(other, m)
		}
	}
	return append(ordered(set), other...)
}
