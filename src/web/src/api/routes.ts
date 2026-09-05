import { createReversibleSlot } from "@/core/reversible-slot";

type Result = {
  success: boolean;
  data: Array<any>;
};

// ========== 策略注入 ==========
// 动态路由获取策略（默认返回空，rbac 插件会覆盖为真实后端调用）
type AsyncRoutesProvider = () => Promise<Result>;

const defaultAsyncRoutesProvider: AsyncRoutesProvider = () => {
  // No provider grants no dynamic metadata; static plugin navigation still works.
  return Promise.resolve({ success: true, data: [] });
};

const providers = createReversibleSlot(defaultAsyncRoutesProvider);

/** 设置动态路由获取策略（由 rbac 插件调用） */
export function setAsyncRoutesProvider(
  provider: AsyncRoutesProvider
): () => void {
  if (typeof provider !== "function")
    throw new TypeError("Route provider must be a function");
  return providers.set(provider);
}

export const getAsyncRoutes = async (): Promise<Result> => {
  const version = providers.version;
  const result = await providers.get()();
  return version === providers.version ? result : { success: false, data: [] };
};
