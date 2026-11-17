"""Reference symbol dump using CPython's ast module.

Reads file paths on stdin, writes one JSON object per line:
  {"path": ..., "symbols": [[qual, lineno, end_lineno], ...]}
  {"path": ..., "error": "..."}   when CPython cannot parse the file

Test-only. indoc never needs Python at runtime.
"""

import ast
import json
import sys
import warnings

DEFS = (ast.FunctionDef, ast.AsyncFunctionDef, ast.ClassDef)


def visit(node, scope, out):
    for child in ast.iter_child_nodes(node):
        if isinstance(child, DEFS):
            qual = scope + [child.name]
            out.append([".".join(qual), child.lineno, child.end_lineno])
            visit(child, qual, out)
        else:
            visit(child, scope, out)


def main():
    warnings.simplefilter("ignore")
    for line in sys.stdin:
        path = line.rstrip("\n")
        try:
            with open(path, "rb") as f:
                tree = ast.parse(f.read())
        except (SyntaxError, ValueError, UnicodeDecodeError, RecursionError, MemoryError) as e:
            print(json.dumps({"path": path, "error": type(e).__name__}))
            continue
        out = []
        visit(tree, [], out)
        print(json.dumps({"path": path, "symbols": out}))


if __name__ == "__main__":
    main()
