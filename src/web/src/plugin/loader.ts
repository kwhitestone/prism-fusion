import type { App } from "vue";
import type { Router, RouteRecordRaw } from "vue-router";
import type {
  PluginModule,
  PluginStatus,
  ReportMenuItem,
  PluginRegistryPayload
} from "./types";
import { constantMenus } from "@/router/index";
import { http } from "@/utils/http";

/**
 * 自动扫描 addons 目录下所有插件
 * Vite 会在构建时静态分析这个 glob pattern
 */
const pluginModules = import.meta.glob<{ default: PluginModule }>(
  "../addons/*/index.ts",
  { eager: true }
);

/** 外部注入的插件列表（业务项目通过 registerExternalPlugins 注册） */
let externalPlugins: PluginModule[] = [];

/** 已加载的插件列表 */
const loadedPlugins: PluginModule[] = [];

/** 插件加载状态 */
const pluginStatuses: PluginStatus[] = [];

/**
 * 注册外部插件（业务项目在 installPlugins 之前调用）
 * @param plugins 业务插件列表
 */
export function registerExternalPlugins(plugins: PluginModule[]) {
  externalPlugins = plugins;
}

/**
 * 获取所有已发现的插件（内置 + 外部）
 */
export function getPlugins(): PluginModule[] {
  const builtin = Object.values(pluginModules).map(mod => mod.default);
  return [...builtin, ...externalPlugins];
}

/**
 * 获取已加载的插件列表
 */
export function getLoadedPlugins(): PluginModule[] {
  return loadedPlugins;
}

/**
 * 获取插件加载状态
 */
export function getPluginStatuses(): PluginStatus[] {
  return pluginStatuses;
}

/**
 * 获取所有插件路由
 */
export function getPluginRoutes(): RouteRecordRaw[] {
  const routes: RouteRecordRaw[] = [];
  for (const plugin of loadedPlugins) {
    if (plugin.routes && plugin.routes.length > 0) {
      routes.push(...plugin.routes);
    }
  }
  return routes;
}

/**
 * 安装所有插件
 * @param app Vue 应用实例
 * @param router Vue Router 实例
 */
export async function installPlugins(
  app: App,
  router: Router
): Promise<PluginStatus[]> {
  const plugins = getPlugins();

  console.log(`[Plugin] Discovered ${plugins.length} plugin(s)`);

  for (const plugin of plugins) {
    const status: PluginStatus = {
      name: plugin.name,
      loaded: false
    };

    try {
      console.log(`[Plugin] Loading: ${plugin.name}`);

      // 1. 注册全局组件
      if (plugin.components) {
        for (const [name, component] of Object.entries(plugin.components)) {
          app.component(name, component);
          console.log(`[Plugin] Registered component: ${name}`);
        }
      }

      // 2. 注册路由（添加到根路由下）
      if (plugin.routes && plugin.routes.length > 0) {
        for (const route of plugin.routes) {
          router.addRoute(route);
          console.log(`[Plugin] Added route: ${route.path}`);
        }
      }

      // 3. 调用 install 钩子
      if (plugin.install) {
        plugin.install(app);
      }

      // 4. 调用 setup 钩子（异步）
      if (plugin.setup) {
        await plugin.setup();
      }

      status.loaded = true;
      loadedPlugins.push(plugin);
      console.log(`[Plugin] Loaded successfully: ${plugin.name}`);
    } catch (error) {
      status.error = error instanceof Error ? error.message : String(error);
      console.error(`[Plugin] Failed to load ${plugin.name}:`, error);
    }

    pluginStatuses.push(status);
  }

  console.log(
    `[Plugin] Finished loading. Success: ${loadedPlugins.length}, Failed: ${pluginStatuses.filter(s => !s.loaded).length}`
  );

  // ===== 修复: 将外部插件路由注入 constantMenus 以显示菜单 =====
  const externalRoutes: RouteRecordRaw[] = [];
  for (const plugin of externalPlugins) {
    if (plugin.routes && plugin.routes.length > 0) {
      externalRoutes.push(...plugin.routes);
    }
  }
  if (externalRoutes.length > 0) {
    constantMenus.push(...(externalRoutes.flat(Infinity) as any[]));
    // 重新按 rank 排序
    constantMenus.sort((a: any, b: any) => {
      return (a.meta?.rank ?? 99) - (b.meta?.rank ?? 99);
    });
    console.log(
      `[Plugin] Injected ${externalRoutes.length} external route(s) into menus`
    );
  }

  return pluginStatuses;
}

/**
 * 从路由配置中提取菜单元数据（去除 component 等不可序列化字段）
 */
function extractMenus(routes?: RouteRecordRaw[]): ReportMenuItem[] {
  if (!routes || routes.length === 0) return [];
  return routes.flat(Infinity).map((route: any) => {
    const item: ReportMenuItem = {
      path: route.path,
      name: route.name as string,
      title: route.meta?.title,
      icon: route.meta?.icon,
      rank: route.meta?.rank,
      showLink: route.meta?.showLink
    };
    if (route.children && route.children.length > 0) {
      item.children = extractMenus(route.children);
    }
    return item;
  });
}

/**
 * 上报插件注册信息（菜单 + 权限）到后端
 * 每次前端刷新时调用，后端收到后打印记录
 */
async function reportPluginRegistry(plugins: PluginModule[]) {
  const payload: PluginRegistryPayload = {
    plugins: plugins.map(p => ({
      name: p.name,
      description: p.description,
      version: p.version,
      menus: extractMenus(p.routes),
      permissions: p.permissions || []
    }))
  };

  console.log("[Plugin] Reporting registry to backend:", payload);

  try {
    await http.post("/api/v1/system/plugin-registry", { data: payload });
    console.log("[Plugin] Registry reported successfully");
  } catch (error) {
    // 上报失败不影响前端运行
    console.warn("[Plugin] Failed to report registry:", error);
  }
}

/**
 * 登录成功后主动上报插件注册表（需在拿到 token 之后调用）
 */
export function triggerPluginRegistryReport(): void {
  reportPluginRegistry(loadedPlugins);
}

/**
 * 卸载所有插件（用于热更新或清理）
 */
export function uninstallPlugins(): void {
  for (const plugin of loadedPlugins) {
    if (plugin.destroy) {
      try {
        plugin.destroy();
        console.log(`[Plugin] Destroyed: ${plugin.name}`);
      } catch (error) {
        console.error(`[Plugin] Failed to destroy ${plugin.name}:`, error);
      }
    }
  }
  loadedPlugins.length = 0;
  pluginStatuses.length = 0;
}
