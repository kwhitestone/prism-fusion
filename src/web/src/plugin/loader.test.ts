import assert from "node:assert/strict";
import test from "node:test";
import { setImmediate as nextTurn } from "node:timers/promises";
import { createApp } from "vue";
import { createMemoryHistory, createRouter, type Router } from "vue-router";
import type { HostRoutes } from "./host-routes.js";
import type { PluginModule, PluginRegistryPayload } from "./types.js";

// The runner supplies only browser/store/transport infrastructure. Plugin
// registry, runtime, route snapshots and Vue Router are the production modules.
const adapterPath = "./loader-host.js";
const host = await import(adapterPath) as {
  router: Router;
  state: HostRoutes;
  counts: { commits: number; clears: number };
  resetHost: () => void;
};
const View = { render: () => null };
const transportPath = "./loader-request.js";
const transport = await import(transportPath) as {
  requests: { url: string; data: PluginRegistryPayload }[];
  resetRequests: (failure?: Error) => void;
};
let scenario = 0;

async function fresh() {
  host.resetHost();
  transport.resetRequests();
  // Query isolation gives each case a new loader singleton without adding a
  // test-only reset API to production code.
  const modulePath = `./loader.js?scenario=${++scenario}`;
  const loader = await import(modulePath) as typeof import("./loader");
  return { loader, app: createApp(View) };
}

function addon(name: string, extra: Partial<PluginModule> = {}): PluginModule {
  return {
    name,
    manifest: {
      apiVersion: "prism-fusion/v2", kind: "frontend-addon", id: name,
      version: "1.0.0", routeScopes: [`/${name}`]
    },
    routes: [{ path: `/${name}`, name, component: View }],
    ...extra
  };
}

function deferred() {
  let resolve!: () => void;
  const promise = new Promise<void>(done => { resolve = done; });
  return { promise, resolve };
}

test("external registration batches reject duplicates without partial registration", async () => {
  const { loader, app } = await fresh();
  loader.registerExternalPlugins([addon("base")]);
  assert.throws(() => loader.registerExternalPlugins([addon("new"), addon("base")]), /duplicate/i);
  assert.deepEqual(loader.getPlugins().map(plugin => plugin.name), ["base"]);
  assert.throws(() => loader.registerExternalPlugins([addon("new"), addon("new")]), /duplicate/i);
  assert.deepEqual(loader.getPlugins().map(plugin => plugin.name), ["base"]);
  await loader.installPlugins(app, host.router);
  assert.equal(host.router.hasRoute("base"), true);
  assert.equal(host.router.hasRoute("new"), false);
  await loader.uninstallPlugins();
});

test("registration closes synchronously when startup begins and stays frozen after uninstall", async () => {
  const { loader, app } = await fresh();
  loader.registerExternalPlugins([addon("base")]);
  const pending = loader.installPlugins(app, host.router);
  assert.throws(() => loader.registerExternalPlugins([addon("late")]), /frozen/i);
  await pending;
  await loader.uninstallPlugins();
  assert.throws(() => loader.registerExternalPlugins([addon("later")]), /frozen/i);
});

test("concurrent and repeated installation runs hooks and commits menus once", async () => {
  const { loader, app } = await fresh();
  const entered = deferred();
  const release = deferred();
  let setups = 0;
  loader.registerExternalPlugins([addon("base", {
    setup: async () => { setups++; entered.resolve(); await release.promise; }
  })]);
  const first = loader.installPlugins(app, host.router);
  await entered.promise;
  const second = loader.installPlugins(app, host.router);
  assert.equal(host.counts.commits, 0);
  release.resolve();
  const [firstStatuses, secondStatuses] = await Promise.all([first, second]);
  assert.equal(setups, 1);
  assert.equal(host.counts.commits, 1);
  firstStatuses[0].loaded = false;
  assert.equal(secondStatuses[0].loaded, true);
  host.router.addRoute({ path: "/dynamic", name: "Dynamic", component: View });
  await loader.installPlugins(app, host.router);
  assert.equal(setups, 1);
  assert.equal(host.counts.commits, 1);
  assert.equal(host.router.hasRoute("Dynamic"), true);
  await loader.uninstallPlugins();
});

