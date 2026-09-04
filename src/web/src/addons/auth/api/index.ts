import service from "@/utils/request";

const AUTH_BASE = "/api/v1/addons/auth";

/** 登录请求 */
export const login = (data: { username: string; password: string }) => {
  return service({
    url: `${AUTH_BASE}/login`,
    method: "post",
    data,
    donNotShowLoading: true
  });
};

/** 注册请求 */
export const register = (data: {
  username: string;
  password: string;
  nickName?: string;
}) => {
  return service({
    url: `${AUTH_BASE}/register`,
    method: "post",
    data
  });
};

/** 刷新 Token */
export const refreshToken = (data: {
  refreshToken: string;
  requestId: string;
}) => {
  return service({
    url: `${AUTH_BASE}/refresh-token`,
    method: "post",
    data: { refreshToken: data.refreshToken },
    headers: { "X-Refresh-Request-ID": data.requestId },
    donNotShowLoading: true
  });
};

/** 注销并吊销 refresh token family */
export const logout = (data: { refreshToken: string }) => {
  return service({
    url: `${AUTH_BASE}/logout`,
    method: "post",
    data,
    donNotShowLoading: true
  });
};

/** 获取当前用户信息 */
export const getUserInfo = () => {
  return service({
    url: `${AUTH_BASE}/user-info`,
    method: "get",
    donNotShowLoading: true
  });
};

/** 修改密码 */
export const changePassword = (data: {
  username: string;
  password: string;
  newPassword: string;
}) => {
  return service({
    url: `${AUTH_BASE}/change-password`,
    method: "post",
    data
  });
};
