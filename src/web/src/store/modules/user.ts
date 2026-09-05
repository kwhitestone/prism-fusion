import { defineStore } from "pinia";
import {
  type userType,
  store,
  router,
  resetRouter,
  routerArrays,
  storageLocal
} from "../utils";
import type { UserResult } from "@/api/user";
import { createReversibleSlot } from "@/core/reversible-slot";
import { useMultiTagsStoreHook } from "./multiTags";
import { type DataInfo, getToken, removeToken, userKey } from "@/utils/auth";
import {
  commitAuthSessionResult,
  currentAuthSessionEpoch,
  isAuthSessionEpoch,
  withAuthSessionLock
} from "@/core/auth-session";

// 默认头像
const DEFAULT_AVATAR = new URL("@/assets/avatar.svg", import.meta.url).href;

// ========== 策略注入 ==========
// 登录处理策略默认为关闭；认证插件或业务 provider 必须显式注入。
type LoginHandler = (data: {
  username: string;
  password: string;
}) => Promise<UserResult>;

type RefreshHandler = (data: {
  refreshToken: string;
  requestId: string;
  sessionId: string;
}) => Promise<{ success: boolean; data?: any; sessionChanged?: boolean }>;

type UserInfoHandler = () => Promise<{
  success: boolean;
  data?: {
    avatar?: string;
    roles?: string[];
    permissions?: string[];
    nickname?: string;
    username?: string;
  };
}>;

type LogoutHandler = (data: { refreshToken: string }) => Promise<void>;

const defaultLoginHandler: LoginHandler = async _data => {
  return {
    success: false,
    message: "未配置认证服务"
  } as UserResult;
};

const defaultRefreshHandler: RefreshHandler = async _data => {
  return { success: false };
};

const defaultLogoutHandler: LogoutHandler = async _data => {};

const loginHandlers = createReversibleSlot(defaultLoginHandler);
const refreshHandlers = createReversibleSlot(defaultRefreshHandler);
const userInfoHandlers = createReversibleSlot<UserInfoHandler | null>(null);
const logoutHandlers = createReversibleSlot(defaultLogoutHandler);

/** 设置登录处理策略（由 auth 插件调用） */
export function setLoginHandler(handler: LoginHandler): () => void {
  if (typeof handler !== "function")
    throw new TypeError("Login handler must be a function");
  return loginHandlers.set(handler);
}

/** 设置 Token 刷新策略（由 auth 插件调用） */
export function setRefreshHandler(handler: RefreshHandler): () => void {
  if (typeof handler !== "function")
    throw new TypeError("Refresh handler must be a function");
  return refreshHandlers.set(handler);
}

/** 设置用户信息获取策略（由 auth 插件调用，用于页面刷新时同步用户信息） */
export function setUserInfoHandler(handler: UserInfoHandler): () => void {
  if (typeof handler !== "function")
    throw new TypeError("User info handler must be a function");
  return userInfoHandlers.set(handler);
}

/** 设置服务端会话注销策略（由 auth 插件调用） */
export function setLogoutHandler(handler: LogoutHandler): () => void {
  if (typeof handler !== "function")
    throw new TypeError("Logout handler must be a function");
  return logoutHandlers.set(handler);
}

