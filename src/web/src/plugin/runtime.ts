import { createRouterMatcher } from "vue-router";
import type { App } from "vue";
import type { Router, RouteRecordRaw } from "vue-router";
import { clonePlugin, cloneRoutes, PluginRegistry } from "./registry.js";
import type { PluginModule, PluginStatus } from "./types";

export type PluginApp = Pick<App, "component" | "_context">;
export type PluginRouter = Pick<
  Router,
  "addRoute" | "getRoutes" | "hasRoute" | "removeRoute"
>;

interface RouteClaim {
  owner: string;
  path: string;
  node: object;
  ancestors: object[];
}

function normalizedPath(path: string): string {
  return path.replace(/\/+$/, "").toLowerCase() || "/";
}

function routePattern(path: string): string {
  // Parameter names do not distinguish routes in Vue Router.
  return normalizedPath(path).replace(/:[a-z0-9_]+/gi, ":param");
}

function joinPath(parent: string, path: string): string {
  if (path.startsWith("/")) return path;
  return `${parent.replace(/\/$/, "")}/${path}`;
}

function validatePath(path: unknown, root: boolean): asserts path is string {
  if (
    typeof path !== "string" ||
    (root && !path.startsWith("/")) ||
    /[%#\\\s]/.test(path) ||
    path.includes("//") ||
    path.split("/").some(segment => segment === "." || segment === "..")
  ) {
    throw new Error(`Invalid plugin route path: ${String(path)}`);
  }
}

function checkScope(plugin: PluginModule, path: string): void {
  if (!plugin.manifest) return;
  const route = normalizedPath(path);
  const scopes = plugin.manifest.routeScopes ?? [];
  if (
    !scopes.some(
      scope =>
        route === normalizedPath(scope) ||
        route.startsWith(`${normalizedPath(scope)}/`)
    )
  ) {
    throw new Error(
      `Plugin ${plugin.name} route ${path} is outside its declared route scopes`
    );
  }
}

function collectClaims(
  plugin: PluginModule,
  names: Set<string | symbol>
): RouteClaim[] {
  const claims: RouteClaim[] = [];
  const walk = (
    routes: RouteRecordRaw[],
    parents: string[],
    ancestors: object[]
  ) => {
    if (!Array.isArray(routes))
      throw new Error(`Plugin ${plugin.name} route children must be an array`);
    for (const route of routes) {
      if (!route || typeof route !== "object")
        throw new Error(`Plugin ${plugin.name} route must be an object`);
      validatePath(route.path, ancestors.length === 0);
      if (route.name !== undefined) {
        if (!["string", "symbol"].includes(typeof route.name))
          throw new Error("Plugin route name must be a string or symbol");
        if (names.has(route.name))
          throw new Error(
            `Route name collision in plugin ${plugin.name}: ${String(route.name)}`
          );
        names.add(route.name);
      }
      const aliases =
        route.alias === undefined
          ? []
          : Array.isArray(route.alias)
            ? route.alias
            : [route.alias];
      aliases.forEach(alias => validatePath(alias, ancestors.length === 0));
      const paths = [
        ...new Set(
          parents.flatMap(parent =>
            [route.path, ...aliases].map(path => joinPath(parent, path))
          )
        )
      ];
      for (const path of paths) {
        checkScope(plugin, path);
        claims.push({ owner: plugin.name, path, node: route, ancestors });
      }
      if (route.children) walk(route.children, paths, [...ancestors, route]);
    }
  };
  walk(plugin.routes ?? [], [""], []);
  return claims;
}

function collision(a: RouteClaim, b: RouteClaim): boolean {
  // Empty-path children intentionally share a URL with their layout ancestors.
  if (
    a.owner === b.owner &&
    (a.node === b.node ||
      a.ancestors.includes(b.node) ||
      b.ancestors.includes(a.node))
  )
    return false;
  if (routePattern(a.path) === routePattern(b.path)) return true;
  if (a.owner === b.owner) return false;
  const aDynamic = a.path.includes(":");
  const bDynamic = b.path.includes(":");
  if (aDynamic && bDynamic) {
    // Custom regular expressions can overlap despite having different spellings.
    // Fail closed for shared static prefixes; owners must use disjoint namespaces.
    const aPrefix = a.path.slice(0, a.path.indexOf(":")).toLowerCase();
    const bPrefix = b.path.slice(0, b.path.indexOf(":")).toLowerCase();
    return aPrefix.startsWith(bPrefix) || bPrefix.startsWith(aPrefix);
  }
  if (!aDynamic && !bDynamic) return false;
  const dynamic = aDynamic ? a.path : b.path;
  const literal = aDynamic ? b.path : a.path;
  const matcher = createRouterMatcher(
    [{ path: dynamic, name: Symbol("preflight") } as RouteRecordRaw],
    {}
  ).getRoutes()[0];
  return matcher.re.test(literal);
}

function nameAnonymousRoutes(plugin: PluginModule): PluginModule {
  const walk = (routes: RouteRecordRaw[]): RouteRecordRaw[] =>
    routes.map(
      route =>
        ({
          ...route,
          name: route.name ?? Symbol(`plugin:${plugin.name}:${route.path}`),
          ...(route.children ? { children: walk(route.children) } : {})
        }) as RouteRecordRaw
    );
  return {
    ...plugin,
    ...(plugin.routes ? { routes: walk(plugin.routes) } : {})
  };
}

function preflight(
  plugins: PluginModule[],
  app: PluginApp,
  router: PluginRouter
): PluginModule[] {
  const names = new Set(
    router
      .getRoutes()
      .flatMap(route => (route.name === undefined ? [] : [route.name]))
  );
  const claims: RouteClaim[] = router
    .getRoutes()
    .map(route => ({
      owner: "$core",
      path: route.path,
      node: route,
      ancestors: []
    }));
  const components = new Set<string>();
  for (const plugin of plugins) {
    const added = collectClaims(plugin, names);
    for (const claim of added) {
      const existing = claims.find(other => collision(claim, other));
      if (existing)
        throw new Error(
          `Route path collision: ${plugin.name} ${claim.path} conflicts with ${existing.owner} ${existing.path}`
        );
      claims.push(claim);
    }
    // Use the same parser as the live router, without registering any live route.
    createRouterMatcher(plugin.routes ?? [], {});
    for (const name of Object.keys(plugin.components ?? {})) {
      if (!name || components.has(name) || app.component(name) !== undefined) {
        throw new Error(`Plugin component name collision: ${name}`);
      }
      components.add(name);
    }
  }
  return plugins.map(nameAnonymousRoutes);
}

/** In-process startup transaction. Hook side effects are undone by destroy hooks. */
export class PluginRuntime {
  private loaded: PluginModule[] = [];
  private statuses: PluginStatus[] = [];
  private initialized: PluginModule[] = [];
  private routeRemovers: (() => void)[] = [];
  private ownedRoutes: { name: string | symbol; path: string }[] = [];
  private componentNames: string[] = [];
  private app: PluginApp | undefined;
  private router: PluginRouter | undefined;
  private installing: Promise<PluginStatus[]> | undefined;
  private uninstalling: Promise<void> | undefined;
  private installed = false;
  private generation = 0;

  constructor(private readonly registry: PluginRegistry) {}

  getLoadedPlugins(): PluginModule[] {
    return this.loaded.map(clonePlugin);
  }
  getStatuses(): PluginStatus[] {
    return this.statuses.map(status => ({ ...status }));
  }
  getRoutes(): RouteRecordRaw[] {
    return cloneRoutes(this.loaded.flatMap(plugin => plugin.routes ?? []));
  }

  async install(app: PluginApp, router: PluginRouter): Promise<PluginStatus[]> {
    if (this.uninstalling)
      throw new Error(
        "Plugin runtime is uninstalling; await cleanup before reinstalling"
      );
    if (this.app && (this.app !== app || this.router !== router))
      throw new Error(
        "Plugin runtime cannot be installed into a different host"
      );
    if (this.installed) return this.getStatuses();
    if (this.installing) {
      await this.installing;
      return this.getStatuses();
    }
    this.app = app;
    this.router = router;
    const generation = this.generation;
    // Deferral stores the singleflight promise before user callbacks can run.
    this.installing = Promise.resolve().then(() =>
      this.start(app, router, generation)
    );
    try {
      await this.installing;
      return this.getStatuses();
    } finally {
      this.installing = undefined;
    }
  }

  private async start(
    app: PluginApp,
    router: PluginRouter,
    generation: number
  ): Promise<PluginStatus[]> {
    let plugins: PluginModule[] = [];
    let current: string | undefined;
    try {
      this.assertGeneration(generation);
      plugins = preflight(this.registry.freeze(), app, router);
      this.assertGeneration(generation);
      this.statuses = plugins.map(plugin => ({
        name: plugin.name,
        loaded: false
      }));
      for (const plugin of plugins) {
        current = plugin.name;
        this.assertGeneration(generation);
        this.initialized = [...this.initialized, plugin];
        for (const [name, component] of Object.entries(
          plugin.components ?? {}
        )) {
          this.componentNames = [...this.componentNames, name];
          app.component(name, component);
        }
        for (const route of plugin.routes ?? []) {
          this.routeRemovers = [
            ...this.routeRemovers,
            router.addRoute(cloneRoutes([route])[0])
          ];
        }
        const named = createRouterMatcher(plugin.routes ?? [], {})
          .getRoutes()
          .map(matcher => ({
            name: matcher.record.name,
            path: matcher.record.path
          }));
        this.ownedRoutes = [...this.ownedRoutes, ...named];
        await plugin.install?.(app as App);
        this.assertGeneration(generation);
        await plugin.setup?.();
        this.assertGeneration(generation);
      }
      this.loaded = plugins.map(clonePlugin);
      this.statuses = plugins.map(plugin => ({
        name: plugin.name,
        loaded: true
      }));
      this.installed = true;
      return this.getStatuses();
    } catch (error) {
      await this.cleanup();
      this.statuses = plugins.map(plugin => ({
        name: plugin.name,
        loaded: false,
        error:
          plugin.name === current
            ? error instanceof Error
              ? error.message
              : String(error)
            : "Startup transaction rolled back"
      }));
      throw error;
    }
  }

  private assertGeneration(generation: number): void {
    if (this.generation !== generation)
      throw new Error("Plugin startup cancelled by uninstall");
  }

  private async cleanup(): Promise<void> {
    for (const plugin of [...this.initialized].reverse()) {
      try {
        await plugin.destroy?.();
      } catch (error) {
        console.error(`[Plugin] Cleanup failed for ${plugin.name}`, error);
      }
    }
    for (const remove of [...this.routeRemovers].reverse()) {
      try {
        remove();
      } catch (error) {
        console.error("[Plugin] Route cleanup failed", error);
      }
    }
    // Resetting a router creates new matchers, so old removal callbacks are stale.
    for (const route of [...this.ownedRoutes].reverse()) {
      if (
        this.router
          ?.getRoutes()
          .some(
            record => record.name === route.name && record.path === route.path
          )
      ) {
        try {
          this.router.removeRoute(route.name);
        } catch (error) {
          console.error("[Plugin] Restored route cleanup failed", error);
        }
      }
    }
    if (this.app) {
      const components = { ...this.app._context.components };
      for (const name of this.componentNames) delete components[name];
      this.app._context.components = components;
    }
    this.loaded = [];
    this.initialized = [];
    this.routeRemovers = [];
    this.ownedRoutes = [];
    this.componentNames = [];
    this.installed = false;
  }

  async uninstall(): Promise<void> {
    if (this.uninstalling) return this.uninstalling;
    this.generation++;
    const pending = this.installing;
    this.uninstalling = Promise.resolve().then(async () => {
      if (pending) {
        try {
          await pending;
        } catch {
          /* start() already rolled back the failed or cancelled transaction. */
        }
      }
      await this.cleanup();
      this.statuses = [];
      this.app = undefined;
      this.router = undefined;
    });
    try {
      await this.uninstalling;
    } finally {
      this.uninstalling = undefined;
    }
  }
}
