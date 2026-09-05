import type { PluginModule } from "@/plugin/types";
import { setAsyncRoutesProvider } from "@/api/routes";
import { getAsyncRoutes } from "./api";
import { getConfig } from "@/config";

let disposeSetup: (() => void) | undefined;

const rbacPlugin: PluginModule = {
  name: "rbac",
  description: "权限管理插件 - 提供动态路由、角色、权限管理",
  version: "2.0.0",
  manifest: {
    apiVersion: "prism-fusion/v2",
    kind: "frontend-addon",
    id: "rbac",
    version: "2.0.0",
    requires: [{ id: "auth" }]
  },
  priority: 20,
  enabled: () => (getConfig()?.RBACProvider || "builtin") === "builtin",

  setup() {
    let active = true;
    // Restore the previous route strategy when this plugin is rolled back.
    const dispose = setAsyncRoutesProvider(async () => {
      if (!active) return { success: false, data: [] };
      try {
        const res = await getAsyncRoutes();
        if (!active) return { success: false, data: [] };
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
    disposeSetup = () => {
      active = false;
      dispose();
    };

    console.log(
      "[Plugin] RBAC plugin setup complete - using real backend routes"
    );
  },

  destroy() {
    const dispose = disposeSetup;
    disposeSetup = undefined;
    dispose?.();
  }
};

export default rbacPlugin;
