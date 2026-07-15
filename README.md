<p align="center">
  <img src="src/web/public/favicon.svg" width="80" alt="Prism Fusion Logo" />
</p>

<h1 align="center">Prism Fusion</h1>

<p align="center">
  <strong>纯净、全插件化的现代全栈框架</strong>
</p>

<p align="center">
  <a href="https://github.com/kwhitestone/prism-fusion/blob/master/LICENSE">
    <img src="https://img.shields.io/badge/license-MIT-blue?style=flat-square" alt="License" />
  </a>
  <img src="https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat-square&logo=go" alt="Go" />
  <img src="https://img.shields.io/badge/Vue-3.5-4FC08D?style=flat-square&logo=vue.js" alt="Vue" />
  <img src="https://img.shields.io/badge/Vite-7-646CFF?style=flat-square&logo=vite" alt="Vite" />
  <img src="https://img.shields.io/badge/Element%20Plus-409EFF?style=flat-square&logo=element&logoColor=white" alt="Element Plus" />
  <img src="https://img.shields.io/badge/TailwindCSS-4-06B6D4?style=flat-square&logo=tailwindcss" alt="TailwindCSS" />
</p>

<p align="center">
  <a href="#特性">特性</a> · <a href="#快速开始">快速开始</a> · <a href="#插件化架构">插件化架构</a> · <a href="#在业务项目中使用">在业务项目中使用</a> · <a href="#致谢">致谢</a>
</p>

---

## 简介

**Prism Fusion** 是一个基于 Vue 3 + Go 的现代化全栈框架，核心设计理念是 **纯净** 与 **全插件化**。

- **纯净**：框架核心极简，不预置任何业务逻辑，所有功能（认证、权限、仪表盘等）作为可插拔插件存在
- **全插件化**：前后端统一的插件体系，前端 `import.meta.glob` 自动发现、后端一行 `import` 即可加载，零配置启用
- **可扩展**：业务项目通过 git submodule 引用框架，自由组合内置插件或开发独立的业务插件

```
prism-fusion/
├── src/
│   ├── admin/                # 前端 (Vue 3 + Vite + Element Plus + TailwindCSS)
│   │   ├── src/addons/       # 前端内置插件（自动发现）
│   │   ├── src/plugin/       # 插件系统核心（loader / types）
│   │   ├── src/core/         # 框架核心模块（npm 导出入口）
│   │   └── src/views/        # 框架公共页面
│   └── server/               # 后端 (Go + Gin + Huma)
│       ├── addons/           # 后端内置插件（init() 自动注册）
│       ├── plugin/           # 插件系统核心（接口定义 + 注册表）
│       ├── router/           # 框架路由（Huma OpenAPI）
│       └── config.example.yaml
├── Dockerfile                # 独立部署的多阶段构建
├── docker-compose.yml        # 本地开发/调试服务编排
├── scripts/                  # entrypoint + supervisord
└── README.md
```

## 特性

### 核心亮点

- **🔌 全插件化架构** — 前后端对称的插件体系，功能即插即用
- **🧊 纯净内核** — 框架零业务耦合，认证、RBAC、仪表盘皆为可选插件
- **🔄 自动发现** — 前端 `import.meta.glob` + 后端 Go `init()` 双重自动注册
- **📐 类型安全** — Go `Plugin` 接口 + TypeScript `PluginModule` 类型全链路保障
- **🔀 Provider 可替换** — 认证 / 权限的 provider 可配置（`builtin` / 自定义 / `casbin` 等），业务项目无需改框架代码
- **📦 Submodule 友好** — 业务项目通过 git submodule + go.work + pnpm workspace 引用框架，独立版本管理

### 后端

| 特性 | 说明 |
|------|------|
| **Gin** | 高性能 HTTP 框架 |
| **Huma** | OpenAPI 3.1 文档自动生成（内置 ReDoc / Scalar UI） |
| **GORM** | ORM，支持 SQLite（默认零配置）/ MySQL |
| **Zap** | 结构化日志 + 日志轮转 |
| **Viper** | YAML 配置 + 环境变量覆盖 |
| **插件系统** | 实现 `Plugin` 接口即可扩展，支持优先级、中间件、模型自动迁移 |
| **优雅关闭** | 安全的服务生命周期管理 |

### 前端

| 特性 | 说明 |
|------|------|
| **Vue 3.5** + **Vite 7** | 极速开发体验 |
| **Element Plus** | 企业级 UI 组件库 |
| **TailwindCSS 4** | 原子化样式 |
| **Pinia** | 状态管理 |
| **TypeScript 5** | 全量类型覆盖 |
| **插件系统** | Vite glob 自动扫描加载，支持路由、组件、权限声明 |
| **插件注册上报** | 前端启动后自动将插件菜单/权限上报后端，便于后台统一管理 |
| **暗黑模式 / 多标签页 / 权限路由** | 开箱即用 |

