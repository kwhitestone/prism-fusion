// 统一导出所有类型定义

// 认证相关类型
export * from "./auth.types";

// API 相关类型
export * from "./api.types";

// 应用相关类型
export * from "./app.types";

// 工具相关类型
export * from "./utils.types";

// Vue 相关类型增强
declare module "vue" {
  interface ComponentCustomProperties {
    $prism: {
      appName: string;
      version: string;
    };
  }
}

// 全局类型声明
declare global {
  interface Window {
    $loadingBar?: any;
    $message?: any;
    $dialog?: any;
    $notification?: any;
  }
}

export {};
