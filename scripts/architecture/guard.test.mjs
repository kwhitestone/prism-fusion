import assert from "node:assert/strict";
import { createRequire } from "node:module";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import { mkdtemp, mkdir, writeFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import { checkRepository, inspectGoFeatures, inspectInventory, inspectTypeScript } from "./guard.mjs";

const require = createRequire(new URL("../../src/web/package.json", import.meta.url));
const ts = require("typescript");
const policy = {
  serverRoot: "src/server",
  frontendRoot: "src/web/src",
  coreFiles: ["src/server/main.go", "src/server/model/common.go", "src/web/src/main.ts", "src/web/src/router/index.ts"],
  composition: { "src/server/main.go": ["github.com/kwhitestone/prism-fusion/addons"] },
  frontendComposition: ["src/web/src/main.ts"],
  discovery: { "src/web/src/plugin/loader.ts": ["../addons/*/index.ts"] },
  routeMutators: { "src/web/src/router/index.ts": ["createRouter", "resetRouter"] },
  routePaths: { "src/web/src/router/index.ts": ["/login"] },
  apiURLs: { "src/web/src/plugin/loader.ts": ["/api/v1/system/plugin-registry"] },
  stores: { "src/web/src/store/modules/user.ts": ["pure-user"] },
  goRouteCalls: { "src/server/initialize/router.go": ["Routers:GET"] },
  goRoutePaths: { "src/server/initialize/router.go": ["/health"] },
  goDatabaseCalls: { "src/server/initialize/tables.go": ["migratePlugin:AutoMigrate"] },
  goDBAccess: ["src/server/main.go"],
  goModelTypes: { "src/server/model/common.go": ["MODEL"] },
  emptyGoTypes: { "src/server/api/v1/enter.go": ["ApiGroup"] }
};

const codes = findings => findings.map(finding => finding.rule);
const frontend = (source, file = "src/web/src/router/index.ts") =>
  inspectTypeScript(file, source, policy, ts);
const backend = (features, file = "src/server/core/server.go") =>
  inspectGoFeatures({ file, imports: [], calls: [], types: [], selectors: [], routePaths: [], ...features }, policy);

test("new runtime source outside addons requires an explicit inventory review", () => {
  assert.deepEqual(codes(inspectInventory([
    "src/server/main.go",
    "src/server/core/orders.go",
    "src/server/addons/orders/model/order.go",
    "src/web/src/views/orders.vue",
    "src/web/src/addons/orders/index.ts"
  ], policy)), ["core-inventory", "core-inventory"]);
});

test("frontend concrete addon imports are forbidden even via relative paths", () => {
  assert.ok(codes(frontend('import x from "../addons/auth/api";')).includes("addon-import"));
  assert.ok(codes(frontend('export { x } from "@/addons/auth/api";')).includes("addon-import"));
  assert.ok(codes(frontend('void import("@/addons/auth/api");')).includes("addon-import"));
  assert.ok(codes(frontend('import x from "../../addons/auth/api";', "src/web/src/core/nested/file.ts")).includes("addon-import"));
});

test("composition imports plugin entrypoints only, not implementation internals", () => {
  assert.deepEqual(frontend('import orders from "./addons/orders";', "src/web/src/main.ts"), []);
  assert.ok(codes(frontend('import api from "./addons/orders/api";', "src/web/src/main.ts")).includes("addon-import"));
});

test("runtime imports cannot escape the reviewed frontend source root", () => {
  assert.ok(codes(frontend('import business from "../business";', "src/web/src/main.ts")).includes("source-root-import"));
  assert.ok(codes(frontend('void import("../../business.ts");', "src/web/src/core/index.ts")).includes("source-root-import"));
  assert.ok(codes(frontend('import business from "@/../business";', "src/web/src/main.ts")).includes("source-root-import"));
  assert.ok(codes(frontend('import business from "../../../business";', "src/web/src/addons/orders/index.ts")).includes("source-root-import"));
  assert.deepEqual(frontend('import favicon from "../public/favicon.svg";', "src/web/src/main.ts"), []);
});

test("only the plugin loader discovers addon entrypoints", () => {
  assert.deepEqual(frontend('import.meta.glob("../addons/*/index.ts", {eager:true});', "src/web/src/plugin/loader.ts"), []);
  assert.ok(codes(frontend('import.meta.glob("../addons/*/router/index.ts", {eager:true});')).includes("addon-discovery"));
});

test("route mutation is forbidden in addons and host setup hooks", () => {
  assert.ok(codes(frontend('router.addRoute({path:"/orders"});', "src/web/src/addons/orders/index.ts")).includes("route-registration"));
  assert.ok(codes(frontend('router["addRoute"]({path:"/orders"});', "src/web/src/main.ts")).includes("route-registration"));
  assert.ok(codes(frontend('registerExternalRoutes([]);', "src/web/src/main.ts")).includes("route-registration"));
});

test("inventoried router infrastructure cannot add arbitrary routes", () => {
  assert.deepEqual(frontend('const r = createRouter({routes:[{path:"/login", component:Login}]});'), []);
  assert.ok(codes(frontend('const r = createRouter({routes:[{path:"/orders", component:Orders}]});')).includes("route-literal"));
  assert.ok(codes(frontend('function bypass(){router.addRoute(route)}')).includes("route-registration"));
});

test("core API endpoint declarations are rejected, including old non-/api URLs", () => {
  assert.ok(codes(frontend('request({url:"/api/v1/orders"});')).includes("business-api"));
  assert.ok(codes(frontend('request({url:"/user/changePassword"});')).includes("business-api"));
  assert.ok(codes(frontend('fetch("/orders");')).includes("business-api"));
  assert.ok(codes(frontend('const prefix = "/api/v1/orders";')).includes("business-api"));
  assert.ok(codes(frontend('request({url:`/orders/${id}`});')).includes("business-api"));
  assert.ok(codes(frontend('http.get(`/orders/${id}`);')).includes("business-api"));
  assert.deepEqual(frontend('request({url:"/api/v1/system/plugin-registry"});', "src/web/src/plugin/loader.ts"), []);
});

test("routing aliases and method references cannot bypass addon registration", () => {
  assert.ok(codes(frontend('import {createRouter as create} from "vue-router"; create({routes:[]});', "src/web/src/main.ts")).includes("route-registration"));
  assert.ok(codes(frontend('const add = router.addRoute; add(route);', "src/web/src/addons/orders/index.ts")).includes("route-registration"));
});

test("Vue script blocks are analyzed without interpreting template text as code", () => {
  assert.ok(codes(frontend('<script setup lang="ts">request({url:"/orders"})</script><template><p>Hello</p></template>', "src/web/src/views/welcome/index.vue")).includes("business-api"));
});

test("new stores cannot be hidden in an inventoried framework source file", () => {
  assert.deepEqual(frontend('defineStore("pure-user", {});', "src/web/src/store/modules/user.ts"), []);
  assert.ok(codes(frontend('defineStore("order-management", {});', "src/web/src/store/modules/user.ts")).includes("business-store"));
});

test("backend composition allows only exact blank imports", () => {
  assert.deepEqual(backend({ imports: [{ path: "github.com/kwhitestone/prism-fusion/addons", alias: "_" }] }, "src/server/main.go"), []);
  assert.ok(codes(backend({ imports: [{ path: "github.com/kwhitestone/prism-fusion/addons/auth/service", alias: "auth" }] }, "src/server/main.go")).includes("addon-import"));
  assert.ok(codes(backend({ imports: [{ path: "github.com/kwhitestone/prism-fusion/addons", alias: "addons" }] }, "src/server/main.go")).includes("addon-import"));
});

test("backend route exemptions are exact calls and exact paths", () => {
  assert.deepEqual(backend({ calls: [{ name: "GET", function: "Routers", routing: true }], routePaths: ["/health"] }, "src/server/initialize/router.go"), []);
  assert.ok(codes(backend({ calls: [{ name: "POST", function: "Routers", routing: true }], routePaths: ["/orders"] }, "src/server/initialize/router.go")).includes("route-registration"));
  assert.ok(codes(backend({ routePaths: ["/orders"] }, "src/server/initialize/router.go")).includes("route-literal"));
});

test("business persistence cannot be added to existing core code", () => {
  assert.ok(codes(backend({ types: [{ name: "Order", persisted: true, fields: 2 }] }, "src/server/model/common.go")).includes("business-model"));
  assert.ok(codes(backend({ calls: [{ name: "Create", database: true, function: "RunServer" }] })).includes("business-persistence"));
  assert.ok(codes(backend({ selectors: ["PRISM_DB"] })).includes("database-access"));
  assert.ok(codes(backend({ types: [{ name: "ApiGroup", fields: 1 }] }, "src/server/api/v1/enter.go")).includes("business-group"));
});

test("plugin business source is allowed while its imperative router bypass is not", () => {
  assert.deepEqual(frontend('export const routes=[{path:"/orders",component:Orders}]; fetch("/api/v1/orders");', "src/web/src/addons/orders/index.ts"), []);
  assert.deepEqual(backend({ types: [{ name: "Order", persisted: true }], routePaths: ["/api/v1/addons/orders"] }, "src/server/addons/orders/plugin.go"), []);
});

test("Go AST extractor handles generic Huma registrations and ignores zap.Any", () => {
  const result = spawnSync("go", ["run", fileURLToPath(new URL("go-inspect/main.go", import.meta.url))], {
    encoding: "utf8",
    input: JSON.stringify([{ file: "src/server/core/server.go", source: `package core
import (
  h "github.com/danielgtaylor/huma/v2"
  "go.uber.org/zap"
)
type Order struct { ID uint \`gorm:"primaryKey"\` }
func RunServer() {
  zap.Any("error", nil)
  h.Register[Input, Output](api, h.Operation{Path:"/orders"}, handler)
  router.GET(routeFromVariable, handler)
  db.Create(&Order{})
}` }])
  });
  assert.equal(result.status, 0, result.stderr);
  const [features] = JSON.parse(result.stdout);
  assert.deepEqual(features.calls.map(call => call.name), ["Register", "GET", "Create"]);
  assert.deepEqual(features.routePaths, ["/orders", "<dynamic>"]);
  assert.ok(codes(inspectGoFeatures(features, policy)).includes("business-model"));
});

test("repository scan accepts a composition-only host and rejects a business file added to core", async () => {
  const root = await mkdtemp(join(tmpdir(), "prism-architecture-guard-"));
  try {
    await mkdir(join(root, "app/src/server"), { recursive: true });
    await mkdir(join(root, "app/src/admin/src/addons/orders"), { recursive: true });
    await writeFile(join(root, "app/src/server/main.go"), 'package main\nfunc main() {}\n');
    await writeFile(join(root, "app/src/admin/src/main.ts"), 'import orders from "./addons/orders"; registerExternalPlugins([orders]);');
    await writeFile(join(root, "app/src/admin/src/addons/orders/index.ts"), 'export default { name:"orders", routes:[{path:"/orders",component:Orders}] };');
    const valid = await checkRepository(root, "example");
    assert.deepEqual(valid.errors, []);
    assert.equal(valid.sourceCount, 3);
    await writeFile(join(root, "app/src/server/orders.go"), 'package main\ntype Order struct { ID uint `gorm:"primaryKey"` }\n');
    const invalid = await checkRepository(root, "example");
    assert.ok(codes(invalid.errors).includes("core-inventory"));
    assert.ok(codes(invalid.errors).includes("business-model"));
    await assert.rejects(checkRepository(root, "unknown"), /Unknown architecture profile/);
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});
