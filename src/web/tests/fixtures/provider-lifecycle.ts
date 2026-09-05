// Test-only HTTP/storage boundaries; the runner loads the real addons and stores.
import { createPinia } from "pinia";

export const store = createPinia();
export const router = {
  push: (path: string) => {
    navigations = [...navigations, { path, sessionId: token?.sessionId }];
  }
};
export const routerArrays = [];
export const resetRouter = () => undefined;
export const useMultiTagsStoreHook = () => ({ handleTags: () => undefined });
const stored = new Map();
export const storageLocal = () => ({
  getItem: (key: string) => stored.get(key),
  setItem: (key: string, value: unknown) => stored.set(key, value)
});

let token: any;
let navigations: { path: string; sessionId: string | undefined }[] = [];
let tokenWrites = 0;
let headerWrites = 0;
let epoch = 0;
let observed: string | undefined;
let loginResponse: () => Promise<any>;
let refreshResponse: () => Promise<any>;
let routeResponse: () => Promise<any>;
let sessionLock: <T>(operation: () => Promise<T>) => Promise<T>;
const listeners = new Set();

export function resetFixture() {
  token = {
    sessionId: "session",
    refreshToken: "refresh",
    refreshRequestId: "request"
  };
  tokenWrites = 0;
  navigations = [];
  headerWrites = 0;
  observed = token.sessionId;
  epoch = 0;
  stored.clear();
  listeners.clear();
  loginResponse = async () => successResponse();
  refreshResponse = async () => successResponse();
  routeResponse = async () => ({
    data: { success: true, data: [{ path: "/rbac" }] }
  });
  sessionLock = operation => operation();
  Object.defineProperty(globalThis, "window", {
    configurable: true,
    value: {
      addEventListener: (_type: string, callback: unknown) =>
        listeners.add(callback),
      removeEventListener: (_type: string, callback: unknown) =>
        listeners.delete(callback),
      location: { reload: () => undefined }
    }
  });
}

export function successResponse() {
  return {
    data: {
      code: 0,
      data: {
        accessToken: "access",
        refreshToken: "next-refresh",
        expiresIn: 3600,
        user: {
          username: "alice",
          roles: ["user"],
          permissions: ["example:item:view"]
        }
      }
    }
  };
}
export const fixtureState = () => ({
  token,
  navigations: [...navigations],
  tokenWrites,
  headerWrites,
  listeners: listeners.size
});
export const configureLogin = (callback: () => Promise<any>) => {
  loginResponse = callback;
};
export const configureRefresh = (callback: () => Promise<any>) => {
  refreshResponse = callback;
};
export const configureRoutes = (callback: () => Promise<any>) => {
  routeResponse = callback;
};
export const configureLock = (callback: typeof sessionLock) => {
  sessionLock = callback;
};
export const login = () => loginResponse();
export const refreshToken = () => refreshResponse();
export const getAsyncRoutes = () => routeResponse();
export const logout = async () => undefined;
export const getUserInfo = async () => ({
  data: {
    code: 0,
    data: { username: "alice", roles: ["user"], permissions: [] }
  }
});
export const getConfig = () => ({
  AuthProvider: "builtin",
  RBACProvider: "builtin"
});
export const triggerPluginRegistryReport = () => undefined;
export const userKey = "user-info";
export const getToken = () => token;
export const setToken = (value: unknown) => {
  token = value;
  tokenWrites++;
};
export const setAuthToken = () => {
  headerWrites++;
};
export const removeToken = (onLockedClear?: () => void) =>
  sessionLock(async () => {
    token = undefined;
    onLockedClear?.();
  });
export const endAuthSessionIfCurrent = async () => undefined;
export const createAuthRequestID = () => "new-id";
export const accessTokenExpiry = () => Date.now() + 3600000;
export const currentAuthSessionEpoch = () => epoch;
export const invalidateAuthSession = () => {
  epoch++;
};
export const isAuthSessionEpoch = (value: number) => value === epoch;
export const getObservedAuthSession = () => ({ sessionId: observed });
export const observeAuthSession = (value: string | undefined) => {
  observed = value;
};
export const crossTabSessionAction = () => "none";
export const withAuthSessionLock = <T>(operation: () => Promise<T>) =>
  sessionLock(operation);
export const commitAuthSessionResult = async () => undefined;
export default { name: "LoginForm", render: () => null };
