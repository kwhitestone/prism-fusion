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

// Concrete authentication HTTP operations are owned by addons/auth/api.
