// 应用全局配置接口
interface AppConfig {
  appName: string;
  version: string;
  description: string;
  author: string;
  homePage: string;
  github: string;
  documentation: string;
  apiPrefix: string;
  tokenKey: string;
  timeout: number;
}

// 应用全局配置
export const config: AppConfig = {
  appName: import.meta.env.VITE_APP_TITLE || "Prism Fusion",
  version: "1.0.0",
  description: "A pure, plugin-based full-stack framework",
  author: "kwhitestone",
  homePage: "https://github.com/kwhitestone/prism-fusion",
  github: "https://github.com/kwhitestone/prism-fusion",
  documentation: "https://github.com/kwhitestone/prism-fusion/wiki",
  apiPrefix: import.meta.env.VITE_BASE_API || "/api",
  tokenKey: "x-token",
  timeout: 10000
};

// 环境变量配置
export const env = {
  NODE_ENV: import.meta.env.NODE_ENV,
  VITE_APP_TITLE: import.meta.env.VITE_APP_TITLE,
  VITE_CLI_PORT: import.meta.env.VITE_CLI_PORT,
  VITE_BASE_API: import.meta.env.VITE_BASE_API,
  VITE_BASE_PATH: import.meta.env.VITE_BASE_PATH,
  VITE_SERVER_PORT: import.meta.env.VITE_SERVER_PORT
};

// 开发模式检查
export const isDev = (): boolean => import.meta.env.DEV;

// 生产模式检查
export const isProd = (): boolean => import.meta.env.PROD;

// 获取完整API地址
export const getApiUrl = (path: string = ""): string => {
  const baseUrl = `${env.VITE_BASE_PATH}:${env.VITE_SERVER_PORT}`;
  const apiPrefix = env.VITE_BASE_API;
  return `${baseUrl}${apiPrefix}${path}`;
};
