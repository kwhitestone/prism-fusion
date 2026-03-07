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
import { useMultiTagsStoreHook } from "./multiTags";
import {
  type DataInfo,
  setToken,
  removeToken,
  userKey,
  setAuthToken,
  removeAuthToken
} from "@/utils/auth";

// 默认头像
const DEFAULT_AVATAR = new URL("@/assets/avatar.svg", import.meta.url).href;

// 生成随机token（默认 mock 实现）
function generateRandomToken(): string {
  return Math.random().toString(36).substring(2) + Date.now().toString(36);
}

// ========== 策略注入 ==========
// 登录处理策略（默认为 mock，auth 插件会覆盖为真实后端调用）
type LoginHandler = (data: {
  username: string;
  password: string;
}) => Promise<UserResult>;

type RefreshHandler = (data: {
  refreshToken: string;
}) => Promise<{ success: boolean; data?: any }>;

type UserInfoHandler = () => Promise<{
  success: boolean;
  data?: {
    avatar?: string;
    roles?: string[];
    nickname?: string;
    username?: string;
  };
}>;

// 默认 mock 登录
const defaultLoginHandler: LoginHandler = async data => {
  const randomToken = generateRandomToken();
  setAuthToken(randomToken);
  const tokenData = {
    avatar: DEFAULT_AVATAR,
    username: data.username,
    nickname: data.username,
    roles: ["admin"],
    permissions: ["*:*:*"],
    accessToken: randomToken,
    refreshToken: randomToken,
    expires: new Date(Date.now() + 7 * 24 * 60 * 60 * 1000)
  };
  setToken(tokenData);
  return { success: true, data: tokenData } as UserResult;
};

// 默认 mock 刷新
const defaultRefreshHandler: RefreshHandler = async _data => {
  const randomToken = generateRandomToken();
  setAuthToken(randomToken);
  const tokenData = {
    accessToken: randomToken,
    refreshToken: randomToken,
    expires: new Date(Date.now() + 7 * 24 * 60 * 60 * 1000)
  };
  setToken(tokenData as any);
  return { success: true, data: tokenData };
};

let _loginHandler: LoginHandler = defaultLoginHandler;
let _refreshHandler: RefreshHandler = defaultRefreshHandler;
let _userInfoHandler: UserInfoHandler | null = null;

/** 设置登录处理策略（由 auth 插件调用） */
export function setLoginHandler(handler: LoginHandler) {
  _loginHandler = handler;
}

/** 设置 Token 刷新策略（由 auth 插件调用） */
export function setRefreshHandler(handler: RefreshHandler) {
  _refreshHandler = handler;
}

/** 设置用户信息获取策略（由 auth 插件调用，用于页面刷新时同步用户信息） */
export function setUserInfoHandler(handler: UserInfoHandler) {
  _userInfoHandler = handler;
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
        const res = await _loginHandler(data);

        if (res.success && res.data) {
          // 更新 store 中的用户信息
          this.SET_AVATAR(res.data.avatar || DEFAULT_AVATAR);
          this.SET_USERNAME(res.data.username || data.username);
          this.SET_NICKNAME(res.data.nickname || data.username);
          this.SET_ROLES(res.data.roles || ["admin"]);
          this.SET_PERMS(res.data.permissions || ["*:*:*"]);
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
      this.username = "";
      this.roles = [];
      this.permissions = [];
      removeToken();
      removeAuthToken();
      useMultiTagsStoreHook().handleTags("equal", [...routerArrays]);
      resetRouter();
      router.push("/login");
    },
    /** 刷新Token - 通过策略注入实现 */
    async handRefreshToken(data: { refreshToken: string }) {
      return _refreshHandler(data);
    },
    /** 从后端刷新用户信息（头像、角色等），更新 store 和 localStorage */
    async fetchUserInfo() {
      if (!_userInfoHandler) return;
      try {
        const res = await _userInfoHandler();
        if (res.success && res.data) {
          if (res.data.avatar) this.SET_AVATAR(res.data.avatar);
          if (res.data.nickname) this.SET_NICKNAME(res.data.nickname);
          if (res.data.username) this.SET_USERNAME(res.data.username);
          if (res.data.roles?.length) this.SET_ROLES(res.data.roles);
          // 同步到 localStorage
          const stored =
            storageLocal().getItem<DataInfo<number>>(userKey) || ({} as any);
          if (res.data.avatar) stored.avatar = res.data.avatar;
          if (res.data.nickname) stored.nickname = res.data.nickname;
          if (res.data.username) stored.username = res.data.username;
          if (res.data.roles?.length) stored.roles = res.data.roles;
          storageLocal().setItem(userKey, stored);
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
