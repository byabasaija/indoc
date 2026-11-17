// Reference route list for an Express application, recorded at runtime.
//
// Usage: node express_routes.cjs path/to/app.js   (NODE_PATH points at express)
// Prints one line per route: "METHOD /full/path".
//
// It wraps express() and express.Router() as the app loads them and records
// every route registration and use() mount, so it does not depend on either
// Express version's internal router layout. Test-only; indoc never runs code.
const Module = require("module");
const path = require("path");

const METHODS = ["get", "post", "put", "patch", "delete", "options", "head", "all"];
const ids = new Map();
let nextId = 1;
const events = [];
let inRegistration = 0; // Express 4 implements router.get via router.route
const idOf = (o) => (ids.has(o) ? ids.get(o) : (ids.set(o, nextId++), ids.get(o)));

function instrument(r, kind) {
  idOf(r);
  r.__indocKind = kind;
  for (const m of METHODS) {
    const orig = r[m];
    if (typeof orig !== "function") continue;
    r[m] = function (p, ...rest) {
      const settingRead = kind === "app" && m === "get" && arguments.length === 1;
      if (!settingRead && !inRegistration) events.push({ op: "route", id: idOf(r), method: m, paths: [].concat(p).map(String) });
      inRegistration++;
      try {
        return orig.apply(this, arguments);
      } finally {
        inRegistration--;
      }
    };
  }
  const use = r.use;
  r.use = function (...args) {
    let prefixes = ["/"];
    let rest = args;
    if (typeof args[0] === "string" || Array.isArray(args[0])) {
      prefixes = [].concat(args[0]).map(String);
      rest = args.slice(1);
    }
    for (const a of rest.flat(Infinity)) {
      if (a && ids.has(a)) events.push({ op: "mount", id: idOf(r), child: idOf(a), prefixes });
    }
    return use.apply(this, args);
  };
  const route = r.route;
  r.route = function (p) {
    const rt = route.apply(this, arguments);
    for (const m of METHODS) {
      const o = rt[m];
      if (typeof o !== "function") continue;
      rt[m] = function () {
        if (!inRegistration) events.push({ op: "route", id: idOf(r), method: m, paths: [].concat(p).map(String) });
        return o.apply(this, arguments);
      };
    }
    return rt;
  };
  if (kind === "app") r.listen = () => ({ close() {} });
  return r;
}

const realLoad = Module._load;
let wrapped;
Module._load = function (request) {
  const exp = realLoad.apply(this, arguments);
  if (request !== "express") return exp;
  if (!wrapped) {
    wrapped = function () { return instrument(exp.apply(this, arguments), "app"); };
    Object.assign(wrapped, exp);
    wrapped.Router = function () { return instrument(exp.Router.apply(this, arguments), "router"); };
  }
  return wrapped;
};

require(path.resolve(process.argv[2]));

const join = (prefix, p) => {
  if (!prefix || prefix === "/") return p || "/";
  prefix = prefix.replace(/\/$/, "");
  if (!p || p === "/") return prefix;
  return prefix + (p.startsWith("/") ? p : "/" + p);
};
const mounted = new Set(events.filter((e) => e.op === "mount").map((e) => e.child));
const roots = [...new Set(events.map((e) => e.id))].filter((id) => !mounted.has(id) && [...ids.entries()].find(([o, i]) => i === id)[0].__indocKind === "app");
const out = [];
const walk = (id, prefix, seen) => {
  if (seen.has(id)) return;
  seen.add(id);
  for (const e of events) {
    if (e.id !== id) continue;
    if (e.op === "route") for (const p of e.paths) out.push(`${e.method === "all" ? "ANY" : e.method.toUpperCase()} ${join(prefix, p)}`);
    if (e.op === "mount") for (const p of e.prefixes) walk(e.child, join(prefix, p), new Set(seen));
  }
};
for (const r of roots) walk(r, "", new Set());
console.log(out.join("\n"));
process.exit(0);
