type Result = {
  success: boolean;
  data: Array<any>;
};

// ========== 策略注入 ==========
// 动态路由获取策略（默认返回空，rbac 插件会覆盖为真实后端调用）
type AsyncRoutesProvider = () => Promise<Result>;

const defaultAsyncRoutesProvider: AsyncRoutesProvider = () => {
  return Promise.resolve({ success: true, data: [] });
};

let _asyncRoutesProvider: AsyncRoutesProvider = defaultAsyncRoutesProvider;

/** 设置动态路由获取策略（由 rbac 插件调用） */
export function setAsyncRoutesProvider(provider: AsyncRoutesProvider) {
  _asyncRoutesProvider = provider;
}

export const getAsyncRoutes = (): Promise<Result> => {
  return _asyncRoutesProvider();
};
