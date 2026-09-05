import type { RouteRecordRaw } from "vue-router";
import type { PluginDependency, PluginManifest, PluginModule } from "./types";

const canonicalID = /^[a-z0-9](?:[a-z0-9._-]*[a-z0-9])?$/;
const semver =
  /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$/;

function isRecord(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

/** Copy data, retaining opaque component instances and executable references. */
function cloneData<T>(value: T, seen = new WeakMap<object, unknown>()): T {
  if (value === null || typeof value !== "object") return value;
  if (seen.has(value)) return seen.get(value) as T;
  if (Array.isArray(value)) {
    const result = [];
    seen.set(value, result);
    result.push(...value.map(item => cloneData(item, seen)));
    return result as T;
  }
  if (Object.getPrototypeOf(value) !== Object.prototype) return value;
  const result = {};
  seen.set(value, result);
  for (const [key, item] of Object.entries(value)) {
    Object.defineProperty(result, key, {
      value: cloneData(item, seen),
      enumerable: true,
      writable: true,
      configurable: true
    });
  }
  return result as T;
}

export function cloneRoutes(
  routes: readonly RouteRecordRaw[] = []
): RouteRecordRaw[] {
  return routes.map(route => {
    const result = { ...route } as RouteRecordRaw;
    if (route.meta) result.meta = cloneData(route.meta);
    if (route.alias) result.alias = cloneData(route.alias);
    if (route.props) result.props = cloneData(route.props);
    if (route.children) result.children = cloneRoutes(route.children);
    if ("components" in route && route.components)
      result.components = { ...route.components };
    if ("beforeEnter" in route && Array.isArray(route.beforeEnter))
      result.beforeEnter = [...route.beforeEnter];
    if (typeof route.redirect === "object")
      result.redirect = cloneData(route.redirect);
    return result;
  });
}

export function clonePlugin(plugin: PluginModule): PluginModule {
  return {
    ...plugin,
    ...(plugin.manifest ? { manifest: cloneData(plugin.manifest) } : {}),
    ...(plugin.routes ? { routes: cloneRoutes(plugin.routes) } : {}),
    ...(plugin.permissions
      ? { permissions: cloneData(plugin.permissions) }
      : {}),
    ...(plugin.components ? { components: { ...plugin.components } } : {})
  };
}

function validateID(value: unknown, label: string): asserts value is string {
  if (typeof value !== "string" || !canonicalID.test(value)) {
    throw new Error(`${label} must be a canonical plugin ID`);
  }
}

function validateDependencies(value: unknown, label: string): void {
  if (value === undefined) return;
  if (!Array.isArray(value)) throw new Error(`${label} must be an array`);
  for (const dependency of value) {
    if (!isRecord(dependency))
      throw new Error(`${label} must contain dependency objects`);
    validateID(dependency.id, `${label} dependency ID`);
    if (dependency.version !== undefined && dependency.version !== "") {
      throw new Error(
        `${label} dependency version constraints are not supported in V2.0`
      );
    }
  }
}

export function validateRouteScope(scope: unknown): asserts scope is string {
  if (
    typeof scope !== "string" ||
    !scope.startsWith("/") ||
    scope === "/" ||
    scope.endsWith("/") ||
    scope.includes("//") ||
    /[\\\s:*?+#%()[\]{}]/.test(scope) ||
    scope.split("/").some(segment => segment === "." || segment === "..")
  ) {
    throw new Error(`Invalid plugin route scope: ${String(scope)}`);
  }
}

function validateManifest(
  manifest: unknown,
  name: string
): asserts manifest is PluginManifest {
  if (!isRecord(manifest))
    throw new Error(`Plugin ${name} manifest must be an object`);
  validateID(name, "Plugin name");
  if (
    manifest.apiVersion !== "prism-fusion/v2" ||
    manifest.kind !== "frontend-addon"
  ) {
    throw new Error(
      `Plugin ${name} has unsupported manifest apiVersion or kind`
    );
  }
  if (manifest.id !== name)
    throw new Error(`Plugin ${name} manifest ID must match name`);
  const match =
    typeof manifest.version === "string" && semver.exec(manifest.version);
  if (!match || match[4]?.split(".").some(part => /^0\d+$/.test(part))) {
    throw new Error(
      `Plugin ${name} manifest version must be semantic versioning`
    );
  }
  validateDependencies(manifest.requires, `${name} requires`);
  validateDependencies(manifest.optional, `${name} optional`);
  if (manifest.conflicts !== undefined) {
    if (!Array.isArray(manifest.conflicts))
      throw new Error(`${name} conflicts must be an array`);
    manifest.conflicts.forEach(id => validateID(id, `${name} conflict`));
  }
  if (manifest.routeScopes !== undefined) {
    if (!Array.isArray(manifest.routeScopes))
      throw new Error(`${name} routeScopes must be an array`);
    manifest.routeScopes.forEach(validateRouteScope);
  }
}

function validatePlugin(plugin: PluginModule): void {
  if (
    !isRecord(plugin) ||
    typeof plugin.name !== "string" ||
    !plugin.name ||
    plugin.name.trim() !== plugin.name
  ) {
    throw new Error("Plugin must be an object with a nonblank, unpadded name");
  }
  for (const key of ["description", "version"] as const) {
    if (plugin[key] !== undefined && typeof plugin[key] !== "string")
      throw new Error(`${plugin.name} ${key} must be a string`);
  }
  if (plugin.priority !== undefined && !Number.isSafeInteger(plugin.priority))
    throw new Error(`${plugin.name} priority must be a safe integer`);
  if (
    plugin.enabled !== undefined &&
    typeof plugin.enabled !== "boolean" &&
    typeof plugin.enabled !== "function"
  ) {
    throw new Error(`${plugin.name} enabled must be a boolean or callback`);
  }
  for (const key of ["install", "setup", "destroy"] as const) {
    if (plugin[key] !== undefined && typeof plugin[key] !== "function")
      throw new Error(`${plugin.name} ${key} must be a function`);
  }
  for (const key of ["routes", "permissions"] as const) {
    if (plugin[key] !== undefined && !Array.isArray(plugin[key]))
      throw new Error(`${plugin.name} ${key} must be an array`);
  }
  for (const permission of plugin.permissions ?? []) {
    if (
      !isRecord(permission) ||
      typeof permission.key !== "string" ||
      !/^[^:\s]+:[^:\s]+:[^:\s]+$/.test(permission.key) ||
      typeof permission.name !== "string" ||
      !permission.name.trim() ||
      (permission.description !== undefined &&
        typeof permission.description !== "string")
    ) {
      throw new Error(
        `${plugin.name} permissions must declare a three-part key and a nonblank name`
      );
    }
  }
  if (
    plugin.components !== undefined &&
    (!isRecord(plugin.components) ||
      Object.values(plugin.components).some(
        value => !value || !["object", "function"].includes(typeof value)
      ))
  ) {
    throw new Error(`${plugin.name} components must be a component map`);
  }
  if (plugin.manifest !== undefined) {
    validateManifest(plugin.manifest, plugin.name);
    if (
      plugin.version !== undefined &&
      plugin.version !== plugin.manifest.version
    ) {
      throw new Error(`${plugin.name} module and manifest versions must match`);
    }
  }
}

function activationSnapshot(plugins: readonly PluginModule[]): PluginModule[] {
  return plugins.map(plugin => {
    const enabled =
      typeof plugin.enabled === "function"
        ? plugin.enabled()
        : (plugin.enabled ?? true);
    if (typeof enabled !== "boolean")
      throw new Error(
        `Plugin ${plugin.name} enabled callback must return a boolean`
      );
    return { ...clonePlugin(plugin), enabled };
  });
}

function dependencies(
  plugin: PluginModule,
  active: ReadonlyMap<string, PluginModule>
): string[] {
  const required = plugin.manifest?.requires ?? [];
  for (const dependency of required) {
    if (!active.has(dependency.id))
      throw new Error(
        `Plugin ${plugin.name} dependency ${dependency.id} is missing or disabled`
      );
  }
  const optional = (plugin.manifest?.optional ?? []).filter(dependency =>
    active.has(dependency.id)
  );
  return [
    ...new Set(
      [...required, ...optional].map(
        (dependency: PluginDependency) => dependency.id
      )
    )
  ];
}

function resolvePlugins(plugins: readonly PluginModule[]): PluginModule[] {
  const active = new Map(
    plugins
      .filter(plugin => plugin.enabled)
      .map(plugin => [plugin.name, plugin])
  );
  const pending = new Map<string, string[]>();
  for (const plugin of active.values()) {
    for (const conflict of plugin.manifest?.conflicts ?? []) {
      if (active.has(conflict))
        throw new Error(`Plugin ${plugin.name} conflicts with ${conflict}`);
    }
    pending.set(plugin.name, dependencies(plugin, active));
  }
  const result: PluginModule[] = [];
  while (pending.size) {
    const available = [...pending.entries()].filter(([, deps]) =>
      deps.every(id => !pending.has(id))
    );
    available.sort(
      ([a], [b]) =>
        (active.get(a).priority ?? 100) - (active.get(b).priority ?? 100) ||
        (a < b ? -1 : a > b ? 1 : 0)
    );
    if (!available.length)
      throw new Error(
        `Plugin dependency cycle: ${[...pending.keys()].sort().join(", ")}`
      );
    const [id] = available[0];
    result.push(active.get(id));
    pending.delete(id);
  }
  return result;
}

/** Registration snapshots metadata; freeze captures activation and startup order. */
export class PluginRegistry {
  private registered: PluginModule[] = [];
  private captured: PluginModule[] | undefined;
  private resolved: PluginModule[] | undefined;
  private resolving = false;

  get isFrozen(): boolean {
    return this.resolved !== undefined;
  }

  register(plugin: PluginModule): void {
    if (this.isFrozen || this.resolving)
      throw new Error(
        "Plugin registry is frozen or resolving; late registration is not allowed"
      );
    validatePlugin(plugin);
    if (this.registered.some(existing => existing.name === plugin.name))
      throw new Error(`Duplicate plugin ID: ${plugin.name}`);
    this.registered = [...this.registered, clonePlugin(plugin)];
  }

  getPlugins(): PluginModule[] {
    return (this.captured ?? this.registered).map(clonePlugin);
  }

  resolve(): PluginModule[] {
    if (this.resolved) return this.resolved.map(clonePlugin);
    if (this.resolving) throw new Error("Plugin registry is already resolving");
    this.resolving = true;
    try {
      return resolvePlugins(activationSnapshot(this.registered)).map(
        clonePlugin
      );
    } finally {
      this.resolving = false;
    }
  }

  freeze(): PluginModule[] {
    if (!this.isFrozen) {
      if (this.resolving)
        throw new Error("Plugin registry is already resolving");
      this.resolving = true;
      try {
        const captured = activationSnapshot(this.registered);
        const resolved = resolvePlugins(captured);
        this.captured = captured;
        this.resolved = resolved;
      } finally {
        this.resolving = false;
      }
    }
    return this.resolved.map(clonePlugin);
  }
}
