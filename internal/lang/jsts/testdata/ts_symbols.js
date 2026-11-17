// Reference symbol dump using the TypeScript compiler API.
//
// Usage: node ts_symbols.js <path-to-typescript-module>  (file paths on stdin)
// Prints one JSON line per file: {"path": ..., "symbols": [[qual, start, end], ...]}
// Lines are 1-based. Test-only; indoc never needs Node.
//
// The symbol set (must match internal/lang/jsts):
//   function declarations with a body                      -> function
//   class declarations; class methods, constructors, accessors with a body -> class, method
//   (methods in object literals are not symbols: they have no stable name)
//   class fields initialized with an arrow/function        -> method
//   variables initialized with an arrow/function/class     -> function, class
//   named class expressions (exports.X = class X {})       -> class
//   interfaces, type aliases, enums, namespaces            -> type
// A declaration spans its whole statement (export, const, decorators).
const ts = require(process.argv[2]);
const fs = require("fs");
const readline = require("readline");

const kinds = { ".ts": ts.ScriptKind.TS, ".tsx": ts.ScriptKind.TSX, ".js": ts.ScriptKind.JS, ".jsx": ts.ScriptKind.JSX,
  ".mjs": ts.ScriptKind.JS, ".cjs": ts.ScriptKind.JS, ".mts": ts.ScriptKind.TS, ".cts": ts.ScriptKind.TS };

function ext(p) { const m = p.match(/(\.d)?(\.[^.\/]+)$/); return m ? m[2] : ""; }

function isFunctionLike(n) {
  return n && (ts.isArrowFunction(n) || ts.isFunctionExpression(n));
}

function dump(path) {
  const text = fs.readFileSync(path, "utf8");
  const sf = ts.createSourceFile(path, text, ts.ScriptTarget.Latest, true, kinds[ext(path)] ?? ts.ScriptKind.TS);
  const out = [];
  const line = (pos) => sf.getLineAndCharacterOfPosition(pos).line + 1;
  const add = (qual, node) => out.push([qual, line(node.getStart(sf)), line(node.getEnd())]);
  const nameOf = (n) => (n.name && (ts.isIdentifier(n.name) || ts.isPrivateIdentifier(n.name) || ts.isStringLiteral(n.name)) ? n.name.text : null);

  function visit(node, scope) {
    let next = scope;
    if (ts.isFunctionDeclaration(node) && node.name && node.body) {
      add([...scope, node.name.text].join("."), node);
      next = [...scope, node.name.text];
    } else if ((ts.isClassDeclaration(node) || ts.isClassExpression(node)) && node.name) {
      add([...scope, node.name.text].join("."), node);
      next = [...scope, node.name.text];
    } else if ((ts.isInterfaceDeclaration(node) || ts.isTypeAliasDeclaration(node) || ts.isEnumDeclaration(node)) && node.name) {
      add([...scope, node.name.text].join("."), node);
      next = [...scope, node.name.text];
    } else if (ts.isModuleDeclaration(node) && ts.isIdentifier(node.name) && node.name.text === "global" && (node.flags & ts.NodeFlags.GlobalAugmentation)) {
      // declare global { ... } declares top-level names; not a namespace.
    } else if (ts.isModuleDeclaration(node) && ts.isIdentifier(node.name)) {
      add([...scope, node.name.text].join("."), node);
      next = [...scope, node.name.text];
    } else if ((ts.isMethodDeclaration(node) || ts.isGetAccessor(node) || ts.isSetAccessor(node)) && node.body && nameOf(node) && ts.isClassLike(node.parent)) {
      add([...scope, nameOf(node)].join("."), node);
      next = [...scope, nameOf(node)];
    } else if (ts.isConstructorDeclaration(node) && node.body) {
      add([...scope, "constructor"].join("."), node);
      next = [...scope, "constructor"];
    } else if (ts.isPropertyDeclaration(node) && isFunctionLike(node.initializer) && nameOf(node)) {
      add([...scope, nameOf(node)].join("."), node);
      next = [...scope, nameOf(node)];
    } else if (ts.isVariableStatement(node)) {
      const decls = node.declarationList.declarations;
      for (const d of decls) {
        if (!ts.isIdentifier(d.name) || !(isFunctionLike(d.initializer) || (d.initializer && ts.isClassExpression(d.initializer)))) {
          if (d.initializer) visit(d.initializer, scope); // nested declarations inside other initializers
          continue;
        }
        const span = decls.length === 1 ? node : d;
        add([...scope, d.name.text].join("."), span);
        if (ts.isClassExpression(d.initializer)) {
          // const A = class B {}: one symbol, named by the variable.
          ts.forEachChild(d.initializer, (c) => visit(c, [...scope, d.name.text]));
        } else {
          visit(d.initializer, [...scope, d.name.text]);
        }
      }
      return;
    }
    ts.forEachChild(node, (c) => visit(c, next));
  }
  visit(sf, []);
  return out;
}

const rl = readline.createInterface({ input: process.stdin });
rl.on("line", (p) => {
  try {
    console.log(JSON.stringify({ path: p, symbols: dump(p) }));
  } catch (e) {
    console.log(JSON.stringify({ path: p, error: String(e) }));
  }
});
