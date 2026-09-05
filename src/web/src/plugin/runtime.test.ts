import assert from "node:assert/strict";
import { test } from "node:test";
import { createApp } from "vue";
import { createMemoryHistory, createRouter } from "vue-router";
import { PluginRegistry } from "./registry.js";
import { PluginRuntime } from "./runtime.js";
import type { PluginModule } from "./types.js";

const View = { render: () => null };
function addon(
  name: string,
  routes: any[] = [],
  extra: Partial<PluginModule> = {}
): PluginModule {
  return {
    name,
    manifest: {
      apiVersion: "prism-fusion/v2",
      kind: "frontend-addon",
      id: name,
      version: "1.0.0",
      routeScopes: [`/${name}`]
    },
    routes: routes.map(route => ({ component: View, ...route })),
    ...extra
  };
}
function host(plugins: PluginModule[], core: any[] = []) {
  const registry = new PluginRegistry();
  plugins.forEach(plugin => registry.register(plugin));
  const runtime = new PluginRuntime(registry);
  const app = createApp(View);
  const router = createRouter({ history: createMemoryHistory(), routes: core });
  return { registry, runtime, app, router };
}

test("preflight rejects route name/path/alias/nested/dynamic collisions before any effects", async () => {
  const cases: [any[], any[]][] = [
    [
      [{ path: "/a", name: "Taken" }],
      [{ path: "/core", name: "Taken", component: View }]
    ],
    [[{ path: "/a" }], [{ path: "/A/", component: View }]],
    [[{ path: "/a", alias: "/core" }], [{ path: "/core", component: View }]],
    [
      [{ path: "/a", children: [{ path: "/core", component: View }] }],
      [{ path: "/core", component: View }]
    ],
    [[{ path: "/a/:id" }], [{ path: "/a/:other", component: View }]],
    [
      [
        {
          path: "/a",
          children: [{ path: "child", name: "Taken", component: View }]
        }
      ],
      [{ path: "/core", name: "Taken", component: View }]
    ],
    [
      [
        {
          path: "/a",
          children: [{ path: "child", alias: "/core", component: View }]
        }
      ],
      [{ path: "/core", component: View }]
    ],
    [
      [
        { path: "/a", name: "Dup" },
        { path: "/a/other", name: "Dup" }
      ],
      []
    ],
    [[{ path: "/a/:id" }, { path: "/a/:other" }], []]
  ];
  for (const [routes, core] of cases) {
    let installed = false;
    const { runtime, app, router } = host(
      [
        addon("valid", [{ path: "/valid" }], {
          setup: () => {
            installed = true;
          }
        }),
        {
          name: "a",
          routes: routes.map(route => ({ component: View, ...route }))
        }
      ],
      core
    );
    await assert.rejects(
      runtime.install(app, router),
      /collision|duplicate|scope/i
    );
    assert.equal(installed, false);
    assert.equal(router.getRoutes().length, core.length);
    assert.deepEqual(runtime.getLoadedPlugins(), []);
    assert.deepEqual(runtime.getRoutes(), []);
  }
});

test("all V2 routes, nested absolute routes, aliases and prefixes must stay in declared scopes", async () => {
  for (const routes of [
    [{ path: "/ab" }],
    [{ path: "/other" }],
    [{ path: "/a", alias: "/other" }],
    [{ path: "/a", children: [{ path: "/other", component: View }] }],
    [{ path: "/a/../other" }],
    [{ path: "/a%2Fother" }]
  ]) {
    const { runtime, app, router } = host([addon("a", routes)]);
    await assert.rejects(runtime.install(app, router), /scope|path/i);
    assert.equal(router.getRoutes().length, 0);
  }
  const { runtime, app, router } = host([
    addon("a", [{ path: "/a" }], {
      manifest: {
        apiVersion: "prism-fusion/v2",
        kind: "frontend-addon",
        id: "a",
        version: "1.0.0"
      }
    })
  ]);
  await assert.rejects(runtime.install(app, router), /scope/i);
});

