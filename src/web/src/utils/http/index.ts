import Axios, {
  type AxiosInstance,
  type AxiosRequestConfig,
  type CustomParamsSerializer
} from "axios";
import type {
  PureHttpError,
  RequestMethods,
  PureHttpResponse,
  PureHttpRequestConfig
} from "./types.d";
import { stringify } from "qs";
import { endAuthSessionIfCurrent, getToken, getAuthToken } from "@/utils/auth";
import router from "@/router/index";
import { refreshAccessToken } from "@/utils/request";
import {
  canRecoverAuthFailure,
  getObservedAuthSession,
  readAuthBindingWithLock,
  withAuthSessionLock
} from "@/addons/auth/session";

// 相关配置请参考：www.axios-js.com/zh-cn/docs/#axios-request-config-1
const defaultConfig: AxiosRequestConfig = {
  // 请求超时时间
  timeout: 10000,
  headers: {
    Accept: "application/json, text/plain, */*",
    "Content-Type": "application/json",
    "X-Requested-With": "XMLHttpRequest"
  },
  // 数组格式参数序列化（https://github.com/axios/axios/issues/5142）
  paramsSerializer: {
    serialize: stringify as unknown as CustomParamsSerializer
  }
};

const isAuthRecoveryRequest = (url = ""): boolean =>
  ["/refresh-token", "/login", "/register", "/logout"].some(path =>
    url.endsWith(path)
  );

const endSessionIfCurrent = (expectedSessionId?: string): Promise<boolean> =>
  endAuthSessionIfCurrent(expectedSessionId);

const bindRequestAuthentication = async (
  config: PureHttpRequestConfig
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
    await endSessionIfCurrent(getObservedAuthSession().sessionId);
  } else if (binding.action === "none") {
    config.authSessionId = binding.sessionId;
    if (binding.authToken) {
      config.headers!["Authorization"] = binding.authToken;
    } else if (config.headers) {
      delete config.headers["Authorization"];
    }
    return;
  }
  throw new Error("登录会话已在其他标签页变更");
};

class PureHttp {
  constructor() {
    this.httpInterceptorsRequest();
    this.httpInterceptorsResponse();
  }

  /** 初始化配置对象 */
  private static initConfig: PureHttpRequestConfig = {};

  /** 保存当前`Axios`实例对象 */
  private static axiosInstance: AxiosInstance = Axios.create(defaultConfig);

  /** 请求拦截 */
  private httpInterceptorsRequest(): void {
    PureHttp.axiosInstance.interceptors.request.use(
      async (config: PureHttpRequestConfig): Promise<any> => {
        // 优先判断post/get等方法是否传入回调，否则执行初始化设置等回调
        if (typeof config.beforeRequestCallback === "function") {
          config.beforeRequestCallback(config);
          return config;
        }
        if (PureHttp.initConfig.beforeRequestCallback) {
          PureHttp.initConfig.beforeRequestCallback(config);
          return config;
        }
        /** 请求白名单，放置一些不需要`token`的接口（通过设置请求白名单，防止`token`过期后再请求造成的死循环问题） */
        if (isAuthRecoveryRequest(config.url)) {
          if (config.headers) delete config.headers["Authorization"];
          return config;
        }
        const data = getToken();
        config.authSessionId = data?.sessionId;
        if (data && data.expires - Date.now() <= 0) {
          const outcome = await refreshAccessToken(config.authSessionId);
          if (outcome !== "ready") {
            if (outcome === "failed") {
              await endSessionIfCurrent(config.authSessionId);
            }
            throw new Error("认证会话已过期，请重新登录");
          }
        }
        await bindRequestAuthentication(config);
        return config;
      },
      error => {
        return Promise.reject(error);
      }
    );
  }

  /** 响应拦截 */
  private httpInterceptorsResponse(): void {
    const instance = PureHttp.axiosInstance;
    instance.interceptors.response.use(
      (response: PureHttpResponse) => {
        const $config = response.config;
        // 优先判断post/get等方法是否传入回调，否则执行初始化设置等回调
        if (typeof $config.beforeResponseCallback === "function") {
          $config.beforeResponseCallback(response);
          return response.data;
        }
        if (PureHttp.initConfig.beforeResponseCallback) {
          PureHttp.initConfig.beforeResponseCallback(response);
          return response.data;
        }
        return response.data;
      },
      async (error: PureHttpError) => {
        const $error = error;
        $error.isCancelRequest = Axios.isCancel($error);

        // 取消请求不做处理
        if ($error.isCancelRequest) {
          return Promise.reject($error);
        }

        const status = $error.response?.status;

        // 401 认证失败 → 单飞刷新并重试一次
        if (status === 401) {
          const config = $error.config as PureHttpRequestConfig | undefined;
          if (isAuthRecoveryRequest(config?.url)) {
            return Promise.reject($error);
          }
          if (!canRecoverAuthFailure(config?.authSessionId)) {
            return Promise.reject($error);
          }
          if (config && !config.authRetry) {
            config.authRetry = true;
            const outcome = await refreshAccessToken(config.authSessionId);
            if (outcome === "ready") {
              return instance.request(config);
            }
            if (outcome === "changed") return Promise.reject($error);
          }
          await endSessionIfCurrent(config.authSessionId);
          return Promise.reject($error);
        }

        // 403 权限不足 → 跳转403页面
        if (status === 403) {
          router.push({ name: "403", replace: true });
          return Promise.reject($error);
        }

        // 其他 HTTP 错误 → 归一化为 {code, message} 格式，让调用方统一处理
        const data = $error.response?.data as any;
        const message =
          data?.detail ||
          data?.message ||
          data?.msg ||
          $error.message ||
          "请求失败";
        return { code: status || 500, message };
      }
    );
  }

  /** 通用请求工具函数 */
  public request<T>(
    method: RequestMethods,
    url: string,
    param?: AxiosRequestConfig,
    axiosConfig?: PureHttpRequestConfig
  ): Promise<T> {
    const config = {
      method,
      url,
      ...param,
      ...axiosConfig
    } as PureHttpRequestConfig;

    // 单独处理自定义请求/响应回调
    return new Promise((resolve, reject) => {
      PureHttp.axiosInstance
        .request(config)
        .then((response: undefined) => {
          resolve(response);
        })
        .catch(error => {
          reject(error);
        });
    });
  }

  /** 单独抽离的`post`工具函数 */
  public post<T, P>(
    url: string,
    params?: AxiosRequestConfig<P>,
    config?: PureHttpRequestConfig
  ): Promise<T> {
    return this.request<T>("post", url, params, config);
  }

  /** 单独抽离的`get`工具函数 */
  public get<T, P>(
    url: string,
    params?: AxiosRequestConfig<P>,
    config?: PureHttpRequestConfig
  ): Promise<T> {
    return this.request<T>("get", url, params, config);
  }

  /** 单独抽离的`delete`工具函数 */
  public delete<T, P>(
    url: string,
    params?: AxiosRequestConfig<P>,
    config?: PureHttpRequestConfig
  ): Promise<T> {
    return this.request<T>("delete", url, params, config);
  }

  /** 单独抽离的`put`工具函数 */
  public put<T, P>(
    url: string,
    params?: AxiosRequestConfig<P>,
    config?: PureHttpRequestConfig
  ): Promise<T> {
    return this.request<T>("put", url, params, config);
  }
}

export const http = new PureHttp();
