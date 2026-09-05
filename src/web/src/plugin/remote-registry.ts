import { createRouterMatcher, type RouteRecordRaw } from "vue-router";
import { PluginRegistry } from "./registry.js";
import { preflightPluginContributions } from "./runtime.js";
import type { PluginDependency, PluginModule } from "./types";

export interface RemoteApplicationRoute {
  path: string;
  name: string;
  /** Child-local Vue Router path template; only parameters declared in path. */
  childPath: string;
}

export interface RemoteMessageCapabilities {
  toChild: string[];
  fromChild: string[];
}

export interface RemoteApplicationManifest {
  apiVersion: "prism-fusion/v2";
  kind: "remote-application";
  id: string;
  version: string;
  entryUrl: string;
  allowedOrigins: string[];
  protocolVersion: 1;
  routeScopes: string[];
  routes: RemoteApplicationRoute[];
  messages: RemoteMessageCapabilities;
  requires?: PluginDependency[];
  optional?: PluginDependency[];
  conflicts?: string[];
}

export interface ResolvedRemoteRoute {
  application: RemoteApplicationManifest;
  route: RemoteApplicationRoute;
  childPath: string;
}

const copy = <T>(value: T): T => structuredClone(value);
const safePath = (value: unknown): value is string => typeof value === "string" &&
  value.startsWith("/") && !/[?#\\\s%]/.test(value) && !value.includes("//") &&
  !value.split("/").some(part => part === "." || part === "..");

export function validateMessageCapabilities(value: RemoteMessageCapabilities): void {
  if (!value || typeof value !== "object") throw new Error("Remote messages must declare capabilities");
  for (const direction of ["toChild", "fromChild"] as const) {
    const names = value[direction];
    if (!Array.isArray(names) || names.length > 128 || names.some(name =>
      typeof name !== "string" || !/^[a-z][a-z0-9.-]{0,63}$/.test(name)
    ) || new Set(names).size !== names.length) {
      throw new Error(`Invalid remote message capabilities: ${direction}`);
    }
  }
}

function asPlugin(app: RemoteApplicationManifest): PluginModule {
  return {
    name: app.id,
    manifest: {
      apiVersion: app.apiVersion, kind: "frontend-addon", id: app.id, version: app.version,
      routeScopes: app.routeScopes, requires: app.requires, optional: app.optional, conflicts: app.conflicts
    },
    routes: app.routes.map(route => ({ path: route.path, name: route.name, component: {} }))
  };
}

export function validateRemoteApplication(app: RemoteApplicationManifest): void {
  if (!app || app.kind !== "remote-application" || app.protocolVersion !== 1) {
    throw new Error("Invalid remote application kind or protocol version");
  }
  const url = new URL(app.entryUrl);
  if (!["http:", "https:"].includes(url.protocol) || url.username || url.password || url.hash) {
    throw new Error("Remote application URL must be HTTP(S) without credentials or a fragment");
  }
  if (!Array.isArray(app.allowedOrigins) || app.allowedOrigins.length === 0 ||
    app.allowedOrigins.some(origin => {
      try { return new URL(origin).origin !== origin || !/^https?:\/\//.test(origin); }
      catch { return true; }
    }) || !app.allowedOrigins.includes(url.origin)) {
    throw new Error("Remote application origin must be explicitly allowed");
  }
  validateMessageCapabilities(app.messages);
  if (!Array.isArray(app.routes) || !app.routes.length || app.routes.length > 256) {
    throw new Error("Remote application must declare bounded routes");
  }
  for (const route of app.routes) {
    if (!route || !safePath(route.path) || !safePath(route.childPath) || typeof route.name !== "string" || !route.name) {
      throw new Error("Invalid remote route or child path");
    }
    const declared = new Set([...route.path.matchAll(/:([a-zA-Z0-9_]+)/g)].map(match => match[1]));
    if ([...route.childPath.matchAll(/:([a-zA-Z0-9_]+)/g)].some(match => !declared.has(match[1]))) {
      throw new Error("Remote child path uses an undeclared parameter");
    }
  }
  const registry = new PluginRegistry();
  registry.register(asPlugin(app));
}

/** Separate from the in-bundle addon registry: no iframe is loaded during validation. */
export class RemoteApplicationRegistry {
  private applications: RemoteApplicationManifest[] = [];
  private frozen: RemoteApplicationManifest[] | undefined;

  register(applications: readonly RemoteApplicationManifest[]): void {
    if (this.frozen) throw new Error("Remote application registry is frozen");
    const next = [...this.applications, ...copy(applications)];
    const registry = new PluginRegistry();
    for (const app of next) {
      validateRemoteApplication(app);
      registry.register(asPlugin(app));
    }
    preflightPluginContributions(next.map(asPlugin), {
      component: () => undefined
    }, { getRoutes: () => [] });
    this.applications = next;
  }

  freeze(): RemoteApplicationManifest[] {
    if (!this.frozen) {
      const registry = new PluginRegistry();
      for (const app of this.applications) registry.register(asPlugin(app));
      const byID = new Map(this.applications.map(app => [app.id, app]));
      this.frozen = registry.freeze().map(plugin => byID.get(plugin.name)!);
    }
    return copy(this.frozen);
  }

  getApplications(): RemoteApplicationManifest[] {
    return copy(this.frozen ?? this.applications);
  }

  resolve(path: string): ResolvedRemoteRoute | undefined {
    if (!safePath(path)) return undefined;
    const applications = this.freeze();
    const records = applications.flatMap(application => application.routes.map(route => ({
      path: route.path, name: route.name, component: {}, meta: { title: route.name, application, route }
    })));
    const matcher = createRouterMatcher(records as RouteRecordRaw[], {});
    const match = matcher.resolve({ path }, { path: "/", name: undefined, params: {}, matched: [], meta: { title: "" } });
    const record = match.matched[0];
    if (!record) return undefined;
    const application = record.meta.application as RemoteApplicationManifest;
    const route = record.meta.route as RemoteApplicationRoute;
    const childPath = route.childPath.replace(/:([a-zA-Z0-9_]+)(?:\([^)]*\))?[?*+]?/g, (_part, name: string) => {
      const value = match.params[name];
      return (Array.isArray(value) ? value : value ? String(value).split("/") : []).map(encodeURIComponent).join("/");
    }).replace(/\/+$/, "") || "/";
    return copy({ application, route, childPath });
  }
}
