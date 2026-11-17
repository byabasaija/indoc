package reference

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/byabasaija/indoc/internal/code"
	"github.com/byabasaija/indoc/internal/doc"
)

var expressFixture = map[string]string{
	"package.json": `{"name": "shop", "dependencies": {"express": "*"}}`,
	"app.js": `const express = require("express");
const users = require("./routes/users");
const sub = require("./sub");
const { health } = require("./handlers");

const app = express();
app.use(express.json());
app.get("/health", health);
app.use("/api/users", users);
app.use(["/v1", "/v2"], require("./routes/items"));
app.use("/sub", sub);
app.route("/books").get(listBooks).post(createBook);
app.all("/ping", (req, res) => res.send("pong"));
app.set("trust proxy", true);
app.get("env");
app.listen(3000);

function listBooks(req, res) {}
function createBook(req, res) {}
`,
	"routes/users.js": `const express = require("express");
const router = require("express").Router();
const settings = express.Router();

router.get("/", list);
router.get("/:id", show);
router.route("/:id/avatar").put(upload).delete(remove);
router.use("/:id/settings", settings);
settings.patch("/", update);

function list() {}
function show() {}
function upload() {}
function remove() {}
function update() {}
module.exports = router;
`,
	"routes/items.js": `const { Router } = require("express");
const r = Router();
r.get("/items", (req, res) => {});
r.post("/items", create);
module.exports = r;
function create() {}
`,
	"sub.js": `const express = require("express");
const sub = express();
sub.get("/status", (req, res) => {});
module.exports = sub;
`,
	"handlers.js": "exports.health = (req, res) => res.send('ok');\n",
	"unused.js": `const { Router } = require("express");
const orphan = Router();
orphan.get("/never", () => {});
`,
}

func TestExpressExtraction(t *testing.T) {
	root := writeFixture(t, expressFixture)
	files, _ := code.ListFiles(root, nil)
	r, err := Extract(root, files)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range section(r, doc.KindRoutes) {
		got = append(got, e.Detail+" "+e.Name)
	}
	want := []string{
		"GET /health",
		"GET /api/users",
		"GET /api/users/:id",
		"PUT /api/users/:id/avatar",
		"DELETE /api/users/:id/avatar",
		"PATCH /api/users/:id/settings",
		"GET /v1/items", "POST /v1/items", "GET /v2/items", "POST /v2/items",
		"GET /sub/status",
		"GET /books", "POST /books",
		"ANY /ping",
	}
	sort.Strings(got)
	sort.Strings(want)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("routes:\n%s\n\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	var cov Coverage
	for _, c := range r.Coverage {
		if c.Extractor == "express" {
			cov = c
		}
	}
	if !strings.Contains(strings.Join(cov.Unresolved, "\n"), "router orphan in unused.js has routes but is never mounted") {
		t.Errorf("missing unmounted-router note: %v", cov.Unresolved)
	}

	// Against Express itself, both major versions:
	//   INDOC_EXPRESS_NODE_PATHS=/path/express4/node_modules:/path/express5/node_modules
	for _, nodePath := range filepath.SplitList(os.Getenv("INDOC_EXPRESS_NODE_PATHS")) {
		script, _ := filepath.Abs("testdata/express_routes.cjs")
		cmd := exec.Command("node", script, filepath.Join(root, "app.js"))
		cmd.Env = append(os.Environ(), "NODE_PATH="+nodePath)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("express tracer (%s): %v\n%s", nodePath, err, out)
		}
		truth := strings.Split(strings.TrimSpace(string(out)), "\n")
		sort.Strings(truth)
		if strings.Join(truth, "\n") != strings.Join(got, "\n") {
			t.Errorf("Express (%s) disagrees:\nexpress:\n%s\n\nindoc:\n%s", nodePath, strings.Join(truth, "\n"), strings.Join(got, "\n"))
		} else {
			t.Logf("matches Express at %s: %d routes", nodePath, len(truth))
		}
	}
}

func TestExpressESMTypeScript(t *testing.T) {
	root := writeFixture(t, map[string]string{
		"src/server.ts": `import express from "express";
import users from "./routes/users.js";
import * as orders from "./routes/orders";
export const app = express();
app.use("/users", users);
app.use("/orders", orders.ordersRouter);
`,
		"src/routes/users.ts": `import { Router } from "express";
const router = Router();
router.get("/:id", async (req, res) => {});
export default router;
`,
		"src/routes/orders.ts": `import express from "express";
export const ordersRouter = express.Router();
ordersRouter.post("/", create);
function create() {}
`,
	})
	files, _ := code.ListFiles(root, nil)
	r, err := Extract(root, files)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range section(r, doc.KindRoutes) {
		got = append(got, e.Detail+" "+e.Name+" "+e.Source)
	}
	want := "GET /users/:id src/routes/users.ts:3\nPOST /orders src/routes/orders.ts:3"
	sort.Strings(got)
	if strings.Join(got, "\n") != want {
		t.Errorf("routes:\n%s\nwant:\n%s", strings.Join(got, "\n"), want)
	}
}
