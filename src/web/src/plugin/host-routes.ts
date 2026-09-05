import type { Router, RouteRecordRaw } from "vue-router";
import { cloneRoutes } from "./registry.js";

export interface PluginHostOptions {
  /** Explicit shell landing page; must resolve to a declared static route. */
  homePath?: string;
}

/** Copy route data without cloning Vue component definitions/functions. */
export function copyRoutes(routes: readonly RouteRecordRaw[]): RouteRecordRaw[] {
  return cloneRoutes(routes);
}

function staticPaths(routes: readonly RouteRecordRaw[], parent = ""): string[] {
  return routes.flatMap(route => {
    const path = route.path.startsWith("/") ? route.path : `${parent}/${route.path}`.replace(/\/$/, "");
    return [path, ...staticPaths(route.children ?? [], path)];
  });
}

/** The reset baseline is committed only after the entire plugin install succeeds. */
export class HostRoutes {
  private readonly core: RouteRecordRaw[];
  private plugins: RouteRecordRaw[] = [];
  private options: PluginHostOptions = {};
  private committed = false;
  private configured = false;

  constructor(core: readonly RouteRecordRaw[]) {
    this.core = copyRoutes(core);
  }

  configure(options: PluginHostOptions): void {
    if (this.committed || this.configured) throw new Error("Plugin host configuration is frozen");
    const path = options.homePath;
    if (path !== undefined && (!/^\/(?!\/)[^?#\\]*$/.test(path) || path === "/" || path.split("/").some(part => part === "." || part === "..") || /[%:*()]/.test(path))) {
      throw new Error("Plugin host homePath must be a clean static local path");
    }
    this.options = { ...options };
    this.configured = true;
  }

  commit(routes: readonly RouteRecordRaw[]): void {
    const next = copyRoutes(routes);
    const home = this.options.homePath;
    if (home && !staticPaths([...this.core, ...next]).includes(home)) {
      throw new Error(`Plugin host homePath ${home} is not an active declared route`);
    }
    this.plugins = next;
    this.committed = true;
  }

  getRoutes(): RouteRecordRaw[] {
    return copyRoutes([...this.core, ...this.plugins]).map(route =>
      this.committed && this.options.homePath && route.path === "/"
        ? { ...route, redirect: this.options.homePath } as RouteRecordRaw
        : route
    );
  }

  getMenus(): RouteRecordRaw[] {
    return this.getRoutes().sort((a, b) => Number(a.meta?.rank ?? 99) - Number(b.meta?.rank ?? 99));
  }

  restore(router: Pick<Router, "clearRoutes" | "addRoute" | "options">): void {
    const routes = this.getRoutes();
    router.clearRoutes();
    for (const route of routes) router.addRoute(route);
    router.options.routes = copyRoutes(routes);
  }

  clear(): void {
    this.plugins = [];
    this.committed = false;
  }
}
