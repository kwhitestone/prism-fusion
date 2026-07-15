import { createPinia } from "pinia";
import type { App } from "vue";

export const store = createPinia();

// 导出所有store模块（注释掉不兼容的导出）
// export { useAuthStore } from './modules/user'  // 已移除，不兼容admin
export { useUserStore } from "./modules/user"; // 重新启用 useUserStore 导出
export { useAppStore } from "./modules/app-web"; // 使用重命名的文件

// Pinia 插件安装函数
export const setupPinia = (app: App<Element>): void => {
  app.use(store);
};
