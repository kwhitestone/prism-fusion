import assert from "node:assert/strict";
import { test } from "node:test";
import { PluginRegistry } from "./registry.js";
import type { PluginModule, PluginManifest } from "./types.js";

function plugin(
  name: string,
  extra: Partial<PluginManifest> = {}
): PluginModule {
  return {
    name,
    manifest: {
      apiVersion: "prism-fusion/v2",
      kind: "frontend-addon",
      id: name,
      version: "1.0.0",
      ...extra
    }
  };
}

test("V1 and V2 resolve by dependencies, priority, then canonical ID", () => {
  const registry = new PluginRegistry();
  registry.register(plugin("consumer", { requires: [{ id: "auth" }] }));
  registry.register({ name: "z-legacy", priority: 1 });
  registry.register({ ...plugin("b-first"), priority: 1 });
  registry.register({ ...plugin("auth"), priority: 200 });
  assert.deepEqual(
    registry.resolve().map(p => p.name),
    ["b-first", "z-legacy", "auth", "consumer"]
  );
  assert.equal(registry.isFrozen, false);
  assert.deepEqual(
    registry.freeze().map(p => p.name),
    registry.resolve().map(p => p.name)
  );
  assert.equal(registry.isFrozen, true);
  assert.throws(() => registry.register(plugin("late")), /frozen/i);
});

test("metadata is copied at registration and every getter returns independent data", () => {
  const registry = new PluginRegistry();
  const original = plugin("a", {
    routeScopes: ["/a"],
    optional: [{ id: "optional" }]
  });
  original.routes = [
    {
      path: "/a",
      name: "A",
      component: { render: () => null },
      meta: { title: "original" }
    }
  ];
  original.permissions = [{ key: "a:item:view", name: "View" }];
  registry.register(original);
  original.name = "changed";
  original.manifest.routeScopes[0] = "/changed";
  original.routes[0].meta.title = "changed";
  original.permissions[0].name = "changed";
  const first = registry.freeze()[0];
  assert.equal(first.name, "a");
  assert.deepEqual(first.manifest.routeScopes, ["/a"]);
  assert.equal(first.routes[0].meta.title, "original");
  assert.equal(first.permissions[0].name, "View");
  first.manifest.routeScopes.push("/other");
  first.routes[0].meta.title = "again";
  first.permissions[0].name = "again";
  assert.deepEqual(registry.getPlugins()[0].manifest.routeScopes, ["/a"]);
  assert.equal(registry.resolve()[0].routes[0].meta.title, "original");
  assert.equal(registry.freeze()[0].permissions[0].name, "View");
});

test("activation is evaluated at freeze, captured once, and disabled plugins cannot satisfy dependencies", () => {
  let enabled = true;
  let calls = 0;
  const registry = new PluginRegistry();
  registry.register({
    ...plugin("auth"),
    enabled: () => {
      calls++;
      return enabled;
    }
  });
  registry.register(plugin("consumer", { requires: [{ id: "auth" }] }));
  assert.equal(calls, 0);
  enabled = false;
  assert.throws(() => registry.freeze(), /consumer.*auth|auth.*consumer/i);
  enabled = true;
  registry.freeze();
  const frozenCalls = calls;
  enabled = false;
  assert.equal(registry.freeze().length, 2);
  assert.equal(registry.resolve().length, 2);
  assert.equal(calls, frozenCalls);
});

test("optional dependencies order present plugins but allow missing and disabled providers", () => {
  const registry = new PluginRegistry();
  registry.register(
    plugin("a", { optional: [{ id: "z" }, { id: "absent" }, { id: "off" }] })
  );
  registry.register(plugin("z"));
  registry.register({ ...plugin("off"), enabled: false });
  assert.deepEqual(
    registry.freeze().map(p => p.name),
    ["z", "a"]
  );
});

