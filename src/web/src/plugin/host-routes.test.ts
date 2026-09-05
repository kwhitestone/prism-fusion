import assert from "node:assert/strict";
import test from "node:test";
import { createRouter, createMemoryHistory } from "vue-router";
import type { RouteRecordRaw } from "vue-router";
import { HostRoutes } from "./host-routes.js";

const View = { render: () => null };
const core = (): RouteRecordRaw[] => [
  { path: "/", name: "Home", redirect: "/welcome", component: View,
    children: [{ path: "/welcome", name: "Welcome", component: View }], meta: { title: "Home", rank: 0 } },
  { path: "/login", name: "Login", component: View, meta: { title: "Login", showLink: false } }
];
const plugins = (): RouteRecordRaw[] => [{ path: "/dashboard", name: "Dashboard", component: View,
  meta: { title: "Dashboard" }, children: [{ path: "", name: "DashboardIndex", component: View }] }];

test("a committed plugin survives logout reset without duplicating routes or menus", () => {
  const state = new HostRoutes(core());
  const router = createRouter({ history: createMemoryHistory(), routes: core() });
  state.configure({ homePath: "/dashboard" });
  state.commit(plugins());
  state.restore(router);
  router.addRoute({ path: "/dynamic", name: "Dynamic", component: View });
  state.restore(router);
  assert.equal(router.hasRoute("DashboardIndex"), true);
  assert.equal(router.hasRoute("Dynamic"), false);
  assert.equal(router.resolve("/dashboard").name, "DashboardIndex");
  assert.equal(state.getRoutes()[0].redirect, "/dashboard");
  assert.equal(state.getMenus().filter(route => route.path === "/dashboard").length, 1);
  assert.throws(() => state.configure({ homePath: "/welcome" }), /frozen/);
});

test("invalid home targets fail before committing routes", () => {
  for (const homePath of ["/", "https://example.com", "/missing", "/dashboard/../welcome", "/dashboard?x=1"]) {
    const state = new HostRoutes(core());
    assert.throws(() => { state.configure({ homePath }); state.commit(plugins()); });
    assert.equal(state.getRoutes().some(route => route.path === "/dashboard"), false);
  }
});

test("snapshots preserve components but do not expose mutable menu or route metadata", () => {
  const state = new HostRoutes(core());
  const declared = plugins();
  state.commit(declared);
  declared[0].meta.title = "changed";
  const exposed = state.getRoutes();
  exposed[2].meta!.title = "also changed";
  exposed[0].children!.length = 0;
  assert.equal(state.getRoutes()[2].meta!.title, "Dashboard");
  assert.equal(state.getRoutes()[0].children!.length, 1);
  assert.equal(state.getRoutes()[2].component, View);
});

test("clearing plugins restores the shell and repeated snapshots are idempotent", () => {
  const state = new HostRoutes(core());
  state.configure({ homePath: "/dashboard" });
  state.commit(plugins());
  state.clear();
  assert.equal(state.getRoutes().length, 2);
  assert.equal(state.getRoutes()[0].redirect, "/welcome");
});
