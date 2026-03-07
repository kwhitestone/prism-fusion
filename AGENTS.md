# AI Agent 贡献规则

## 基本原则

- **禁止** 自行启动测试服务或运行 `go run`、`pnpm dev` 等命令
- 修改代码前先理解现有结构，遵循已有模式

## 项目结构

| 目录 | 说明 |
|------|------|
| `src/admin/` | Vue 3 前端，使用 pnpm |
| `src/server/` | Go 后端，使用 Gin + Huma |

## 后端规范 (Go)

- API 路由放 `api/v1/{模块}/`，并在 `api/v1/enter.go` 注册
- 业务逻辑放 `service/`，并在 `service/enter.go` 注册
- 插件/扩展放 `addons/`，通过 `init()` 自动注册
- 使用 `global.PRISM_LOG` 记录日志
- 使用 `global.PRISM_DB` 操作数据库

## 前端规范 (Vue)

- 组件放 `src/components/`
- 页面放 `src/views/`
- API 调用放 `src/api/`
- 状态管理放 `src/store/`

## 代码风格

- Go: 遵循 gofmt
- Vue/TS: 遵循项目 ESLint 配置，前端代码修改后必须通过 `pnpm lint` 检查
