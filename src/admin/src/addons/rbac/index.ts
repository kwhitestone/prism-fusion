import type { PluginModule } from "@/plugin/types";
import { setAsyncRoutesProvider } from "@/api/routes";
import { getAsyncRoutes } from "./api";
import { getConfig } from "@/config";

const rbacPlugin: PluginModule = {
  name: "rbac",
  description: "权限管理插件 - 提供动态路由、角色、权限管理",
  version: "1.0.0",

  setup() {
    const provider = getConfig()?.RBACProvider || "builtin";
    if (provider !== "builtin") {
      console.log("[Plugin] RBAC plugin skipped - provider is", provider);
      return;
    }
    // 注入真实动态路由获取策略，替换默认 mock
    setAsyncRoutesProvider(async () => {
      try {
        const res = await getAsyncRoutes();
        // axios 响应拦截器会将 { success, data } 包装为 { code, data: { success, data }, msg }
        const body = res.data;
        const inner = body?.data; // 真正的后端返回体 { success, data: [...] }
        return {
          success: inner?.success ?? body?.success ?? false,
          data: Array.isArray(inner?.data)
            ? inner.data
            : Array.isArray(inner)
              ? inner
              : Array.isArray(body?.data)
                ? body.data
                : []
        };
      } catch {
        console.warn("[RBAC Plugin] Failed to fetch async routes from backend");
        return { success: false, data: [] };
      }
    });

    console.log(
      "[Plugin] RBAC plugin setup complete - using real backend routes"
    );
  }
};

export default rbacPlugin;
