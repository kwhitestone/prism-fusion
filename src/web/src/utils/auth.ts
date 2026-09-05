import Cookies from "js-cookie";
import { useUserStoreHook } from "@/store/modules/user";
import { storageLocal, isString, isIncludeAllChildren } from "@pureadmin/utils";
import {
  invalidateAuthSession,
  observeAuthSession,
  withAuthSessionLock
} from "@/core/auth-session";

export interface DataInfo<T> {
  /** token */
  accessToken: string;
  /** `accessToken`的过期时间（时间戳） */
  expires: T;
  /** 用于调用刷新accessToken的接口时所需的token */
  refreshToken: string;
  /** 当前浏览器登录会话标识，用于阻止跨账号请求串线 */
  sessionId?: string;
  /** 当前 refresh token 的稳定轮换请求标识 */
  refreshRequestId?: string;
  /** 头像 */
  avatar?: string;
  /** 用户名 */
  username?: string;
  /** 昵称 */
  nickname?: string;
  /** 当前登录用户的角色 */
  roles?: Array<string>;
  /** 当前登录用户的按钮级别权限 */
  permissions?: Array<string>;
}

export const userKey = "user-info";
export const AuthTokenKey = "auth_token";
/**
 * 通过`multiple-tabs`是否在`cookie`中，判断用户是否已经登录系统，
 * 从而支持多标签页打开已经登录的系统后无需再登录。
 * 浏览器完全关闭后`multiple-tabs`将自动从`cookie`中销毁，
 * 再次打开浏览器需要重新登录系统
 * */
export const multipleTabsKey = "multiple-tabs";

/** 获取 auth_token */
export function getAuthToken(): string | null {
  return localStorage.getItem(AuthTokenKey);
}

/** 设置 auth_token */
export function setAuthToken(token: string | object): void {
  const encodedToken =
    typeof token === "string" ? token : btoa(JSON.stringify(token));
  localStorage.setItem(AuthTokenKey, encodedToken);
}

/** 删除 auth_token */
export function removeAuthToken(): void {
  localStorage.removeItem(AuthTokenKey);
}

function clearTokenStorage(): void {
  removeAuthToken();
  Cookies.remove(multipleTabsKey);
  storageLocal().removeItem(userKey);
}

/** 获取token信息 */
export function getToken(): DataInfo<number> | null {
  const userInfo = storageLocal().getItem<DataInfo<number>>(userKey);
  if (!userInfo) return null;
  return userInfo;
}

/**
 * @description 设置`token`以及一些必要信息
 * 将用户信息放在key值为`user-info`的localStorage里
 */
export function setToken(data: DataInfo<Date>) {
  const { isRemembered, loginDay } = useUserStoreHook();

  Cookies.set(
    multipleTabsKey,
    "true",
    isRemembered
      ? {
          expires: loginDay
        }
      : {}
  );

  function setUserKey({
    avatar,
    username,
    nickname,
    roles,
    permissions,
    refreshToken,
    sessionId,
    refreshRequestId,
    expires
  }) {
    useUserStoreHook().SET_AVATAR(avatar);
    useUserStoreHook().SET_USERNAME(username);
    useUserStoreHook().SET_NICKNAME(nickname);
    useUserStoreHook().SET_ROLES(roles);
    useUserStoreHook().SET_PERMS(permissions);
    storageLocal().setItem(userKey, {
      refreshToken,
      sessionId,
      refreshRequestId,
      expires,
      avatar,
      username,
      nickname,
      roles,
      permissions
    });
  }

  const expires = new Date(data.expires).getTime();

  if (data.username && data.roles) {
    const { username, roles, refreshToken, sessionId, refreshRequestId } = data;
    setUserKey({
      avatar: data?.avatar ?? "",
      username,
      nickname: data?.nickname ?? "",
      roles,
      permissions: data?.permissions ?? [],
      refreshToken,
      sessionId,
      refreshRequestId,
      expires
    });
  } else {
    const avatar =
      storageLocal().getItem<DataInfo<number>>(userKey)?.avatar ?? "";
    const username =
      storageLocal().getItem<DataInfo<number>>(userKey)?.username ?? "";
    const nickname =
      storageLocal().getItem<DataInfo<number>>(userKey)?.nickname ?? "";
    const roles =
      storageLocal().getItem<DataInfo<number>>(userKey)?.roles ?? [];
    const permissions =
      storageLocal().getItem<DataInfo<number>>(userKey)?.permissions ?? [];
    setUserKey({
      avatar,
      username,
      nickname,
      roles,
      permissions,
      refreshToken: data.refreshToken,
      sessionId: data.sessionId,
      refreshRequestId: data.refreshRequestId,
      expires
    });
  }
}

/** 删除`token`以及key值为`user-info`的localStorage信息 */
export function removeToken(onLockedClear?: () => void): Promise<void> {
  invalidateAuthSession();

  // 与 login/refresh 使用同一把锁，确保凭据不会被分段覆盖。
  const clear = async () => {
    invalidateAuthSession();
    observeAuthSession(undefined);
    clearTokenStorage();
    onLockedClear?.();
  };
  if (typeof navigator === "undefined" || !navigator.locks) {
    observeAuthSession(undefined);
    clearTokenStorage();
    onLockedClear?.();
    return Promise.resolve();
  }
  return withAuthSessionLock(clear);
}

/** Clear credentials and visible identity only if no newer session exists. */
export async function endAuthSessionIfCurrent(
  expectedSessionId?: string
): Promise<boolean> {
  const end = () => {
    const current = getToken();
    if (current && current.sessionId !== expectedSessionId) return false;
    invalidateAuthSession();
    observeAuthSession(undefined);
    clearTokenStorage();
    useUserStoreHook().endSession();
    return true;
  };
  if (typeof navigator === "undefined" || !navigator.locks) {
    // Built-in auth requires Web Locks, but preserve fail-closed cleanup for
    // a legacy stored session when the current identity can be checked exactly.
    return end();
  }
  return withAuthSessionLock(async () => end());
}

/**
 * Clear credentials only when they still identify the refresh operation that
 * requested invalidation. The caller must hold the shared auth-session lock
 * whenever a sessionId is present.
 */
export function clearAuthSessionIfMatches(expected: {
  refreshToken: string;
  sessionId?: string;
}): boolean {
  const current = getToken();
  if (
    !current ||
    current.refreshToken !== expected.refreshToken ||
    (expected.sessionId !== undefined &&
      current.sessionId !== expected.sessionId)
  ) {
    return false;
  }
  invalidateAuthSession();
  observeAuthSession(undefined);
  clearTokenStorage();
  useUserStoreHook().endSession();
  return true;
}

/** 是否有按钮级别的权限（根据登录接口返回的`permissions`字段进行判断）*/
export const hasPerms = (value: string | Array<string>): boolean => {
  if (!value) return false;
  const allPerms = "*:*:*";
  const { permissions } = useUserStoreHook();
  if (!permissions) return false;
  if (permissions.length === 1 && permissions[0] === allPerms) return true;
  const isAuths = isString(value)
    ? permissions.includes(value)
    : isIncludeAllChildren(value, permissions);
  return isAuths ? true : false;
};
