package jsts

import (
	"testing"
)

func TestParseModule(t *testing.T) {
	src := `import express, { Router as R } from "express";
import * as ns from "./x.js";
const { Router, json: parseJSON } = require("express");
const users = require(` + "`./routes/users`" + `);
const app = express();
app.use("/api", users);
router.route("/x").get(h).post((req, res) => {});
module.exports = router;
exports.a = b;
export { app as server };
`
	m := ParseModule("app.js", []byte(src))
	imports := map[string]Import{}
	for _, imp := range m.Imports {
		imports[imp.Local] = imp
	}
	for local, want := range map[string]Import{
		"express":   {Source: "express", Name: "default"},
		"R":         {Source: "express", Name: "Router"},
		"ns":        {Source: "./x.js", Name: "*"},
		"Router":    {Source: "express", Name: "Router"},
		"parseJSON": {Source: "express", Name: "json"},
		"users":     {Source: "./routes/users", Name: "default"},
	} {
		got := imports[local]
		if got.Source != want.Source || got.Name != want.Name {
			t.Errorf("import %s = %+v, want %+v", local, got, want)
		}
	}
	if c, ok := m.Bindings["app"].(*Call); !ok || c.CalleeName() != "express" {
		t.Errorf("app binding = %#v", m.Bindings["app"])
	}
	if m.Exports["default"] != "router" || m.Exports["a"] != "b" || m.Exports["server"] != "app" {
		t.Errorf("exports = %v", m.Exports)
	}
	var chain *Call
	for _, c := range m.Calls {
		if mem, ok := c.Callee.(*Member); ok && mem.Prop == "post" {
			chain = c
		}
	}
	if chain == nil {
		t.Fatal("chained .post call not found")
	}
	get := chain.Callee.(*Member).X.(*Call)
	route := get.Callee.(*Member).X.(*Call)
	if route.CalleeName() != "router.route" || route.Arg(0).(*Str).Value != "/x" {
		t.Errorf("route call = %s", route.Text())
	}
	if _, ok := chain.Arg(0).(*Func); !ok {
		t.Errorf("inline handler = %#v", chain.Arg(0))
	}
}

func TestParseModuleNestDecorators(t *testing.T) {
	src := `@Controller("users")
export class UsersController {
  constructor(private readonly svc: UsersService) {}

  @Get(":id")
  @UseGuards(AuthGuard)
  find(@Param("id") id: string) {}

  @Post()
  create() {}

  helper() {}
}
`
	m := ParseModule("users.controller.ts", []byte(src))
	if len(m.Classes) != 1 {
		t.Fatalf("classes = %+v", m.Classes)
	}
	c := m.Classes[0]
	if c.Name != "UsersController" || len(c.Decorators) != 1 || c.Decorators[0].CalleeName() != "Controller" || !c.Exported {
		t.Errorf("class = %+v", c)
	}
	byName := map[string]*Method{}
	for _, meth := range c.Methods {
		byName[meth.Name] = meth
	}
	if f := byName["find"]; f == nil || len(f.Decorators) != 2 || f.Decorators[0].CalleeName() != "Get" {
		t.Errorf("find = %+v", f)
	}
	if p := byName["create"]; p == nil || len(p.Decorators) != 1 || len(p.Decorators[0].Args) != 0 {
		t.Errorf("create = %+v", p)
	}
	if h := byName["helper"]; h == nil || len(h.Decorators) != 0 {
		t.Errorf("helper = %+v", h)
	}
}
