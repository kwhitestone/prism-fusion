import axios from "axios";
import type {
  AxiosInstance,
  InternalAxiosRequestConfig,
  AxiosResponse,
  AxiosError
} from "axios";
import { ElLoading } from "element-plus";
import type { LoadingInstance } from "element-plus/es/components/loading/src/loading";
import { useUserStoreHook } from "@/store/modules/user";
import {
  clearAuthSessionIfMatches,
  endAuthSessionIfCurrent,
  getAuthToken,
  getToken
} from "@/utils/auth";
import router from "@/router/index";
import type { BaseResponse } from "@/types";
import {
  applyRequestAuthBinding,
  canRecoverAuthFailure,
  createRefreshCoordinator,
  getObservedAuthSession,
  readAuthBindingWithLock,
  withAuthSessionLock
} from "@/core/auth-session";

// 扩展 AxiosRequestConfig 接口
declare module "axios" {
  interface AxiosRequestConfig {
    donNotShowLoading?: boolean;
    authRetry?: boolean;
    authSessionId?: string;
    loadingOption?: {
      target?: HTMLElement | null;
      text?: string;
      background?: string;
    };
  }
}

// 创建 axios 实例
const service: AxiosInstance = axios.create({
  timeout: 99999
});

const ACCESS_REFRESH_SKEW_MS = 30 * 1000;

const isAuthRecoveryRequest = (url = ""): boolean =>
  ["/login", "/register", "/refresh-token", "/logout"].some(path =>
    url.endsWith(path)
  );

export const refreshAccessToken = createRefreshCoordinator({
  readSession: getToken,
  hasAuthToken: () => Boolean(getAuthToken()),
  withExclusiveLock: withAuthSessionLock,
  invalidate: clearAuthSessionIfMatches,
  rotate: async current => {
    const result = await useUserStoreHook().handRefreshToken({
      refreshToken: current.refreshToken,
      requestId: current.refreshRequestId,
      sessionId: current.sessionId
    });
    if (result.sessionChanged) return "changed";
    return result.success && result.data ? "ready" : "failed";
  }
});

const endInvalidSession = (expectedSessionId?: string): Promise<boolean> =>
  endAuthSessionIfCurrent(expectedSessionId);

const bindRequestAuthentication = async (
  config: InternalAxiosRequestConfig
): Promise<void> => {
  const binding = await readAuthBindingWithLock({
    withExclusiveLock: withAuthSessionLock,
    readSessionId: () => getToken()?.sessionId,
    readObservedSessionId: () => getObservedAuthSession().sessionId,
    readAuthToken: getAuthToken
  });
  if (binding.action === "reload" && typeof window !== "undefined") {
    window.location.reload();
  } else if (binding.action === "end") {
    // The shared lock is still held by readAuthBindingWithLock only while it
    // reads; credentials are already absent here, so defer conditional UI and
    // storage cleanup to the caller's normal invalid-session path.
    await endInvalidSession(getObservedAuthSession().sessionId);
  } else if (binding.action === "none") {
    config.authSessionId = binding.sessionId;
    if (binding.authToken) {
      config.headers.set("Authorization", binding.authToken);
    } else {
      config.headers.delete("Authorization");
    }
    return;
  }
  throw new Error("登录会话已在其他标签页变更");
};

// Loading 状态管理
let activeAxios = 0;
let timer: ReturnType<typeof setTimeout> | null = null;
let loadingInstance: LoadingInstance | null = null;
let isLoadingVisible = false;
let forceCloseTimer: ReturnType<typeof setTimeout> | null = null;

// Loading 配置选项接口
interface LoadingOption {
  target?: HTMLElement | null;
  text?: string;
  background?: string;
}

// 显示 Loading
const showLoading = (option: LoadingOption = {}): void => {
  const loadDom = document.getElementById("prism-base-load-dom");
  activeAxios++;

  if (timer) {
    clearTimeout(timer);
  }

  if (forceCloseTimer) {
    clearTimeout(forceCloseTimer);
  }

  timer = setTimeout(() => {
    if (activeAxios > 0 && !isLoadingVisible) {
      const target = option.target || loadDom || document.body;
      loadingInstance = ElLoading.service({
        target,
        text: option.text,
        background: option.background
      });
      isLoadingVisible = true;

      // 设置强制关闭定时器，防止loading永远不关闭（30秒超时）
      forceCloseTimer = setTimeout(() => {
        if (isLoadingVisible && loadingInstance) {
          console.warn("Loading强制关闭：超时30秒");
          loadingInstance.close();
          isLoadingVisible = false;
          activeAxios = 0;
        }
      }, 30000);
    }
  }, 400);
};

// 关闭 Loading
const closeLoading = (): void => {
  activeAxios--;
  if (activeAxios <= 0) {
    activeAxios = 0;

    if (timer) {
      clearTimeout(timer);
      timer = null;
    }

    if (forceCloseTimer) {
      clearTimeout(forceCloseTimer);
      forceCloseTimer = null;
    }

    if (isLoadingVisible && loadingInstance) {
      loadingInstance.close();
      isLoadingVisible = false;
    }
    loadingInstance = null;
  }
};