test("different router is rejected before effects and different app cannot join pending startup", async () => {
  const { loader, app } = await fresh();
  const entered = deferred();
  const release = deferred();
  let setups = 0;
  loader.registerExternalPlugins([addon("base", {
    setup: async () => { setups++; entered.resolve(); await release.promise; }
  })]);
  const foreignRouter = createRouter({ history: createMemoryHistory(), routes: [] });
  await assert.rejects(loader.installPlugins(app, foreignRouter), /framework router|host/i);
  assert.equal(setups, 0);
  assert.equal(foreignRouter.getRoutes().length, 0);
  const pending = loader.installPlugins(app, host.router);
  await entered.promise;
  await assert.rejects(loader.installPlugins(createApp(View), host.router), /host/i);
  release.resolve();
  await pending;
  await assert.rejects(loader.installPlugins(createApp(View), host.router), /host/i);
  await loader.uninstallPlugins();
});

test("reset during delayed setup recovers all declared routes on successful commit", async () => {
  const { loader, app } = await fresh();
  const entered = deferred();
  const release = deferred();
  host.state.configure({ homePath: "/base" });
  loader.registerExternalPlugins([addon("base", {
    setup: async () => { entered.resolve(); await release.promise; }
  })]);
  const pending = loader.installPlugins(app, host.router);
  await entered.promise;
  assert.equal(host.router.hasRoute("base"), true);
  // Equivalent to logout/session cleanup before the host baseline is committed.
  host.state.restore(host.router);
  assert.equal(host.router.hasRoute("base"), false);
  release.resolve();
  await pending;
  assert.equal(host.router.hasRoute("base"), true);
  assert.equal(host.router.resolve("/base").name, "base");
  assert.equal(host.state.getRoutes()[0].redirect, "/base");
  assert.equal(host.state.getMenus().filter(route => route.path === "/base").length, 1);
  host.state.restore(host.router);
  assert.equal(host.router.hasRoute("base"), true);
  await loader.uninstallPlugins();
  assert.equal(host.router.hasRoute("base"), false);
  assert.equal(host.router.hasRoute("Home"), true);
});

test("immediate concurrent uninstall cancels deferred startup before hooks or commit", async () => {
  const { loader, app } = await fresh();
  let setups = 0;
  let destroys = 0;
  loader.registerExternalPlugins([addon("base", {
    setup: () => { setups++; }, destroy: () => { destroys++; }
  })]);
  const pending = loader.installPlugins(app, host.router);
  const rejected = assert.rejects(pending, /cancel/i);
  await Promise.all([loader.uninstallPlugins(), loader.uninstallPlugins(), rejected]);
  assert.equal(setups, 0);
  assert.equal(destroys, 0);
  assert.equal(host.counts.commits, 0);
  assert.equal(host.counts.clears, 1);
  assert.equal(host.router.hasRoute("base"), false);
  assert.deepEqual(loader.getLoadedPlugins(), []);
  await loader.installPlugins(app, host.router);
  assert.equal(setups, 1);
  assert.equal(host.router.hasRoute("base"), true);
  await loader.uninstallPlugins();
});

test("uninstall during delayed setup rolls back once and blocks racing reinstall", async () => {
  const { loader, app } = await fresh();
  const entered = deferred();
  const release = deferred();
  let destroys = 0;
  loader.registerExternalPlugins([addon("base", {
    setup: async () => { entered.resolve(); await release.promise; },
    destroy: () => { destroys++; }
  })]);
  const pending = loader.installPlugins(app, host.router);
  const rejected = assert.rejects(pending, /cancel/i);
  await entered.promise;
  const firstCleanup = loader.uninstallPlugins();
  const secondCleanup = loader.uninstallPlugins();
  await assert.rejects(loader.installPlugins(app, host.router), /cleanup|uninstall/i);
  release.resolve();
  await Promise.all([rejected, firstCleanup, secondCleanup]);
  assert.equal(destroys, 1);
  assert.equal(host.counts.commits, 0);
  assert.equal(host.counts.clears, 1);
  assert.equal(host.router.hasRoute("base"), false);
  assert.deepEqual(loader.getPluginStatuses(), []);
  assert.deepEqual(loader.getPluginRoutes(), []);
});

test("failed host commit rolls back runtime effects and keeps core routes", async () => {
  const { loader, app } = await fresh();
  let destroys = 0;
  host.state.configure({ homePath: "/missing" });
  loader.registerExternalPlugins([addon("base", { destroy: () => { destroys++; } })]);
  await assert.rejects(loader.installPlugins(app, host.router), /homePath|declared route/i);
  assert.equal(destroys, 1);
  assert.equal(host.router.hasRoute("base"), false);
  assert.equal(host.router.hasRoute("Home"), true);
  assert.deepEqual(loader.getLoadedPlugins(), []);
  assert.deepEqual(loader.getPluginRoutes(), []);
  assert.equal(host.state.getMenus().some(route => route.path === "/base"), false);
  await loader.uninstallPlugins();
});

