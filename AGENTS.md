# prism-fusion

全栈框架：Go 后端 (Gin + Huma + GORM) + Vue 前端 (Vite + Element Plus)。

## 构建

```bash
# 后端
cd src/server && go build ./...

# 前端
cd src/web && pnpm install && pnpm build
```

## 架构约束

- 后端：插件化架构，所有功能通过 `plugin.Plugin` 接口 + `init()` 注册
- 前端：插件化架构，`import.meta.glob` 自动发现 `addons/*/index.ts`
- 业务项目通过 git submodule + go.work + pnpm workspace 引用本框架
- 框架保持纯净：零业务逻辑，auth/RBAC 等都是可选 addon
- `src/admin` 已改名为 `src/web`，禁止恢复旧名

## 目录结构

```
src/
├── server/           # Go 后端
│   ├── addons/       # 内置插件 (auth, rbac)
│   ├── plugin/       # 插件系统核心 (接口 + 注册表)
│   ├── core/         # 框架核心 (server, viper, zap)
│   ├── global/       # 全局变量 (PRISM_DB, PRISM_LOG, PRISM_CONFIG)
│   ├── initialize/   # 初始化 (Gorm, Router, Tables)
│   └── config/       # 配置定义
└── web/              # Vue 前端
    ├── src/addons/   # 前端内置插件
    ├── src/plugin/   # 插件系统核心 (loader, types)
    └── src/core/     # 框架核心模块
```

## 修改规范

- 修改 `plugin/plugin.go` 的 Plugin 接口需要向后兼容（新方法加默认实现）
- 修改 `global/global.go` 的全局变量需要评估对业务项目的影响
- 前端 `plugin/types.ts` 的 PluginModule 接口变更需要向后兼容

## 边界

- **Always**: 框架改动后跑 `go build ./...` 和 `pnpm build` 验证
- **Ask first**: 修改 Plugin 接口（影响所有业务项目）
- **Never**: 禁止在框架里写业务逻辑
- **Never**: 禁止把 src/web 改回 src/admin
