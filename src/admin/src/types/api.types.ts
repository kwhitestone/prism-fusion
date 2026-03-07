// 基础API响应类型
export interface BaseApiResponse<T = any> {
  code: number;
  data: T;
  msg: string;
}

// 分页请求参数
export interface PageInfo {
  page: number;
  pageSize: number;
  keyword?: string;
}

// 分页响应数据
export interface PageResult<T = any> {
  list: T[];
  total: number;
  page: number;
  pageSize: number;
}

// 验证码响应
export interface CaptchaResponse {
  captchaId: string;
  picPath: string;
  captchaLength: number;
  openCaptcha: boolean;
}

// 权限信息类型
export interface Authority {
  createdAt?: string;
  updatedAt?: string;
  deletedAt?: string;
  authorityId: number;
  authorityName: string;
  parentId?: number;
  defaultRouter: string;
  dataAuthorityId?: Authority[];
  children?: Authority[];
}

// 菜单类型
export interface Menu {
  ID: number;
  createdAt: string;
  updatedAt: string;
  parentId: number;
  path: string;
  name: string;
  hidden: boolean;
  component: string;
  sort: number;
  meta: MenuMeta;
  authoritys?: Authority[];
  children?: Menu[];
  parameters?: MenuParameter[];
}

export interface MenuMeta {
  keepAlive: boolean;
  defaultMenu: boolean;
  title: string;
  icon: string;
  closeTab: boolean;
}

export interface MenuParameter {
  ID: number;
  createdAt: string;
  updatedAt: string;
  sysBaseMenuID: number;
  type: string;
  key: string;
  value: string;
}

// 文件上传类型
export interface FileUploadResponse {
  file: {
    name: string;
    url: string;
    tag: string;
    key: string;
  };
}

// 系统配置类型
export interface SystemConfig {
  env: string;
  addr: number;
  dbType: string;
  oss: string;
  multiLogin: boolean;
  iplimitcount: number;
  iplimittime: number;
  useMultipoint: boolean;
  useRedis: boolean;
  globalLanguage: string;
}

// 错误类型
export interface ApiError {
  code: number;
  message: string;
  details?: string;
}