test("failed plugin setup never commits menus and exposes only rolled-back statuses", async () => {
  const { loader, app } = await fresh();
  let destroys = 0;
  loader.registerExternalPlugins([addon("base", {
    setup: () => { throw new Error("setup unavailable"); },
    destroy: () => { destroys++; }
  })]);
  await assert.rejects(loader.installPlugins(app, host.router), /setup unavailable/);
  assert.equal(destroys, 1);
  assert.equal(host.counts.commits, 0);
  assert.equal(host.router.hasRoute("base"), false);
  assert.deepEqual(loader.getLoadedPlugins(), []);
  assert.deepEqual(loader.getPluginRoutes(), []);
  assert.equal(loader.getPluginStatuses()[0].loaded, false);
  assert.match(loader.getPluginStatuses()[0].error!, /setup unavailable/);
  await loader.uninstallPlugins();
});

test("registry reporting reflects committed menus and transport failure does not unload plugins", async context => {
  const { loader, app } = await fresh();
  loader.registerExternalPlugins([addon("base", {
    routes: [{ path: "/base", name: "Base", component: View,
      meta: { title: "Base", icon: "ep/home", rank: 4 }, children: [
        { path: "child", component: View, meta: { title: "Child", showLink: false } }
      ] }],
    permissions: [{ key: "base:item:view", name: "View items" }]
  })]);
  loader.triggerPluginRegistryReport();
  await nextTurn();
  assert.deepEqual(transport.requests[0].data.plugins, []);
  await loader.installPlugins(app, host.router);
  loader.triggerPluginRegistryReport();
  await nextTurn();
  const payload = transport.requests[1];
  assert.equal(payload.url, "/api/v1/system/plugin-registry");
  assert.equal(payload.data.plugins[0].version, "1.0.0");
  assert.equal(payload.data.plugins[0].menus[0].name, "Base");
  assert.equal(payload.data.plugins[0].menus[0].children[0].showLink, false);
  assert.equal(payload.data.plugins[0].permissions[0].key, "base:item:view");
  transport.resetRequests(new Error("report transport unavailable"));
  const warnings = context.mock.method(console, "warn", () => {});
  loader.triggerPluginRegistryReport();
  await nextTurn();
  assert.equal(warnings.mock.callCount(), 1);
  assert.equal(loader.getPluginStatuses()[0].loaded, true);
  assert.equal(host.router.hasRoute("Base"), true);
  await loader.uninstallPlugins();
});

test("successful uninstall removes restored routes and permits a new app on the same router", async () => {
  const { loader, app } = await fresh();
  let setups = 0;
  let destroys = 0;
  loader.registerExternalPlugins([addon("base", {
    setup: () => { setups++; }, destroy: () => { destroys++; }
  })]);
  await loader.installPlugins(app, host.router);
  host.state.restore(host.router);
  await loader.uninstallPlugins();
  assert.equal(host.router.hasRoute("base"), false);
  await loader.installPlugins(createApp(View), host.router);
  assert.equal(setups, 2);
  assert.equal(destroys, 1);
  assert.equal(host.counts.commits, 2);
  assert.equal(host.router.hasRoute("base"), true);
  await loader.uninstallPlugins();
  assert.equal(destroys, 2);
});

test("committed route redirect and props snapshots cannot alter later resets", async () => {
  const { loader, app } = await fresh();
  loader.registerExternalPlugins([addon("base", { routes: [
    { path: "/base", name: "base", redirect: { path: "/base/page", query: { source: "original" } } },
    { path: "/base/page", name: "BasePage", component: View, props: { label: { text: "original" } } }
  ] })]);
  await loader.installPlugins(app, host.router);
  const exposed = host.state.getRoutes();
  const redirect = exposed.find(route => route.name === "base")!.redirect as { path: string; query: { source: string } };
  redirect.path = "/elsewhere";
  redirect.query.source = "changed";
  const props = exposed.find(route => route.name === "BasePage")!.props as { label: { text: string } };
  props.label.text = "changed";
  host.state.restore(host.router);
  const restored = host.state.getRoutes();
  assert.deepEqual(restored.find(route => route.name === "base")!.redirect,
    { path: "/base/page", query: { source: "original" } });
  assert.deepEqual(restored.find(route => route.name === "BasePage")!.props,
    { label: { text: "original" } });
  await loader.uninstallPlugins();
});
