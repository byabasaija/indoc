"""Reference URL list from Django's own resolver.

Run inside the project's environment with its settings module set. Prints one
JSON object per route:

  {"path": "/api/v1/x/", "methods": ["GET", "POST"], "external": false, "static": false}

methods is null when Django cannot know them (plain function views).
external is true when the route comes from a URLconf outside the repository
(admin, allauth, wagtail); static is true for django.views.static.serve.

Test-only. indoc never runs project code.
"""

import importlib.util
import json
import os
import sys
import types

import django

sys.path.insert(0, os.getcwd())
django.setup()

from django.urls import URLPattern, URLResolver, get_resolver  # noqa: E402

HTTP = ["get", "post", "put", "patch", "delete"]


def methods(callback):
    actions = getattr(callback, "actions", None)  # DRF ViewSet
    if actions:
        return sorted(m.upper() for m in actions)
    cls = getattr(callback, "view_class", None) or getattr(callback, "cls", None)
    if cls is not None:
        allowed = getattr(cls, "http_method_names", HTTP)
        return sorted(m.upper() for m in HTTP if m in allowed and hasattr(cls, m))
    return None


ROOT = os.path.realpath(os.getcwd())


def outside(urlconf):
    """True if a resolver's URLconf module lives outside the repository."""
    if isinstance(urlconf, (list, tuple)):
        return False
    if isinstance(urlconf, types.ModuleType):
        path = getattr(urlconf, "__file__", "") or ""
    else:
        try:
            spec = importlib.util.find_spec(str(urlconf))
            path = (spec.origin or "") if spec else ""
        except (ImportError, ValueError):
            path = ""
    path = os.path.realpath(path) if path else ""
    return not path.startswith(ROOT + os.sep) or "site-packages" in path


def module_outside(name):
    try:
        spec = importlib.util.find_spec(name)
    except (ImportError, ValueError):
        return True
    origin = os.path.realpath(spec.origin) if spec and spec.origin else ""
    return not origin.startswith(ROOT + os.sep) or "site-packages" in origin


def leaves(patterns):
    for p in patterns:
        if isinstance(p, URLResolver):
            yield from leaves(p.url_patterns)
        else:
            yield p


def all_outside(patterns):
    """An inline list (admin.site.urls, router.urls) has no module of its own:
    it is external only if every view in it lives outside the repository."""
    mods = [getattr(p.callback, "__module__", "") or "" for p in leaves(patterns)]
    return bool(mods) and all(m and module_outside(m) for m in mods)


def walk(patterns, prefix, external):
    for p in patterns:
        route = prefix + str(p.pattern)
        if isinstance(p, URLResolver):
            if isinstance(p.urlconf_name, (list, tuple)):
                ext = external or all_outside(p.url_patterns)
            else:
                ext = external or outside(p.urlconf_name)
            yield from walk(p.url_patterns, route, ext)
        elif isinstance(p, URLPattern):
            mod = getattr(p.callback, "__module__", "") or ""
            yield {
                "path": "/" + route,
                "methods": methods(p.callback),
                "external": external,
                "static": mod == "django.views.static",
            }


for row in walk(get_resolver().url_patterns, "", False):
    print(json.dumps(row))