test("missing dependencies, self-dependencies, cycles, and conflicts fail closed", () => {
  const cases = [
    [plugin("a", { requires: [{ id: "missing" }] })],
    [plugin("a", { requires: [{ id: "a" }] })],
    [
      plugin("a", { requires: [{ id: "b" }] }),
      plugin("b", { optional: [{ id: "a" }] })
    ],
    [plugin("a", { conflicts: ["b"] }), plugin("b")]
  ];
  for (const plugins of cases) {
    const registry = new PluginRegistry();
    plugins.forEach(p => registry.register(p));
    assert.throws(() => registry.freeze(), /dependency|cycle|conflict/i);
  }
  const registry = new PluginRegistry();
  registry.register(plugin("a", { conflicts: ["off"] }));
  registry.register({ ...plugin("off"), enabled: false });
  assert.deepEqual(
    registry.freeze().map(p => p.name),
    ["a"]
  );
});

test("duplicate IDs and malformed modules or manifests are rejected immediately", () => {
  const registry = new PluginRegistry();
  registry.register(plugin("a"));
  assert.throws(() => registry.register({ name: "a" }), /duplicate/i);
  const invalid: unknown[] = [
    null,
    [],
    { name: "" },
    { name: " padded " },
    { name: "a", enabled: "yes" },
    { name: "a", priority: NaN },
    { name: "a", priority: "10" },
    { name: "a", routes: {} },
    { name: "a", install: true },
    { name: "a", setup: "run" },
    { name: "a", destroy: {} },
    { name: "a", components: [] },
    { name: "a", permissions: [null] },
    { name: "a", permissions: [{ key: 1, name: "View" }] },
    { name: "a", permissions: [{ key: "a:item:view", name: "" }] },
    {
      name: "a",
      permissions: [{ key: "a:item:view", name: "View", description: false }]
    },
    plugin("Bad-ID"),
    plugin("a", { apiVersion: "v3" as any }),
    plugin("a", { kind: "backend-addon" as any }),
    plugin("a", { id: "b" }),
    plugin("a", { version: "latest" }),
    plugin("a", { version: "01.0.0" }),
    { ...plugin("a"), version: "2.0.0" },
    plugin("a", { version: "1.0.0-01" }),
    plugin("a", { requires: "x" as any }),
    plugin("a", { optional: ["x"] as any }),
    plugin("a", { conflicts: [1] as any }),
    plugin("a", { requires: [{ id: "b", version: "^1" }] }),
    plugin("a", { optional: [{ id: "b", version: 1 as any }] }),
    plugin("a", { routeScopes: ["/"] }),
    plugin("a", { routeScopes: ["/a/:id"] }),
    plugin("a", { routeScopes: ["/a/../b"] }),
    plugin("a", { routeScopes: ["/a%2Fb"] }),
    plugin("a", { routeScopes: ["/a/"] }),
    plugin("a", { routeScopes: ["//a"] })
  ];
  for (const input of invalid) {
    assert.throws(
      () => new PluginRegistry().register(input as PluginModule),
      undefined,
      `accepted invalid module: ${JSON.stringify(input)}`
    );
  }
});

test("activation callbacks must return a boolean and preserve thrown cause", () => {
  const invalid = new PluginRegistry();
  invalid.register({ name: "bad", enabled: (() => "yes") as any });
  assert.throws(() => invalid.freeze(), /boolean/i);
  const failing = new PluginRegistry();
  failing.register({
    name: "bad",
    enabled: () => {
      throw new Error("config failed");
    }
  });
  assert.throws(() => failing.freeze(), /config failed/);
});

test("valid prereleases and deduplicated dependencies remain deterministic", () => {
  const registry = new PluginRegistry();
  registry.register(
    plugin("consumer", {
      version: "1.2.3-alpha.1+build.02",
      requires: [{ id: "base" }, { id: "base", version: "" }],
      optional: [{ id: "base" }],
      routeScopes: ["/consumer", "/consumer"]
    })
  );
  registry.register(plugin("base"));
  assert.deepEqual(
    registry.freeze().map(p => p.name),
    ["base", "consumer"]
  );
});

test("activation cannot register or freeze recursively while snapshots are being resolved", () => {
  for (const action of ["register", "freeze", "resolve"] as const) {
    const registry = new PluginRegistry();
    registry.register({
      name: "a",
      enabled: () => {
        if (action === "register") registry.register({ name: "hidden" });
        else registry[action]();
        return true;
      }
    });
    assert.throws(() => registry.freeze(), /resolving|frozen/i);
    assert.equal(registry.getPlugins().length, 1);
  }
});