export const useUserStore = defineStore("pure-user", {
  state: (): userType => ({
    avatar:
      storageLocal().getItem<DataInfo<number>>(userKey)?.avatar ??
      DEFAULT_AVATAR,
    username: storageLocal().getItem<DataInfo<number>>(userKey)?.username ?? "",
    nickname: storageLocal().getItem<DataInfo<number>>(userKey)?.nickname ?? "",
    roles: storageLocal().getItem<DataInfo<number>>(userKey)?.roles ?? [],
    permissions:
      storageLocal().getItem<DataInfo<number>>(userKey)?.permissions ?? [],
    isRemembered: false,
    loginDay: 7
  }),
  actions: {
    /** 存储头像 */
    SET_AVATAR(avatar: string) {
      this.avatar = avatar;
    },
    /** 存储用户名 */
    SET_USERNAME(username: string) {
      this.username = username;
    },
    /** 存储昵称 */
    SET_NICKNAME(nickname: string) {
      this.nickname = nickname;
    },
    /** 存储角色 */
    SET_ROLES(roles: Array<string>) {
      this.roles = roles;
    },
    /** 存储按钮级别权限 */
    SET_PERMS(permissions: Array<string>) {
      this.permissions = permissions;
    },
    /** 存储是否勾选了登录页的免登录 */
    SET_ISREMEMBERED(bool: boolean) {
      this.isRemembered = bool;
    },
    /** 设置登录页的免登录存储几天 */
    SET_LOGINDAY(value: number) {
      this.loginDay = Number(value);
    },
    /** 登入 - 通过策略注入实现，auth 插件可覆盖为真实后端调用 */
    async loginByPassword(data: { username: string; password: string }) {
      try {
        const version = loginHandlers.version;
        const res = await loginHandlers.get()(data);
        if (version !== loginHandlers.version)
          return {
            success: false,
            message: "认证服务已变更，请重新登录"
          } as UserResult;

        if (res.success && res.data) {
          // 更新 store 中的用户信息
          this.SET_AVATAR(res.data.avatar || DEFAULT_AVATAR);
          this.SET_USERNAME(res.data.username || data.username);
          this.SET_NICKNAME(res.data.nickname || data.username);
          this.SET_ROLES(
            Array.isArray(res.data.roles) ? [...res.data.roles] : []
          );
          this.SET_PERMS(
            Array.isArray(res.data.permissions) ? [...res.data.permissions] : []
          );
        }

        return res;
      } catch (error) {
        console.error("登录失败:", error);
        return {
          success: false,
          message: error instanceof Error ? error.message : "登录失败"
        } as UserResult;
      }
    },
    /** 前端登出 */
    logOut() {
      const refreshToken = getToken()?.refreshToken;
      const version = logoutHandlers.version;
      const handler = logoutHandlers.get();
      this.endSession();
      const finalized = removeToken();
      if (refreshToken) {
        void finalized
          .then(() =>
            version === logoutHandlers.version
              ? handler({ refreshToken })
              : undefined
          )
          .catch(error => console.warn("服务端会话注销失败:", error));
      }
    },
    /** 结束当前 UI 会话；凭据由持有认证锁的调用方清理。 */
    endSession() {
      this.username = "";
      this.roles = [];
      this.permissions = [];
      useMultiTagsStoreHook().handleTags("equal", [...routerArrays]);
      resetRouter();
      router.push("/login");
    },
    /** 刷新Token - 通过策略注入实现 */
    async handRefreshToken(data: Parameters<RefreshHandler>[0]) {
      const version = refreshHandlers.version;
      const result = await refreshHandlers.get()(data);
      return version === refreshHandlers.version
        ? result
        : { success: false, sessionChanged: true };
    },
    /** 从后端刷新用户信息（头像、角色等），更新 store 和 localStorage */
    async fetchUserInfo() {
      const handler = userInfoHandlers.get();
      if (!handler) return;
      const providerVersion = userInfoHandlers.version;
      const expectedSession = getToken();
      if (!expectedSession?.sessionId) return;
      const expectedSessionId = expectedSession.sessionId;
      const expectedEpoch = currentAuthSessionEpoch();
      try {
        const res = await handler();
        if (providerVersion !== userInfoHandlers.version) return;
        if (res.success && res.data) {
          await commitAuthSessionResult({
            expectedSessionId,
            expectedEpoch,
            withExclusiveLock: withAuthSessionLock,
            readSessionId: () => getToken()?.sessionId,
            isEpochCurrent: isAuthSessionEpoch,
            commit: () => {
              if (providerVersion !== userInfoHandlers.version) return;
              const stored = storageLocal().getItem<DataInfo<number>>(userKey);
              if (!stored) return;
              const roles =
                res.data?.roles !== undefined ? [...res.data.roles] : undefined;
              const permissions =
                res.data?.permissions !== undefined
                  ? [...res.data.permissions]
                  : undefined;
              const nextStored: DataInfo<number> = {
                ...stored,
                ...(res.data?.avatar ? { avatar: res.data.avatar } : {}),
                ...(res.data?.nickname ? { nickname: res.data.nickname } : {}),
                ...(res.data?.username ? { username: res.data.username } : {}),
                ...(roles !== undefined ? { roles } : {}),
                ...(permissions !== undefined ? { permissions } : {})
              };
              storageLocal().setItem(userKey, nextStored);
              if (res.data?.avatar) this.SET_AVATAR(res.data.avatar);
              if (res.data?.nickname) this.SET_NICKNAME(res.data.nickname);
              if (res.data?.username) this.SET_USERNAME(res.data.username);
              if (roles !== undefined) this.SET_ROLES(roles);
              if (permissions !== undefined) this.SET_PERMS(permissions);
            }
          });
        }
      } catch (e) {
        console.warn("fetchUserInfo failed:", e);
      }
    }
  }
});

export function useUserStoreHook() {
  return useUserStore(store);
}
