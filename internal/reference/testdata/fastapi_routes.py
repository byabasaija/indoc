"""Reference route list from a FastAPI application's own route table.

Usage: python fastapi_routes.py module:attribute   (run from the project root)
Prints one JSON object per route: {"path": "/x", "methods": ["GET"]}.
Mounted non-FastAPI apps print {"path": "/static/*", "methods": ["MOUNT"]}.

Test-only. indoc never runs project code.
"""

import importlib
import json
import os
import sys

sys.path.insert(0, os.getcwd())
module, attr = sys.argv[1].split(":")
app = getattr(importlib.import_module(module), attr)


def row(route, path):
    kind = type(route).__name__
    if kind in ("APIWebSocketRoute", "WebSocketRoute"):
        return {"path": path, "methods": ["WS"]}
    ms = getattr(route, "methods", None) or []
    return {"path": path, "methods": sorted(m for m in ms if m != "HEAD" or len(ms) == 1)}


def walk(routes, prefix):
    for r in routes:
        # FastAPI >= 0.16x keeps included routers as _IncludedRouter entries
        # whose effective routes (with full paths) are computed on demand.
        if hasattr(r, "effective_candidates"):
            for ctx in r.effective_candidates():
                if hasattr(ctx, "effective_candidates"):
                    yield from walk([ctx], prefix)
                else:
                    yield row(ctx.original_route, prefix + ctx.path)
            continue
        kind = type(r).__name__
        path = prefix + getattr(r, "path", "")
        if kind == "Mount":
            sub = getattr(r.app, "routes", None)
            if sub is not None and type(r.app).__name__ == "FastAPI":
                yield from walk(sub, path)
            else:
                yield {"path": path + "/*", "methods": ["MOUNT"]}
        else:
            yield row(r, path)


for item in walk(app.routes, ""):
    print(json.dumps(item))
