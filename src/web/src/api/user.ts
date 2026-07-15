import service from "@/utils/request";
import type { BaseResponse, UserInfo } from "@/types";

export type UserResult = {
  success: boolean;
  message?: string;
  /** 是否需要图形验证码 */
  requireCaptcha?: boolean;
  /** UC会话ID（需要验证码时使用） */
  sessionId?: string;
  /** UC会话密钥（需要验证码时使用） */
  sessionKey?: string;
  /** 验证码图片URL */
  captchaUrl?: string;
  data?: {
    /** 头像 */
    avatar: string;
    /** 用户名 */
    username: string;
    /** 昵称 */
    nickname: string;
    /** 当前登录用户的角色 */
    roles: Array<string>;
    /** 按钮级别权限 */
    permissions: Array<string>;
    /** `token` */
    accessToken: string;
    /** 用于调用刷新`accessToken`的接口时所需的`token` */
    refreshToken: string;
    /** `accessToken`的过期时间（格式'xxxx/xx/xx xx:xx:xx'） */
    expires: Date;
  };
};

export type RefreshTokenResult = {
  success: boolean;
  data: {
    /** `token` */
    accessToken: string;
    /** 用于调用刷新`accessToken`的接口时所需的`token` */
    refreshToken: string;
    /** `accessToken`的过期时间（格式'xxxx/xx/xx xx:xx:xx'） */
    expires: Date;
  };
};

// 注意：登录已改为前端直接调用 UC，不再需要后端接口
// getLogin, loginByToken, login, getUserInfo 已移除

/**
 * 用户注册
 * @param data 注册信息
 * @returns Promise<BaseResponse>
 */
export const register = (data: {
  username: string;
  password: string;
  nickName: string;
  headerImg?: string;
  authorityId?: number;
}): Promise<BaseResponse> => {
  return service({
    url: "/user/admin_register",
    method: "post",
    data
  });
};

/**
 * 修改密码
 * @param data 密码信息
 * @returns Promise<BaseResponse>
 */
export const changePassword = (data: {
  username: string;
  password: string;
  newPassword: string;
}): Promise<BaseResponse> => {
  return service({
    url: "/user/changePassword",
    method: "post",
    data
  });
};

/**
 * 重置密码
 * @param data 重置信息
 * @returns Promise<BaseResponse>
 */
export const resetPassword = (data: { ID: number }): Promise<BaseResponse> => {
  return service({
    url: "/user/resetPassword",
    method: "post",
    data
  });
};

/**
 * 设置用户信息
 * @param data 用户信息
 * @returns Promise<BaseResponse>
 */
export const setUserInfo = (data: Partial<UserInfo>): Promise<BaseResponse> => {
  return service({
    url: "/user/setUserInfo",
    method: "put",
    data
  });
};

/**
 * 设置自身信息
 * @param data 用户信息
 * @returns Promise<BaseResponse>
 */
export const setSelfInfo = (data: Partial<UserInfo>): Promise<BaseResponse> => {
  return service({
    url: "/user/setSelfInfo",
    method: "put",
    data
  });
};