## 快速开始

### 前置要求

| 工具 | 版本 |
|------|------|
| Go | >= 1.24 |
| Node.js | >= 22 |
| pnpm | >= 9 |
| Docker + Compose | 部署时需要 |

### 后端

```bash
cd src/server
cp config.example.yaml config.yaml   # 按需修改配置
go mod tidy
go run main.go
```

服务启动后默认监听 `:3180`（通过 `config.yaml` 中 `system.addr` 配置）：

- API 文档 (ReDoc)：http://localhost:3180/redoc
- API 文档 (Scalar)：http://localhost:3180/scalar
- OpenAPI JSON：http://localhost:3180/openapi.json

> 默认使用 SQLite，数据库文件 `prism_fusion.db` 自动创建于工作目录。如需 MySQL，在 `config.yaml` 中配置数据库连接即可。

### 前端

```bash
cd src/web
pnpm install
pnpm dev
```

开发服务器：http://localhost:3188（已配置代理，`/api` 请求转发到后端 `:3180`）

### Docker 部署

```bash
# 独立构建
docker build -t prism-fusion .

# 使用 Compose（builtin 认证）
cp .env.example .env   # 按需修改
docker compose up -d
```

## 插件化架构

Prism Fusion 的核心竞争力在于**前后端统一的全插件化架构**。

### 设计理念

| 原则 | 说明 |
|------|------|
| **高内聚低耦合** | 插件内部自包含（路由、模型、服务、中间件），与框架通过接口解耦 |
| **零配置加载** | 前端完全自动发现，后端仅需一行 import |
| **Provider 可替换** | 同一功能域（如认证）可由不同插件提供实现，通过 `config.yaml` 切换 |
| **类型安全** | Go `Plugin` 接口 + TypeScript `PluginModule` 类型保障开发体验 |
| **前后端对称** | 目录结构一致，便于团队协作 |

### 后端插件

实现 `Plugin` 接口并通过 `init()` 自动注册：

```go
// addons/my-plugin/plugin.go
package myplugin

import "whitestone.top/prism-fusion/plugin"

func init() {
    plugin.Register(&MyPlugin{
        BasePlugin: plugin.BasePlugin{
            PluginName:        "my-plugin",
            PluginDescription: "我的插件",
        },
    })
}

type MyPlugin struct {
    plugin.BasePlugin
}

func (p *MyPlugin) Priority() int    { return 100 }  // 默认优先级
func (p *MyPlugin) RegisterRoutes(api huma.API) { /* 注册 Huma 路由 */ }
func (p *MyPlugin) Models() []interface{} { return []interface{}{&MyModel{}} }
```

在 `addons/addons.go` 中添加一行导入即可启用：

```go
import _ "whitestone.top/prism-fusion/addons/my-plugin"
```

#### Plugin 接口

```go
type Plugin interface {
    Name() string                          // 唯一标识
    Description() string                   // 描述
    Priority() int                         // 优先级（越小越先执行）
    RoutePrefix() string                   // 路由前缀（限定中间件作用域）
    RegisterRoutes(api huma.API)           // 注册 Huma 路由
    Models() []interface{}                 // 需要自动迁移的 GORM 模型
    Middlewares() []gin.HandlerFunc        // 插件级中间件
    GlobalMiddlewares() []gin.HandlerFunc  // 全局中间件
}
```

### 前端插件

导出 `PluginModule` 到 `addons/*/index.ts`，**无需任何配置**自动加载：

```typescript
// addons/my-plugin/index.ts
import type { PluginModule } from "@/plugin/types";

const plugin: PluginModule = {
  name: "my-plugin",
  description: "我的插件",
  version: "1.0.0",
  routes: [/* Vue Router 路由配置 */],
  permissions: [
    { key: "my-plugin:create", name: "新建" },
    { key: "my-plugin:delete", name: "删除" },
  ],
  setup() {
    // 插件初始化逻辑（注入策略、注册处理器等）
  },
};
export default plugin;
```

#### PluginModule 接口

```typescript
interface PluginModule {
  name: string;                              // 唯一标识（与后端对应）
  description?: string;                      // 描述
  version?: string;                          // 版本
  routes?: RouteRecordRaw[];                 // 路由配置
  permissions?: PluginPermission[];          // 权限声明
  components?: Record<string, Component>;    // 全局组件
  install?: (app: App) => void;              // Vue 插件钩子
  setup?: () => void | Promise<void>;        // 初始化钩子
  destroy?: () => void;                      // 卸载钩子
}
```

### 自动发现机制

