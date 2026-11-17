// Reference route list for a compiled NestJS application.
//
// Usage: node nest_routes.js dist/app.module.js AppModule [globalPrefix]
// Prints one line per route: "METHOD /path". Test-only; indoc never runs code.
require("reflect-metadata");
const path = require("path");
const { NestFactory } = require("@nestjs/core");

(async () => {
  const mod = require(path.resolve(process.argv[2]))[process.argv[3]];
  const app = await NestFactory.create(mod, { logger: false });
  if (process.argv[4]) app.setGlobalPrefix(process.argv[4]);
  await app.init();
  const http = app.getHttpAdapter().getInstance();
  const router = http.router || http._router;
  // @All registers either Express's "_all" or, on Express 5, every HTTP
  // method separately; both are reported as ANY.
  const all = require("http").METHODS.length;
  const byPath = new Map();
  for (const layer of router.stack) {
    if (!layer.route) continue;
    const ms = byPath.get(layer.route.path) || new Set();
    for (const [m, on] of Object.entries(layer.route.methods)) if (on) ms.add(m);
    byPath.set(layer.route.path, ms);
  }
  for (const [p, ms] of byPath) {
    if (ms.has("_all") || ms.size >= all) console.log(`ANY ${p}`);
    else for (const m of ms) console.log(`${m.toUpperCase()} ${p}`);
  }
  await app.close();
})();
