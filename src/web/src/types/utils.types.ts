// 通用工具类型

// 可选字段类型
export type Optional<T, K extends keyof T> = Omit<T, K> & Partial<Pick<T, K>>;

// 必需字段类型
export type Required<T, K extends keyof T> = Omit<T, K> & Pick<T, K>;

// 深度只读类型
export type DeepReadonly<T> = {
  readonly [P in keyof T]: T[P] extends object ? DeepReadonly<T[P]> : T[P];
};

// 深度可选类型
export type DeepPartial<T> = {
  [P in keyof T]?: T[P] extends object ? DeepPartial<T[P]> : T[P];
};

// 函数类型
export type Fn<T = void> = () => T;
export type AsyncFn<T = void> = () => Promise<T>;

// 事件处理器类型
export type EventHandler<E = Event> = (event: E) => void;
export type AsyncEventHandler<E = Event> = (event: E) => Promise<void>;

// HTTP方法类型
export type HttpMethod =
  | "GET"
  | "POST"
  | "PUT"
  | "DELETE"
  | "PATCH"
  | "HEAD"
  | "OPTIONS";

// HTTP状态码类型
export type HttpStatusCode =
  | 200 // OK
  | 201 // Created
  | 204 // No Content
  | 400 // Bad Request
  | 401 // Unauthorized
  | 403 // Forbidden
  | 404 // Not Found
  | 422 // Unprocessable Entity
  | 500; // Internal Server Error

// 时间戳类型
export type Timestamp = number;
export type DateString = string;

// ID类型
export type ID = string | number;

// 键值对类型
export type KeyValue<T = any> = Record<string, T>;

// 分页参数类型
export interface PaginationParams {
  page: number;
  pageSize: number;
  total?: number;
}

// 排序参数类型
export interface SortParams {
  field: string;
  order: "asc" | "desc";
}

// 筛选参数类型
export interface FilterParams {
  field: string;
  operator:
    | "eq"
    | "ne"
    | "gt"
    | "gte"
    | "lt"
    | "lte"
    | "like"
    | "in"
    | "not_in";
  value: any;
}

// 表格列定义类型
export interface TableColumn<T = any> {
  key: keyof T;
  title: string;
  width?: number | string;
  align?: "left" | "center" | "right";
  sortable?: boolean;
  filterable?: boolean;
  fixed?: "left" | "right";
  resizable?: boolean;
  ellipsis?: boolean;
  render?: (value: any, record: T, index: number) => any;
}

// 表单字段类型
export interface FormField {
  name: string;
  label: string;
  type:
    | "input"
    | "select"
    | "textarea"
    | "number"
    | "date"
    | "datetime"
    | "checkbox"
    | "radio"
    | "switch"
    | "upload";
  required?: boolean;
  placeholder?: string;
  options?: Array<{ label: string; value: any; disabled?: boolean }>;
  rules?: any[];
  props?: Record<string, any>;
}

// 文件类型
export interface FileInfo {
  name: string;
  size: number;
  type: string;
  url?: string;
  status?: "uploading" | "success" | "error";
  percent?: number;
  raw?: File;
}

// 树形数据类型
export interface TreeNode<T = any> {
  id: ID;
  label: string;
  children?: TreeNode<T>[];
  disabled?: boolean;
  data?: T;
}

// 下拉选择项类型
export interface SelectOption<T = any> {
  label: string;
  value: T;
  disabled?: boolean;
  children?: SelectOption<T>[];
}

// 环境变量类型
export interface EnvConfig {
  NODE_ENV: string;
  VITE_APP_TITLE: string;
  VITE_CLI_PORT: string;
  VITE_BASE_API: string;
  VITE_BASE_PATH: string;
  VITE_SERVER_PORT: string;
}

// Promise返回类型提取器
export type PromiseType<T extends Promise<any>> =
  T extends Promise<infer U> ? U : never;

// 数组元素类型提取器
export type ArrayElementType<T extends readonly any[]> =
  T extends readonly (infer U)[] ? U : never;

// 对象值类型提取器
export type ObjectValueType<T extends Record<string, any>> = T[keyof T];
