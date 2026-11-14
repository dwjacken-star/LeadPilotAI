# LeadPilot AI Management UI

Vue 3 / TypeScript / Rsbuild 管理台，使用 Pinia、Vue Router、多语言资源和业务组件。  
Vue 3/TypeScript/Rsbuild console with Pinia, Vue Router, localization and domain components.

## Local development / 本地开发

Use Node.js 22 and pnpm compatible with the checked-in lockfile:

```sh
pnpm install --frozen-lockfile
pnpm dev
```

Use the actual backend/environment configuration; do not put credentials into frontend source.  
按实际后端环境配置连接，不把凭据写入前端源码。

## Build and checks / 构建与检查

```sh
pnpm build
pnpm i18n:check
pnpm test:workspace
pnpm test:repair
```

`dist/` is ignored. Use ordinary `build` for build-only work; Git/deployment shortcut scripts have additional side effects.  
`dist/` 不提交。仅构建使用普通 `build`，Git/部署快捷脚本具有额外副作用。

## Guides / 指南

- [中文操作手册](../../docs/handbook/zh-CN/user-manual.md)
- [English user manual](../../docs/handbook/en/user-manual.md)
- [中英文文档中心 / Documentation hub](../../docs/handbook/README.md)

Documentation screenshots use this application's real UI with fictional intercepted API data. They are not production data or backend integration acceptance.  
文档截图使用真实前端和虚构 API 数据，不代表生产实况或后端联调验收。
