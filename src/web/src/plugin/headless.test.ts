import assert from "node:assert/strict";
import test from "node:test";
import { createApp } from "vue";
import { createMemoryHistory, createRouter, type RouteRecordRaw } from "vue-router";
import { createPluginHost } from "./headless.js";
import type { PluginModule } from "./types";

function fixture(homePath = "/orders") {
  const app = createApp({ render: () => null });
  const coreRoutes: RouteRecordRaw[] = [
    { path: "/", redirect: "/orders", name: "root" },
    { path: "/:pathMatch(.*)*", redirect: "/orders", name: "not-found" }
  ];
  const router = createRouter({ history: createMemoryHistory(), routes: coreRoutes });
  const host = createPluginHost({ app, router, coreRoutes, homePath });
  return { app, router, host };
}

function addon(name = "orders", extra: Partial<PluginModule> = {}): PluginModule {
  return {
    name,
    manifest: { apiVersion: "prism-fusion/v2", kind: "frontend-addon", id: name, version: "1.0.0", routeScopes: [`/${name}`] },
    routes: [{ path: `/${name}`, name, component: { render: () => null } }],
    ...extra
  };
}

test("headless host supports a core fallback without importing the framework UI", async () => {
  const { host, router } = fixture();
  host.register([addon()]);
  await host.install();
  assert.equal(router.resolve("/orders").name, "orders");
  assert.equal(router.resolve("/missing").name, "not-found");
  assert.deepEqual(host.getStatuses(), [{ name: "orders", loaded: true }]);
  await host.uninstall();
  assert.equal(router.hasRoute("orders"), false);
  assert.equal(router.hasRoute("not-found"), true);
});

test("headless registration is atomic and freezes at install", async () => {
  const { host } = fixture();
  assert.throws(() => host.register([addon(), addon()]), /duplicate/i);
  host.register([addon()]);
  await host.install();
  assert.throws(() => host.register([addon("other")]), /frozen/i);
  await host.uninstall();
});

test("headless host awaits hooks and commits route snapshots once", async () => {
  const { host, router } = fixture();
  let release!: () => void;
  let installs = 0;
  host.register([addon("orders", { setup: () => { installs++; return new Promise<void>(resolve => { release = resolve; }); } })]);
  const first = host.install();
  const second = host.install();
  await new Promise(resolve => setTimeout(resolve, 0));
  assert.equal(host.getRoutes().some(route => route.path === "/orders"), false);
  release();
  await Promise.all([first, second]);
  await host.install();
  assert.equal(installs, 1);
  router.clearRoutes();
  host.restoreRoutes();
  assert.equal(router.hasRoute("orders"), true);
  assert.equal(router.getRoutes().filter(route => route.path === "/orders").length, 1);
  const snapshot = host.getRoutes();
  snapshot[0].path = "/tampered";
  assert.equal(host.getRoutes()[0].path, "/");
  await host.uninstall();
});

test("headless host rolls back failed hooks and missing landing page", async () => {
  const { host, router } = fixture();
  let cleaned = 0;
  host.register([addon("orders", { setup: () => { throw new Error("boom"); }, destroy: () => { cleaned++; } })]);
  await assert.rejects(host.install(), /boom/);
  assert.equal(cleaned, 1);
  assert.equal(router.hasRoute("orders"), false);
  assert.equal(router.hasRoute("root"), true);
  const invalid = fixture("/missing");
  invalid.host.register([addon()]);
  await assert.rejects(invalid.host.install(), /homePath/);
  assert.equal(invalid.router.hasRoute("orders"), false);
});

test("headless host still rejects core and plugin collisions", async () => {
  const { host, router } = fixture();
  router.addRoute({ path: "/orders", name: "existing", component: {} });
  host.register([addon()]);
  await assert.rejects(host.install(), /collision/);
  assert.equal(router.hasRoute("existing"), true, "failed preflight must not delete prior host routes");
});

test("headless immediate cancellation cannot mount a pending plugin", async () => {
  const { host, router } = fixture();
  let installed = 0;
  host.register([addon("orders", { install: () => { installed++; } })]);
  const pending = host.install();
  const stopping = host.uninstall();
  await assert.rejects(pending, /cancelled/);
  await stopping;
  assert.equal(installed, 0);
  assert.equal(router.hasRoute("orders"), false);
});

test("headless uninstall cancels an in-flight installation", async () => {
  const { host, router } = fixture();
  let release!: () => void;
  let cleaned = 0;
  host.register([addon("orders", { setup: () => new Promise<void>(resolve => { release = resolve; }), destroy: () => { cleaned++; } })]);
  const pending = host.install();
  await new Promise(resolve => setTimeout(resolve, 0));
  const stopping = host.uninstall();
  release();
  await assert.rejects(pending, /cancelled/);
  await stopping;
  assert.equal(cleaned, 1);
  assert.equal(router.hasRoute("orders"), false);
  assert.equal(host.getRoutes().some(route => route.path === "/orders"), false);
});

test("headless hosts have independent registries and no shared router state", async () => {
  const left = fixture();
  const right = fixture("/other");
  left.host.register([addon()]);
  right.host.register([addon("other")]);
  await Promise.all([left.host.install(), right.host.install()]);
  assert.equal(left.router.hasRoute("other"), false);
  assert.equal(right.router.hasRoute("orders"), false);
  await Promise.all([left.host.uninstall(), right.host.uninstall()]);
});