test("default children, relative children and their aliases work in scope", async () => {
  const { runtime, app, router } = host([
    addon(
      "a",
      [
        {
          path: "/a",
          name: "Layout",
          alias: "/a-alias",
          children: [
            { path: "", name: "Default", component: View },
            {
              path: "child/:id",
              name: "Child",
              component: View,
              alias: "alt/:id"
            }
          ]
        }
      ],
      {
        manifest: {
          apiVersion: "prism-fusion/v2",
          kind: "frontend-addon",
          id: "a",
          version: "1.0.0",
          routeScopes: ["/a", "/a-alias"]
        }
      }
    )
  ]);
  await runtime.install(app, router);
  assert.equal(router.resolve("/a").name, "Default");
  assert.equal(router.resolve("/a-alias/alt/42").name, "Child");
  assert.equal(runtime.getRoutes().length, 1);
  await runtime.uninstall();
  assert.equal(router.getRoutes().length, 0);
});

test("failed setup rolls back routes/components and destroys initialized plugins in reverse order", async context => {
  const logged = context.mock.method(console, "error", () => {});
  const calls: string[] = [];
  const { runtime, app, router } = host(
    [
      addon("a", [{ path: "/a", name: "A" }], {
        components: { WidgetA: View },
        setup: () => {
          calls.push("a:setup");
        },
        destroy: () => {
          calls.push("a:destroy");
        }
      }),
      addon("b", [{ path: "/b", name: "B" }], {
        components: { WidgetB: View },
        setup: () => {
          calls.push("b:setup");
          throw new Error("setup failed");
        },
        destroy: () => {
          calls.push("b:destroy");
          throw new Error("cleanup failed");
        }
      })
    ],
    [{ path: "/core", name: "Core", component: View }]
  );
  await assert.rejects(runtime.install(app, router), /setup failed/);
  assert.deepEqual(calls, ["a:setup", "b:setup", "b:destroy", "a:destroy"]);
  assert.equal(router.hasRoute("A"), false);
  assert.equal(router.hasRoute("B"), false);
  assert.equal(router.hasRoute("Core"), true);
  assert.equal(app.component("WidgetA"), undefined);
  assert.equal(app.component("WidgetB"), undefined);
  assert.deepEqual(runtime.getRoutes(), []);
  assert.equal(
    runtime.getStatuses().every(status => !status.loaded),
    true
  );
  assert.equal(logged.mock.calls.length, 1);
});

test("component name collisions and invalid patterns fail before setup", async () => {
  for (const input of [
    [
      addon("a", [], { components: { Shared: View } }),
      addon("b", [], { components: { Shared: View } })
    ],
    [addon("a", [{ path: "/a/:broken(" }])]
  ]) {
    const { runtime, app, router } = host(input);
    await assert.rejects(runtime.install(app, router));
    assert.equal(router.getRoutes().length, 0);
    assert.equal(app.component("Shared"), undefined);
  }
  const { runtime, app, router } = host([
    addon("a", [], { components: { Shared: View } })
  ]);
  app.component("Shared", View);
  await assert.rejects(runtime.install(app, router), /component.*collision/i);
  assert.equal(app.component("Shared"), View);
});

test("repeated/concurrent installation is singleflight and snapshots cannot corrupt runtime state", async () => {
  let setups = 0;
  let release: () => void;
  const waiting = new Promise<void>(resolve => {
    release = resolve;
  });
  const { runtime, app, router } = host([
    addon("a", [{ path: "/a", name: "A" }], {
      setup: async () => {
        setups++;
        await waiting;
      }
    })
  ]);
  const first = runtime.install(app, router);
  const second = runtime.install(app, router);
  release();
  const [firstStatuses, secondStatuses] = await Promise.all([first, second]);
  assert.equal(setups, 1);
  assert.deepEqual(firstStatuses, [{ name: "a", loaded: true }]);
  firstStatuses[0].loaded = false;
  assert.equal(secondStatuses[0].loaded, true);
  await runtime.install(app, router);
  assert.equal(setups, 1);
  const plugins = runtime.getLoadedPlugins();
  plugins[0].routes[0].path = "/changed";
  assert.equal(runtime.getRoutes()[0].path, "/a");
  await assert.rejects(
    runtime.install(createApp(View), router),
    /host|different/i
  );
  await runtime.uninstall();
  assert.deepEqual(runtime.getStatuses(), []);
  await runtime.install(app, router);
  assert.equal(setups, 2);
  assert.equal(router.getRoutes().length, 1);
});

