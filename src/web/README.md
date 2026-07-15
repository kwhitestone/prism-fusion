# Prism Fusion — 前端

[![license](https://img.shields.io/github/license/kwhitestone/prism-fusion.svg)](LICENSE)

**中文** | [English](./README.en-US.md)

## 简介

Prism Fusion 前端基于 Vue 3 + Vite + Element Plus + TailwindCSS 构建，采用**全插件化架构**，通过 `import.meta.glob` 实现插件自动发现与零配置加载。

前端部分参考了 [vue-pure-admin](https://github.com/pure-admin/vue-pure-admin) 的优秀设计，使用了 `@pureadmin/table`、`@pureadmin/utils` 等组件。

## 开发

```bash
pnpm install
pnpm dev
```

## 构建

```bash
pnpm build
```

## 插件开发

将插件放入 `src/addons/your-plugin/` 目录，导出 `PluginModule` 即可自动加载，无需任何额外配置。

详见项目根目录 [README.md](../../README.md) 的插件化架构章节。

## 许可证

[MIT © 2025-present, kwhitestone](./LICENSE)
