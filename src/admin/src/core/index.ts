/**
 * Prism Fusion Admin 框架核心导出
 *
 * 业务项目通过此入口引用框架能力，无需直接依赖内部模块路径
 * 使用方式: import { ... } from "prism-fusion-admin"
 */

// ========== 核心组件 ==========
export { default as App } from "../App.vue";

// ========== 路由 ==========
export {
  default as router,
  resetRouter,
  resetLoadedPaths,
  registerExternalRoutes
} from "../router";
export {
  ascending,
  getTopMenu,
  initRouter,
  getHistoryMode,
  findRouteByPath,
  handleAliveRoute,
  formatTwoStageRoutes,
  formatFlatteningRoutes
} from "../router/utils";

// ========== 状态管理 ==========
export { setupStore, store } from "../store";

// ========== 配置 ==========
export {
  getConfig,
  setConfig,
  getPlatformConfig,
  responsiveStorageNameSpace
} from "../config";

// ========== 登录 UI ==========
export { setFooterComponent } from "../store/modules/loginUI";

// ========== 插件系统 ==========
export {
  installPlugins,
  registerExternalPlugins,
  getPlugins,
  getLoadedPlugins,
  getPluginStatuses,
  getPluginRoutes,
  uninstallPlugins
} from "../plugin/loader";

export type {
  PluginModule,
  PluginStatus,
  PluginPermission,
  ReportMenuItem,
  PluginRegistryEntry,
  PluginRegistryPayload
} from "../plugin/types";

// ========== 工具 ==========
export { injectResponsiveStorage } from "../utils/responsive";

// ========== Layout (供业务 addons 使用) ==========
export const Layout = () => import("../layout/index.vue");