| 端 | 机制 | 触发方式 |
|---|------|---------|
| 后端 | Go `init()` + `plugin.Register()` | 在 `addons/addons.go` 中 import 插件包 |
| 前端 | Vite `import.meta.glob("../addons/*/index.ts")` | 自动扫描，无需手动注册 |
| 联动 | 前端启动后上报插件注册表到后端 | `POST /api/v1/system/plugin-registry` |

### 内置插件

框架内置两个基础插件，提供开箱即用的认证与权限能力：

| 插件 | 说明 | Provider | 优先级 |
|------|------|----------|--------|
| `auth` | JWT 登录、注册、Token 刷新、用户管理 | `builtin` | 10 |
| `rbac` | 角色、权限、动态路由管理 | `builtin` | 20 |

> 业务项目可通过注册同名但不同 provider 的插件来替换内置实现（如 `oauth-auth` 替换 `auth`），在 `config.yaml` 中切换 `auth.provider` / `rbac.provider` 即可。

## 在业务项目中使用

Prism Fusion 设计为通过 **git submodule** 方式在业务项目中引用，参见 [prism-example-site](https://github.com/kwhitestone/prism-example-site) 获取完整示例。

### 业务项目结构

```
my-project/
├── prism-fusion/                  # git submodule（框架源码）
├── app/
│   ├── src/server/                # Go 后端
│   │   ├── addons/                # 业务插件
│   │   ├── go.mod
│   │   ├── go.work                # 引用框架: use ../../prism-fusion/src/server
│   │   └── main.go
│   └── src/web/                 # Vue 前端
│       ├── src/addons/            # 业务前端插件
│       ├── pnpm-workspace.yaml    # 引用框架: ../../prism-fusion/src/web
│       ├── vite.config.ts         # alias @ → 框架 src
│       └── package.json           # 依赖 "prism-fusion-admin": "workspace:*"
├── docker-compose.yaml
└── .env
```

### 关键集成点

| 层 | 机制 | 说明 |
|----|------|------|
| Go 后端 | `go.work` + `replace` | 将 `whitestone.top/prism-fusion` 指向本地 submodule |
| Vue 前端 | pnpm workspace | 框架作为 workspace 包，业务项目直接 import |
| Vite | alias `@` → 框架 src | 框架内部 `@/` 引用自动解析到正确路径 |
| 插件 | 框架内置 + 业务自定义 | 两者并存，统一自动发现加载 |

## 配置参考

`config.yaml` 核心配置项：

```yaml
system:
  env: local            # local / public
  addr: 3180            # 服务端口

auth:
  provider: builtin     # 认证 provider（builtin / custom / ...）

rbac:
  provider: builtin     # 权限 provider（builtin / casbin / ...）

jwt:
  signing-key: change-me-in-production
  expires-time: 7d

sqlite:
  path: ./prism_fusion.db
```

完整配置见 [config.example.yaml](src/server/config.example.yaml)。

## 技术栈

| 层 | 技术 |
|----|------|
| 前端框架 | Vue 3.5 + TypeScript 5 |
| 构建工具 | Vite 7 |
| UI 框架 | Element Plus |
| 样式方案 | TailwindCSS 4 |
| 状态管理 | Pinia |
| 后端框架 | Go 1.24+ / Gin |
| API 文档 | Huma (OpenAPI 3.1) + ReDoc / Scalar |
| ORM | GORM (SQLite / MySQL) |
| 日志 | Zap + file-rotatelogs |
| 配置 | Viper |
| 容器化 | Docker 多阶段构建 + Supervisor |

## 致谢

Prism Fusion 的诞生离不开以下优秀的开源项目：

- **[vue-pure-admin](https://github.com/pure-admin/vue-pure-admin)** — 前端部分参考了 vue-pure-admin 的优秀设计与实现
- **[Gin](https://github.com/gin-gonic/gin)** — 高性能 Go HTTP 框架
- **[Huma](https://github.com/danielgtaylor/huma)** — 优雅的 OpenAPI 3.1 框架
- **[GORM](https://github.com/go-gorm/gorm)** — Go 生态最流行的 ORM
- **[Element Plus](https://github.com/element-plus/element-plus)** — 企业级 Vue 3 组件库
- **[Vite](https://github.com/vitejs/vite)** — 下一代前端构建工具
- **[Casbin](https://github.com/casbin/casbin)** — 强大的权限管理库
- **[Zap](https://github.com/uber-go/zap)** — 高性能结构化日志
- **[Viper](https://github.com/spf13/viper)** — Go 配置管理

## 许可证

[MIT License](LICENSE) © 2025-present [kwhitestone](https://github.com/kwhitestone)

前端部分基于 [vue-pure-admin](https://github.com/pure-admin/vue-pure-admin)（MIT 协议）进行开发，已保留原始许可声明。
