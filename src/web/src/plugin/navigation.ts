import type { RouteRecordRaw, RouteMeta } from "vue-router";
import { copyRoutes } from "./host-routes.js";

const MAX_REMOTE_NODES = 10_000;
const MAX_REMOTE_DEPTH = 64;
type DisplayMetadata = Partial<Pick<RouteMeta, "title" | "icon" | "rank" | "showLink">>;

function resolvedPath(value: unknown, parent: string): string | undefined {
  if (typeof value !== "string" || /[?#\\\s]/.test(value) || value.includes("://") || value.includes("//")) return undefined;
  if (value.split("/").some(segment => segment === "." || segment === "..")) return undefined;
  if (!value.startsWith("/") && !parent) return undefined;
  const joined = value.startsWith("/") ? value : `${parent.replace(/\/$/, "")}/${value}`;
  return joined.replace(/\/+$/, "") || "/";
}

function displayMetadata(value: unknown): DisplayMetadata {
  if (!value || typeof value !== "object" || Array.isArray(value)) return {};
  const meta = value as Record<string, unknown>;
  return {
    ...(typeof meta.title === "string" && meta.title.length <= 512 ? { title: meta.title } : {}),
    ...(typeof meta.icon === "string" && meta.icon.length <= 128 ? { icon: meta.icon } : {}),
    ...(typeof meta.rank === "number" && Number.isFinite(meta.rank) ? { rank: meta.rank } : {}),
    ...(typeof meta.showLink === "boolean" ? { showLink: meta.showLink } : {})
  };
}

function remoteMetadata(input: unknown): ReadonlyMap<string, DisplayMetadata> {
  const metadata = new Map<string, DisplayMetadata>();
  const seen = new WeakSet<object>();
  let remaining = MAX_REMOTE_NODES;
  const visit = (nodes: unknown, parent: string, depth: number): void => {
    if (!Array.isArray(nodes) || depth > MAX_REMOTE_DEPTH) return;
    for (const node of nodes) {
      if (--remaining < 0) return;
      if (!node || typeof node !== "object" || seen.has(node)) continue;
      seen.add(node);
      const path = resolvedPath(node.path, parent);
      if (path && !metadata.has(path)) metadata.set(path, displayMetadata(node.meta));
      visit(node.children, path ?? "", depth + 1);
    }
  };
  visit(input, "", 0);
  return metadata;
}

/** Backend menus decorate declared navigation; they never create routes or grant access. */
export function mergeNavigationMetadata(
  declared: readonly RouteRecordRaw[],
  backendMenus: unknown
): RouteRecordRaw[] {
  const metadata = remoteMetadata(backendMenus);
  const merge = (routes: RouteRecordRaw[], parent: string): RouteRecordRaw[] => routes.map(route => {
    const path = resolvedPath(route.path, parent);
    const display = path ? metadata.get(path) : undefined;
    return {
      ...route,
      ...(display && Object.keys(display).length > 0 ? { meta: { ...route.meta, ...display } } : {}),
      ...(route.children ? { children: merge(route.children, path ?? "") } : {})
    } as RouteRecordRaw;
  });
  return merge(copyRoutes(declared), "");
}
