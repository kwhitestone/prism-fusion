import type { App, Component } from "vue";
import type { RouteRecordRaw } from "vue-router";

/**
 * 插件功能权限声明
 * 每个插件可声明自己提供的功能权限点，用于后续 RBAC 管理
 */
export interface PluginPermission {
  /** 权限唯一标识，格式 "domain:resource:action"，如 "example:item:create" */
  key: string;
  /** 权限名称（人类可读） */
  name: string;
  /** 权限描述（可选） */
  description?: string;
}

/** V2.0 validates dependency IDs; nonempty version constraints are reserved. */
export interface PluginDependency {
  id: string;
  version?: string;
}

export interface PluginManifest {
  apiVersion: "prism-fusion/v2";
  kind: "frontend-addon";
  id: string;
  version: string;
  requires?: PluginDependency[];
  optional?: PluginDependency[];
  conflicts?: string[];
  /** Static absolute prefixes owned by this plugin, e.g. /example. */
  routeScopes?: string[];
}

/**
 * 插件模块接口定义
 * 所有前端插件必须导出一个符合此接口的默认对象
 */
export interface PluginModule {
  /** 插件唯一标识（与后端插件名称对应） */
  name: string;
  /** 插件描述 */
  description?: string;
  /** 插件版本 */
  version?: string;
  /** Omit to retain V1 compatibility. */
  manifest?: PluginManifest;
  /** Lower values load first when dependency ordering does not decide. */
  priority?: number;
  /** Evaluated once when configuration is ready and the registry freezes. */
  enabled?: boolean | (() => boolean);
  /** 插件路由配置 */
  routes?: RouteRecordRaw[];
  /** 插件功能权限声明 */
  permissions?: PluginPermission[];
  /** 全局组件注册 { 组件名: 组件 } */
  components?: Record<string, Component>;
  /** Vue 插件安装钩子 */
  install?: (app: App) => void | Promise<void>;
  /** 插件初始化钩子（异步） */
  setup?: () => void | Promise<void>;
  /** 插件卸载钩子 */
  destroy?: () => void | Promise<void>;
}

/**
 * 插件加载状态
 */
export interface PluginStatus {
  name: string;
  loaded: boolean;
  error?: string;
}

/** 上报到后端的菜单项结构（从路由 meta 中提取） */
export interface ReportMenuItem {
  path: string;
  name?: string;
  title?: string;
  icon?: string;
  rank?: number;
  showLink?: boolean;
  children?: ReportMenuItem[];
}

/** 上报到后端的单个插件注册信息 */
export interface PluginRegistryEntry {
  name: string;
  description?: string;
  version?: string;
  menus: ReportMenuItem[];
  permissions: PluginPermission[];
}

/** 上报到后端的完整插件注册表 */
export interface PluginRegistryPayload {
  plugins: PluginRegistryEntry[];
}
