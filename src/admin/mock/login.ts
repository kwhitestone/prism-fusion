// 用户登录已迁移到后端UC Token验证
// 此mock文件已废弃，保留空导出以避免构建错误
import { defineFakeRoute } from "vite-plugin-fake-server/client";

// 不再mock登录接口，直接使用后端API
export default defineFakeRoute([]);