// 重置 Loading 状态
const resetLoading = (): void => {
  activeAxios = 0;
  isLoadingVisible = false;

  if (timer) {
    clearTimeout(timer);
    timer = null;
  }

  if (forceCloseTimer) {
    clearTimeout(forceCloseTimer);
    forceCloseTimer = null;
  }

  if (loadingInstance) {
    try {
      loadingInstance.close();
    } catch (e) {
      console.warn("关闭loading时出错:", e);
    }
    loadingInstance = null;
  }
};

// 获取错误信息
const getErrorMessage = (error: AxiosError): string => {
  const responseData = error.response?.data as
    | { msg?: string; detail?: string; message?: string }
    | undefined;
  return (
    responseData?.detail ||
    responseData?.msg ||
    responseData?.message ||
    error.response?.statusText ||
    error.message ||
    "请求失败"
  );
};

// HTTP请求拦截器
service.interceptors.request.use(
  async (
    config: InternalAxiosRequestConfig
  ): Promise<InternalAxiosRequestConfig> => {
    if (!config.donNotShowLoading) {
      showLoading(config.loadingOption);
    }

    // 不设置baseURL，让API调用使用完整路径
    // config.baseURL = config.baseURL || import.meta.env.VITE_BASE_API
    const userStore = useUserStoreHook();

    const recoveryRequest = isAuthRecoveryRequest(config.url);
    if (!recoveryRequest) {
      const token = getToken();
      config.authSessionId = token?.sessionId;
      if (token && token.expires - Date.now() <= ACCESS_REFRESH_SKEW_MS) {
        const refreshOutcome = await refreshAccessToken(config.authSessionId);
        if (refreshOutcome !== "ready") {
          if (refreshOutcome === "failed") {
            await endInvalidSession(config.authSessionId);
          }
          throw new Error("认证会话已过期，请重新登录");
        }
      }
    }
    await applyRequestAuthBinding({
      isRecoveryRequest: recoveryRequest,
      clearAuthorization: () => config.headers.delete("Authorization"),
      bind: () => bindRequestAuthentication(config)
    });

    config.headers.set("Content-Type", "application/json");
    // 防御性编码：username 可能含非 ASCII 字符（如中文显示名），
    // HTTP 头要求 ISO-8859-1，直接放中文会导致浏览器抛异常
    config.headers.set(
      "x-user-id",
      encodeURIComponent(String(userStore.username || ""))
    );

    return config;
  },
  (error: AxiosError): Promise<AxiosError> => {
    if (!error.config?.donNotShowLoading) {
      closeLoading();
    }
    console.error("❌ 请求发送失败:", error.message || "请求发送失败");
    return Promise.reject(error);
  }
);

// HTTP响应拦截器
service.interceptors.response.use(
  (response: AxiosResponse<BaseResponse>): AxiosResponse<BaseResponse> => {
    if (!response.config.donNotShowLoading) {
      closeLoading();
    }

    // 处理新token - 注意：新的认证系统不使用这种方式
    // 如果需要token刷新，会通过自动刷新机制处理
    // if (response.headers['new-token']) {
    //   userStore.refreshToken(response.headers['new-token'])
    // }

    // 如果没有业务状态码，包装响应数据
    if (typeof response.data?.code === "undefined") {
      response.data = {
        code: 0,
        data: response.data,
        msg: "操作成功"
      };
    }

    // 处理业务成功响应
    if (response.data.code === 0 || response.headers.success === "true") {
      if (response.headers.msg) {
        response.data.msg = decodeURI(response.headers.msg);
      }
    } else {
      // 处理业务错误
      console.error(
        "❌ 业务错误:",
        response.data.msg || decodeURI(response.headers.msg || "")
      );
    }

    return response;
  },
  async (error: AxiosError): Promise<AxiosResponse> => {
    if (!error.config?.donNotShowLoading) {
      closeLoading();
    }

    if (!error.response) {
      console.error(
        "❌ 网络错误:",
        getErrorMessage(error),
        "\n  url:",
        error.config?.url,
        "\n  method:",
        error.config?.method,
        "\n  code:",
        error.code,
        "\n  message:",
        error.message
      );
      return Promise.reject(error);
    }

    if (error.response.status === 401) {
      console.error("❌ 认证错误:", getErrorMessage(error));
      const original = error.config;
      // Recovery requests own their session transition. In particular, a late
      // refresh 401 must not invalidate credentials established by another tab.
      if (isAuthRecoveryRequest(original?.url)) return Promise.reject(error);

      // An anonymous request may finish after a new account logs in. Never
      // refresh or replay that old request with the replacement identity.
      if (!canRecoverAuthFailure(original?.authSessionId)) {
        return Promise.reject(error);
      }

      if (original && !original.authRetry) {
        original.authRetry = true;
        const refreshOutcome = await refreshAccessToken(original.authSessionId);
        if (refreshOutcome === "ready") {
          return service.request(original);
        }
        if (refreshOutcome === "changed") return Promise.reject(error);
      }
      await endInvalidSession(original.authSessionId);
      return Promise.reject(error);
    }

    if (error.response.status === 403) {
      console.error("❌ 权限不足:", getErrorMessage(error));
      router.push({ name: "403", replace: true });
      return Promise.reject(error);
    }

    console.error("❌ 请求错误:", getErrorMessage(error));
    return Promise.reject(error);
  }
);

// 监听页面卸载事件，确保loading被正确清理
if (typeof window !== "undefined") {
  window.addEventListener("beforeunload", resetLoading);
  window.addEventListener("pagehide", resetLoading);
}

export { resetLoading };
export default service;
