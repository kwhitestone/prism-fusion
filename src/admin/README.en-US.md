# Prism Fusion — Frontend

[![license](https://img.shields.io/github/license/kwhitestone/prism-fusion.svg)](LICENSE)

**English** | [中文](./README.md)

## Introduction

The Prism Fusion frontend is built with Vue 3 + Vite + Element Plus + TailwindCSS, featuring a **fully plugin-based architecture** with automatic plugin discovery via `import.meta.glob` — zero configuration required.

The frontend design was inspired by [vue-pure-admin](https://github.com/pure-admin/vue-pure-admin) and uses components like `@pureadmin/table` and `@pureadmin/utils`.

## Development

```bash
pnpm install
pnpm dev
```

## Build

```bash
pnpm build
```

## Plugin Development

Place your plugin in `src/addons/your-plugin/`, export a `PluginModule`, and it will be automatically loaded — no extra configuration needed.

See the Plugin Architecture section in the root [README.md](../../README.md) for details.

## License

[MIT © 2025-present, kwhitestone](./LICENSE)
