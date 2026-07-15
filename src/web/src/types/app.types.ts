import type { RouteRecordRaw } from "vue-router";

// 应用主题类型
export type ThemeType = "light" | "dark" | "auto";

// 布局模式类型
export type LayoutMode = "default" | "classic" | "sidebar" | "head";

// 语言类型
export type Language = "zh-cn" | "en" | "ja";

// 组件尺寸类型
export type ComponentSize = "large" | "default" | "small";

// 应用配置接口
export interface AppConfig {
  theme: ThemeType;
  size: ComponentSize;
  language: Language;
  layout: LayoutMode;
  sidebarCollapse: boolean;
  enableTabs: boolean;
  showBreadcrumb: boolean;
  enableProgress: boolean;
  enableKeepAlive: boolean;
  enableWatermark: boolean;
  primaryColor: string;
  grayMode: boolean;
  colorWeakness: boolean;
}

// 路由菜单类型
export interface RouteMenu {
  id: string;
  path: string;
  name: string;
  title: string;
  icon?: string;
  component?: string;
  redirect?: string;
  hidden?: boolean;
  disabled?: boolean;
  keepAlive?: boolean;
  children?: RouteMenu[];
  meta?: RouteMeta;
}

// 路由元信息
export interface RouteMeta {
  title: string;
  icon?: string;
  keepAlive?: boolean;
  requiresAuth?: boolean;
  roles?: string[];
  permissions?: string[];
  hidden?: boolean;
  disabled?: boolean;
  activeMenu?: string;
  noCache?: boolean;
  breadcrumb?: boolean;
  affix?: boolean;
}

// 动态路由类型
export interface AsyncRoute extends Omit<RouteRecordRaw, "children"> {
  children?: AsyncRoute[];
}

// 标签页类型
export interface TabView {
  title: string;
  name: string;
  path: string;
  fullPath: string;
  query?: Record<string, string | number>;
  params?: Record<string, string | number>;
  closable?: boolean;
  affix?: boolean;
  icon?: string;
}

// 面包屑类型
export interface BreadcrumbItem {
  title: string;
  path?: string;
  icon?: string;
}

// 操作记录类型
export interface OperationRecord {
  id: number;
  ip: string;
  method: string;
  path: string;
  status: number;
  latency: number;
  agent: string;
  errorMessage: string;
  body: string;
  resp: string;
  userID: number;
  user: {
    userName: string;
    nickName: string;
    headerImg: string;
  };
  createdAt: string;
}

// 系统状态类型
export interface SystemStatus {
  cpu: {
    cores: number;
    usage: number;
  };
  memory: {
    total: number;
    used: number;
    usage: number;
  };
  disk: {
    total: number;
    used: number;
    usage: number;
  };
  network: {
    upload: number;
    download: number;
  };
}

// 通知消息类型
export interface NotificationItem {
  id: string;
  type: "info" | "success" | "warning" | "error";
  title: string;
  content: string;
  timestamp: number;
  read: boolean;
  avatar?: string;
}

// 搜索建议类型
export interface SearchSuggestion {
  key: string;
  value: string;
  type: "menu" | "function" | "data";
  icon?: string;
  description?: string;
}
