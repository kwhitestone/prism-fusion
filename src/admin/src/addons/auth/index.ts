import type { PluginModule } from "@/plugin/types";
import { triggerPluginRegistryReport } from "@/plugin/loader";
import {
  setLoginHandler,
  setRefreshHandler,
  setUserInfoHandler
} from "@/store/modules/user";
import { setLoginComponent } from "@/store/modules/loginUI";
import { setToken, setAuthToken } from "@/utils/auth";
import { login, refreshToken, getUserInfo } from "./api";
import type { UserResult } from "@/api/user";
import { getConfig } from "@/config";
import LoginForm from "./components/LoginForm.vue";

const authPlugin: PluginModule = {
  name: "auth",
  description: "认证插件 - 提供用户名密码登录、Token 刷新",
  version: "1.0.0",

  setup() {
    const provider = getConfig()?.AuthProvider || "builtin";
    if (provider !== "builtin") {
      console.log("[Plugin] Auth plugin skipped - provider is", provider);
      return;
    }

    // 注册登录界面组件：用户名 + 密码表单
    setLoginComponent(LoginForm);

    // 注入真实登录策略，替换默认 mock
    setLoginHandler(async data => {
      try {
        const res = await login(data);
        const body = res.data;

        if (body.code !== 0) {
          return {
            success: false,
            message: body.message || "登录失败"
          } as UserResult;
        }

        const { accessToken, refreshToken: rToken, user } = body.data;

        // 设置 auth_token（发送给后端的 Authorization 头）
        setAuthToken(`Bearer ${accessToken}`);

        // 获取用户完整信息（包括角色和权限）
        let roles = ["user"];
        const permissions: string[] = ["*:*:*"];
        try {
          const infoRes = await getUserInfo();
          if (infoRes.data?.code === 0 && infoRes.data?.data) {
            roles = infoRes.data.data.roles || roles;
          }
        } catch {
          // 获取用户信息失败，使用默认值
          if (user?.roleId === 999) {
            roles = ["admin"];
          }
        }

        // 设置 token 数据到 localStorage
        const tokenData = {
          avatar: user?.headerImg || "",
          username: user?.username || data.username,
          nickname: user?.nickName || data.username,
          roles,
          permissions,
          accessToken,
          refreshToken: rToken || accessToken,
          expires: new Date(Date.now() + 7 * 24 * 60 * 60 * 1000) // 7天
        };

        setToken(tokenData);

        // 登录成功后上报插件注册表（此时已有 token）
        triggerPluginRegistryReport();

        return {
          success: true,
          data: tokenData
        } as UserResult;
      } catch (error: any) {
        return {
          success: false,
          message:
            error?.response?.data?.message ||
            error?.message ||
            "登录失败，请检查网络连接"
        } as UserResult;
      }
    });

    // 注入真实 Token 刷新策略
    setRefreshHandler(async data => {
      try {
        const res = await refreshToken(data);
        const body = res.data;

        if (body.code !== 0) {
          return { success: false };
        }

        const { accessToken, refreshToken: rToken } = body.data;
        setAuthToken(`Bearer ${accessToken}`);

        const tokenData = {
          accessToken,
          refreshToken: rToken || accessToken,
          expires: new Date(Date.now() + 7 * 24 * 60 * 60 * 1000)
        };

        setToken(tokenData as any);
        return { success: true, data: tokenData };
      } catch {
        return { success: false };
      }
    });

    // 注入用户信息获取策略（页面刷新时同步头像、角色等）
    setUserInfoHandler(async () => {
      try {
        const res = await getUserInfo();
        if (res.data?.code === 0 && res.data?.data) {
          return {
            success: true,
            data: {
              avatar: res.data.data.avatar || "",
              roles: res.data.data.roles || [],
              nickname: res.data.data.nickname || ""
            }
          };
        }
        return { success: false };
      } catch {
        return { success: false };
      }
    });

    console.log("[Plugin] Auth plugin setup complete - using real backend");
  }
};

export default authPlugin;
