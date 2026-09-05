import type { App } from "vue";
import type { Router, RouteRecordRaw } from "vue-router";
import type { PluginModule, PluginStatus, ReportMenuItem, PluginRegistryPayload } from "./types";
import { router as frameworkRouter, commitPluginRoutes, clearPluginRoutes } from "@/router/index";
import { PluginRegistry } from "./registry";
import { PluginRuntime } from "./runtime";
import service from "@/utils/request";

const pluginModules = import.meta.glob<{ default: PluginModule }>(
  "../addons/*/index.ts", { eager: true }
);

// Lazy construction lets eager addon imports finish evaluating first.
let registry: PluginRegistry | undefined;
let runtime: PluginRuntime | undefined;
let started = false;
let installing: Promise<PluginStatus[]> | undefined;
let uninstalling: Promise<void> | undefined;
let host: { app: App; router: Router } | undefined;
let committed = false;
let generation = 0;

function getRegistry(): PluginRegistry {
  if (!registry) {
    const candidate = new PluginRegistry();
    Object.values(pluginModules).forEach(module => candidate.register(module.default));
    registry = candidate;
  }
  return registry;
}

/** Add a batch atomically before startup. Duplicate IDs never replace builtins. */
export function registerExternalPlugins(plugins: PluginModule[]): void {
  if (started) throw new Error("Plugin registration is frozen after startup begins");
  const candidate = new PluginRegistry();
  [...getRegistry().getPlugins(), ...plugins].forEach(plugin => candidate.register(plugin));
  registry = candidate;
}

export function getPlugins(): PluginModule[] { return getRegistry().getPlugins(); }
export function getLoadedPlugins(): PluginModule[] { return runtime?.getLoadedPlugins() ?? []; }
export function getPluginStatuses(): PluginStatus[] { return runtime?.getStatuses() ?? []; }
export function getPluginRoutes(): RouteRecordRaw[] { return runtime?.getRoutes() ?? []; }

/** Menus become visible only after the startup transaction succeeds. */
export async function installPlugins(app: App, router: Router): Promise<PluginStatus[]> {
  if (router !== frameworkRouter) throw new Error("installPlugins requires the framework router");
  if (uninstalling) throw new Error("Await plugin cleanup before installing");
  if (host && (host.app !== app || host.router !== router)) {
    throw new Error("Plugin runtime cannot be installed into a different host");
  }
  if (committed) return getPluginStatuses();
  if (installing) { await installing; return getPluginStatuses(); }
  host = { app, router };
  started = true;
  runtime ??= new PluginRuntime(getRegistry());
  const currentGeneration = generation;
  installing = Promise.resolve().then(async () => {
    if (currentGeneration !== generation) throw new Error("Plugin startup cancelled");
    await runtime.install(app, router);
    if (currentGeneration !== generation) throw new Error("Plugin startup cancelled");
    try {
      commitPluginRoutes(runtime.getRoutes());
      committed = true;
      return runtime.getStatuses();
    } catch (error) {
      await runtime.uninstall();
      clearPluginRoutes();
      throw error;
    }
  });
  try { await installing; return getPluginStatuses(); }
  finally { installing = undefined; }
}

function extractMenus(routes: RouteRecordRaw[] = []): ReportMenuItem[] {
  return routes.map(route => ({
    path: route.path,
    name: typeof route.name === "string" ? route.name : undefined,
    title: route.meta?.title,
    icon: typeof route.meta?.icon === "string" ? route.meta.icon : undefined,
    rank: typeof route.meta?.rank === "number" ? route.meta.rank : undefined,
    showLink: route.meta?.showLink,
    ...(route.children?.length ? { children: extractMenus(route.children) } : {})
  }));
}

/** Reporting is observability, not part of the startup transaction. */
async function reportPluginRegistry(plugins: PluginModule[]): Promise<void> {
  const payload: PluginRegistryPayload = {
    plugins: plugins.map(plugin => ({
      name: plugin.name, description: plugin.description, version: plugin.manifest?.version ?? plugin.version,
      menus: extractMenus(plugin.routes), permissions: plugin.permissions ?? []
    }))
  };
  try {
    await service({ url: "/api/v1/system/plugin-registry", method: "post",
      data: payload, donNotShowLoading: true });
  } catch (error) {
    console.warn("[Plugin] Failed to report registry:", error);
  }
}

export function triggerPluginRegistryReport(): void {
  void reportPluginRegistry(getLoadedPlugins());
}

export async function uninstallPlugins(): Promise<void> {
  if (uninstalling) return uninstalling;
  generation++;
  uninstalling = Promise.resolve().then(async () => {
    await runtime?.uninstall();
    if (installing) { try { await installing; } catch { /* Startup failed closed. */ } }
    clearPluginRoutes();
    committed = false;
    host = undefined;
  });
  try { await uninstalling; }
  finally { uninstalling = undefined; }
}