test("uninstall during async setup cancels startup and leaves no routes or plugins", async () => {
  let release: () => void;
  let entered: () => void;
  const started = new Promise<void>(resolve => {
    entered = resolve;
  });
  const waiting = new Promise<void>(resolve => {
    release = resolve;
  });
  const { runtime, app, router } = host([
    addon("a", [{ path: "/a" }], {
      setup: async () => {
        entered();
        await waiting;
      }
    })
  ]);
  const installing = runtime.install(app, router);
  await started;
  const uninstalling = runtime.uninstall();
  release();
  await assert.rejects(installing, /cancel/i);
  await uninstalling;
  assert.deepEqual(runtime.getLoadedPlugins(), []);
  assert.deepEqual(runtime.getStatuses(), []);
  assert.equal(router.getRoutes().length, 0);
});

test("disabled plugins have no routes, hooks, components or menu snapshots", async () => {
  const { runtime, app, router } = host([
    addon("off", [{ path: "/off" }], {
      enabled: false,
      components: { Hidden: View },
      setup: () => {
        throw new Error("disabled setup called");
      }
    })
  ]);
  assert.deepEqual(await runtime.install(app, router), []);
  assert.deepEqual(runtime.getRoutes(), []);
  assert.equal(app.component("Hidden"), undefined);
  assert.equal(router.getRoutes().length, 0);
});

test("router reset preserves generated route identities and uninstall removes restored routes", async () => {
  const core = { path: "/core", name: "Core", component: View };
  const { runtime, app, router } = host(
    [
      addon("a", [
        {
          path: "/a",
          children: [
            { path: "", component: View },
            { path: "child", name: "Child", component: View }
          ]
        }
      ])
    ],
    [core]
  );
  await runtime.install(app, router);
  const restored = runtime.getRoutes();
  assert.equal(typeof restored[0].name, "symbol");
  router.clearRoutes();
  [core, ...restored].forEach(route => router.addRoute(route));
  assert.equal(router.resolve("/a/child").name, "Child");
  await runtime.uninstall();
  assert.deepEqual(
    router.getRoutes().map(route => route.name),
    ["Core"]
  );
});

test("immediate cancellation also rejects startup with no active plugins", async () => {
  const { runtime, app, router } = host([]);
  const installing = runtime.install(app, router);
  const uninstalling = runtime.uninstall();
  await assert.rejects(installing, /cancel/i);
  await uninstalling;
  assert.deepEqual(runtime.getStatuses(), []);
});

test("different owners cannot intercept each other's static or dynamic routes", async () => {
  for (const [corePath, pluginPath] of [
    ["/a/:id", "/a/create"],
    ["/a/create", "/a/:id"],
    ["/a/:id(\\d+)", "/a/:other([0-9]+)"],
    ["/a/:id?", "/a"],
    ["/a/:tail(.*)*", "/a/sub/:id"]
  ]) {
    const { runtime, app, router } = host(
      [addon("a", [{ path: pluginPath }])],
      [{ path: corePath, name: "Core", component: View }]
    );
    await assert.rejects(runtime.install(app, router), /collision/i);
    assert.equal(router.getRoutes().length, 1);
  }
});

test("static and dynamic alternatives within the same owner and disjoint owner prefixes stay valid", async () => {
  const { runtime, app, router } = host(
    [
      addon("a", [
        { path: "/a/create", name: "Create" },
        { path: "/a/:id", name: "Item" }
      ]),
      addon("b", [{ path: "/b/:id", name: "OtherItem" }])
    ],
    [{ path: "/core/:id", component: View }]
  );
  await runtime.install(app, router);
  assert.equal(router.resolve("/a/create").name, "Create");
  assert.equal(router.resolve("/a/42").name, "Item");
  await runtime.uninstall();
});
