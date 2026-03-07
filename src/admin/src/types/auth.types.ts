// API响应类型定义
export interface BaseResponse<T = any> {
  code: number;
  data: T;
  msg: string;
}

// Token结果类型
export interface TokenResult {
  accessToken: string;
  refreshToken: string;
  expiresAt: string;
  tokenType: string;
}

// 登录请求类型
export interface LoginRequest {
  username: string;
  password: string;
  captchaId?: string;
  captcha?: string;
}

// 用户信息类型
export interface UserInfo {
  ID: number;
  uuid: string;
  username?: string;
  nickName: string;
  headerImg: string;
  phone?: string;
  email?: string;
  enable?: number;
  authority: {
    authorityId: number;
    authorityName: string;
    defaultRouter: string;
  };
  authorities?: any[];
  originSetting?: Record<string, any>;
}

// 登录响应类型
export interface LoginResponse {
  user: UserInfo;
  token: TokenResult;
  expiresAt: number;
}

// 注册请求类型
export interface RegisterRequest {
  username: string;
  password: string;
  nickName: string;
}

// 刷新Token请求类型
export interface RefreshTokenRequest {
  refreshToken: string;
}
