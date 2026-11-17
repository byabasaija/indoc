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

var nestFixture = map[string]string{
	"package.json": `{"name": "api", "dependencies": {"@nestjs/core": "*"}}`,
	"tsconfig.json": `{"compilerOptions": {"module": "commonjs", "target": "es2021", "experimentalDecorators": true,
  "emitDecoratorMetadata": true, "outDir": "dist", "rootDir": "src", "skipLibCheck": true}}`,
	"src/main.ts": `import { NestFactory } from "@nestjs/core";
import { AppModule } from "./app.module";
async function bootstrap() {
  const app = await NestFactory.create(AppModule);
  app.setGlobalPrefix("api");
  await app.listen(3000);
}
bootstrap();
`,
	"src/app.module.ts": `import { Module } from "@nestjs/common";
import { UsersController } from "./users/users.controller";
import { HealthController } from "./health.controller";
@Module({ controllers: [UsersController, HealthController] })
export class AppModule {}
`,
	"src/users/users.controller.ts": `import { Controller, Get, Post, Patch, Delete, All, Param, Body } from "@nestjs/common";

@Controller("users")
export class UsersController {
  @Get()
  list() { return []; }

  @Get(":id")
  find(@Param("id") id: string) { return { id }; }

  @Post()
  create(@Body() body: unknown) { return body; }

  @Patch([":id", ":id/profile"])
  update() { return {}; }

  @Delete(":id")
  remove() {}

  @All("ping")
  ping() { return "pong"; }

  helper() {}
}
`,
	"src/health.controller.ts": `import { Controller, Get } from "@nestjs/common";
@Controller({ path: "health" })
export class HealthController {
  @Get()
  check() { return "ok"; }
}
`,
}

func TestNestExtraction(t *testing.T) {
	root := writeFixture(t, nestFixture)
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
		"GET /api/users", "GET /api/users/:id", "POST /api/users",
		"PATCH /api/users/:id", "PATCH /api/users/:id/profile", "DELETE /api/users/:id",
		"ANY /api/users/ping", "GET /api/health",
	}
	sort.Strings(got)
	sort.Strings(want)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("routes:\n%s\n\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	// Against Nest itself: INDOC_NEST_ENV=<dir with node_modules/@nestjs>.
	env := os.Getenv("INDOC_NEST_ENV")
	if env == "" {
		return
	}
	os.Symlink(filepath.Join(env, "node_modules"), filepath.Join(root, "node_modules"))
	tsc := exec.Command("node", filepath.Join(root, "node_modules/typescript/bin/tsc"), "-p", root)
	if out, err := tsc.CombinedOutput(); err != nil {
		t.Fatalf("tsc: %v\n%s", err, out)
	}
	script, _ := filepath.Abs("testdata/nest_routes.js")
	run := exec.Command("node", script, filepath.Join(root, "dist/app.module.js"), "AppModule", "api")
	run.Dir = root
	run.Env = append(os.Environ(), "NODE_PATH="+filepath.Join(root, "node_modules"))
	out, err := run.CombinedOutput()
	if err != nil {
		t.Fatalf("nest: %v\n%s", err, out)
	}
	truth := strings.Split(strings.TrimSpace(string(out)), "\n")
	sort.Strings(truth)
	if strings.Join(truth, "\n") != strings.Join(got, "\n") {
		t.Errorf("Nest disagrees:\nnest:\n%s\n\nindoc:\n%s", strings.Join(truth, "\n"), strings.Join(got, "\n"))
	} else {
		t.Logf("matches Nest: %d routes", len(truth))
	}
}
