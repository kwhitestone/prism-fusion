import type { App } from "vue";
import type { Router, RouteRecordRaw } from "vue-router";
import { clonePlugin, cloneRoutes, PluginRegistry } from "./registry.js";
import { PluginRuntime, type PluginRouter } from "./runtime.js";
import { HostRoutes } from "./host-routes.js";
import type { PluginModule, PluginStatus } from "./types";

export type { PluginModule, PluginManifest, PluginDependency, PluginPermission, PluginStatus } from "./types";
export { PluginRegistry } from "./registry.js";
export { PluginRuntime } from "./runtime.js";
export { mergeNavigationMetadata } from "./navigation.js";

export interface HeadlessHostOptions {
  app: App;
  router: Router;
  /** Host-owned infrastructure only, normally the root redirect and fallback. */
  coreRoutes?: readonly RouteRecordRaw[];
  homePath?: string;
}

export interface HeadlessPluginHost {
  register(plugins: readonly PluginModule[]): void;
  install(): Promise<PluginStatus[]>;
  uninstall(): Promise<void>;
  restoreRoutes(): void;
  getRoutes(): RouteRecordRaw[];
  getStatuses(): PluginStatus[];
}

/** Per-application adapter over the same V2 runtime used by the framework UI. */
export function createPluginHost(options: HeadlessHostOptions): HeadlessPluginHost {
  const { app, router } = options;
  const coreRoutes = cloneRoutes(options.coreRoutes ?? router.options.routes);
  const baseline = new HostRoutes(coreRoutes);
  baseline.configure({ homePath: options.homePath });
  const registry = new PluginRegistry();
  const runtime = new PluginRuntime(registry);
  // A host fallback is deliberately the lowest-ranked match, not a business
  // namespace claim. Only this exact, explicitly host-declared pattern is exempt.
  const fallbackNames = new Set(coreRoutes
    .filter(route => route.path === "/:pathMatch(.*)*" && route.name !== undefined)
    .map(route => route.name));
  const runtimeRouter: PluginRouter = {
    addRoute: router.addRoute.bind(router),
    removeRoute: router.removeRoute.bind(router),
    hasRoute: router.hasRoute.bind(router),
    getRoutes: () => router.getRoutes().filter(route => !(
      route.path === "/:pathMatch(.*)*" && fallbackNames.has(route.name)
    ))
  };
  let pending: Promise<PluginStatus[]> | undefined;
  let stopping: Promise<void> | undefined;
  let installed = false;
  let started = false;

  const restoreRoutes = () => baseline.restore(router);

  const install = (): Promise<PluginStatus[]> => {
    if (stopping) return Promise.reject(new Error("Plugin host is uninstalling"));
    if (installed) return Promise.resolve(runtime.getStatuses());
    if (pending) return pending;
    started = true;
    pending = Promise.resolve().then(async () => {
      try {
        const statuses = await runtime.install(app, runtimeRouter);
        baseline.commit(runtime.getRoutes());
        restoreRoutes();
        installed = true;
        return statuses;
      } catch (error) {
        // uninstall() may already be waiting for runtime.start; do not wait on
        // the host-level stop promise here, which would create a dependency loop.
        await runtime.uninstall();
        baseline.clear();
        // Runtime rollback removes only its own contributions. Do not clear
        // pre-existing host routes merely because preflight rejected a collision.
        throw error;
      } finally {
        pending = undefined;
      }
    });
    return pending;
  };

  const uninstall = (): Promise<void> => {
    if (stopping) return stopping;
    const installation = pending;
    stopping = Promise.resolve().then(async () => {
      await runtime.uninstall();
      if (installation) await installation.catch(() => undefined);
      baseline.clear();
      restoreRoutes();
      installed = false;
    }).finally(() => { stopping = undefined; });
    return stopping;
  };

  return {
    register(plugins) {
      if (started) throw new Error("Plugin host registration is frozen");
      const candidates = plugins.map(clonePlugin);
      const trial = new PluginRegistry();
      for (const plugin of [...registry.getPlugins(), ...candidates]) trial.register(plugin);
      for (const plugin of candidates) registry.register(plugin);
    },
    install,
    uninstall,
    restoreRoutes,
    getRoutes: () => baseline.getRoutes(),
    getStatuses: () => runtime.getStatuses()
  };
}
