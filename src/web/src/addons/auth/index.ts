import type { PluginModule } from "@/plugin/types";
import { triggerPluginRegistryReport } from "@/plugin/loader";
import {
  setLoginHandler,
  setLogoutHandler,
  setRefreshHandler,
  setUserInfoHandler,
  useUserStoreHook
} from "@/store/modules/user";
import { setLoginComponent } from "@/store/modules/loginUI";
import {
  endAuthSessionIfCurrent,
  getToken,
  setToken,
  setAuthToken,
  userKey
} from "@/utils/auth";
import { getUserInfo, login, logout, refreshToken } from "./api";
import type { UserResult } from "@/api/user";
import { getConfig } from "@/config";
import LoginForm from "./components/LoginForm.vue";
import {
  accessTokenExpiry,
  createAuthRequestID,
  currentAuthSessionEpoch,
  crossTabSessionAction,
  getObservedAuthSession,
  invalidateAuthSession,
  isAuthSessionEpoch,
  observeAuthSession,
  withAuthSessionLock
} from "./session";

let storageListenerInstalled = false;
let disposeSetup: (() => void) | undefined;

function installCrossTabSessionListener(): () => void {
  if (storageListenerInstalled || typeof window === "undefined")
    return () => {};
  storageListenerInstalled = true;
  const listener = (event: StorageEvent) => {
    if (event.storageArea !== localStorage || event.key !== userKey) return;
    const observed = getObservedAuthSession();
    const nextSessionId = getToken()?.sessionId;
    const action = crossTabSessionAction(observed.sessionId, nextSessionId);
    if (action === "reload") {
      // Do not adopt the new ID before reload: request guards must continue to
      // reject traffic while the visible roles/routes still belong to S1.
      window.location.reload();
      return;
    }
    if (action === "end") {
      observeAuthSession(undefined);
      useUserStoreHook().endSession();
    }
  };
  window.addEventListener("storage", listener);
  return () => {
    window.removeEventListener("storage", listener);
    storageListenerInstalled = false;
  };
}

const authPlugin: PluginModule = {
  name: "auth",
  description: "认证插件 - 提供用户名密码登录、Token 刷新",
  version: "1.0.0",
  manifest: {
    apiVersion: "prism-fusion/v2",
    kind: "frontend-addon",
    id: "auth",
    version: "1.0.0"
  },
  priority: 10,
  enabled: () => (getConfig()?.AuthProvider || "builtin") === "builtin",

  setup() {
    let active = true;
    let disposers: (() => void)[] = [];
    const own = (dispose: () => void) => {
      disposers = [...disposers, dispose];
    };
    const unavailable = () =>
      ({ success: false, message: "认证插件已停止" }) as UserResult;
    disposeSetup = () => {
      active = false;
      for (const dispose of [...disposers].reverse()) {
        try {
          dispose();
        } catch (error) {
          console.error("[Auth Plugin] Cleanup failed", error);
        }
      }
      disposers = [];
    };
    const initialSession = getToken();
    if (
      initialSession &&
      (!initialSession.sessionId || !initialSession.refreshRequestId)
    ) {
      // Pre-V2 browser sessions have no identity with which requests can be
      // bound safely. Mark them stale immediately and clear under the lock.
      observeAuthSession("invalid-legacy-session");
      void endAuthSessionIfCurrent(undefined);
    } else {
      observeAuthSession(initialSession?.sessionId);
    }
    own(installCrossTabSessionListener());

    // 注册登录界面组件：用户名 + 密码表单
    own(setLoginComponent(LoginForm));

    // 注入真实登录策略；未注册 provider 时壳保持认证关闭。
    own(
      setLoginHandler(async data => {
        if (!active) return unavailable();
        try {
          const res = await login(data);
          if (!active) return unavailable();
          const body = res.data;

          if (body.code !== 0) {
            return {
              success: false,
              message: body.message || "登录失败"
            } as UserResult;
          }

          const {
            accessToken,
            refreshToken: rToken,
            expiresIn,
            user
          } = body.data;

          // 登录响应已携带后端解析的角色和权限；不再在登录中发起可能
          // 被旧会话刷新拦截的二次身份请求。
          const roles: string[] = Array.isArray(user?.roles)
            ? [...user.roles]
            : [];
          const permissions: string[] = Array.isArray(user?.permissions)
            ? [...user.permissions]
            : [];

          // 设置 token 数据到 localStorage
          const tokenData = {
            avatar: user?.headerImg || "",
            username: user?.username || data.username,
            nickname: user?.nickName || data.username,
            roles,
            permissions,
            accessToken,
            refreshToken: rToken || "",
            sessionId: createAuthRequestID(),
            refreshRequestId: createAuthRequestID(),
            expires: accessTokenExpiry(expiresIn)
          };

          const committed = await withAuthSessionLock(async () => {
            if (!active) return false;
            // 登录提交与 refresh/logout 共用跨标签页互斥锁，最后获得锁的
            // 用户操作决定最终会话，旧刷新不能覆盖新账号。
            invalidateAuthSession();
            setAuthToken(`Bearer ${accessToken}`);
            setToken(tokenData);
            observeAuthSession(tokenData.sessionId);
            return true;
          });
          if (!committed || !active) return unavailable();

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
      })
    );

    // 注入真实 Token 刷新策略
    own(
      setRefreshHandler(async data => {
        const epoch = currentAuthSessionEpoch();
        const sessionChanged = () => {
          const current = getToken();
          return (
            !active ||
            !isAuthSessionEpoch(epoch) ||
            current?.sessionId !== data.sessionId ||
            current?.refreshToken !== data.refreshToken
          );
        };
        if (!active) return { success: false, sessionChanged: true };
        try {
          const res = await refreshToken({
            refreshToken: data.refreshToken,
            requestId: data.requestId
          });
          const body = res.data;

          if (body.code !== 0) {
            return { success: false, sessionChanged: sessionChanged() };
          }

          if (sessionChanged()) {
            return { success: false, sessionChanged: true };
          }

          const { accessToken, refreshToken: rToken, expiresIn } = body.data;
          setAuthToken(`Bearer ${accessToken}`);

          const tokenData = {
            accessToken,
            refreshToken: rToken || "",
            sessionId: data.sessionId,
            refreshRequestId: createAuthRequestID(),
            expires: accessTokenExpiry(expiresIn)
          };

          setToken(tokenData as any);
          observeAuthSession(data.sessionId);
          return { success: true, data: tokenData };
        } catch {
          return { success: false, sessionChanged: sessionChanged() };
        }
      })
    );

    own(
      setLogoutHandler(async data => {
        if (!active) return;
        await logout(data);
      })
    );

    // 注入用户信息获取策略（页面刷新时同步头像、角色等）
    own(
      setUserInfoHandler(async () => {
        if (!active) return { success: false };
        try {
          const res = await getUserInfo();
          if (!active) return { success: false };
          if (res.data?.code === 0 && res.data?.data) {
            return {
              success: true,
              data: {
                avatar: res.data.data.headerImg || "",
                roles: res.data.data.roles || [],
                permissions: res.data.data.permissions || [],
                nickname: res.data.data.nickName || "",
                username: res.data.data.username || ""
              }
            };
          }
          return { success: false };
        } catch {
          return { success: false };
        }
      })
    );

    console.log("[Plugin] Auth plugin setup complete - using real backend");
  },

  destroy() {
    const dispose = disposeSetup;
    disposeSetup = undefined;
    dispose?.();
  }
};

export default authPlugin;
