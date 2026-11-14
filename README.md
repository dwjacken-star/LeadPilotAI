# 领客AI · LeadPilot AI

面向 B2B 团队的业务资料、智能获客、内容营销与客户跟进平台。  
A platform for B2B materials, intelligent prospecting, content marketing and customer follow-up.

## 文档与操作手册 / Documentation

**[中英文文档中心 / Bilingual documentation hub](docs/handbook/README.md)**

| 内容 / Content | 中文 | English |
|---|---|---|
| 技术架构 / Technical architecture | [中文](docs/handbook/zh-CN/technical-architecture.md) | [English](docs/handbook/en/technical-architecture.md) |
| 功能架构 / Functional architecture | [中文](docs/handbook/zh-CN/functional-architecture.md) | [English](docs/handbook/en/functional-architecture.md) |
| 项目操作手册 / User manual | [中文](docs/handbook/zh-CN/user-manual.md) | [English](docs/handbook/en/user-manual.md) |
| 部署、维护与排障 / Operations | [中文](docs/handbook/zh-CN/operations-guide.md) | [English](docs/handbook/en/operations-guide.md) |
| 含界面图的 PDF / Illustrated PDF | [下载中文版](docs/handbook/pdf/LeadPilotAI-User-and-Operations-Guide-zh-CN.pdf) | [Download English](docs/handbook/pdf/LeadPilotAI-User-and-Operations-Guide-en.pdf) |

PDF 和在线手册中的界面图由真实前端组件与虚构数据生成，带“演示/非生产”标识，不包含真实账号、客户或密钥。  
Interface captures use the real frontend with labeled fictional data, not production accounts, customers or secrets.

## 主要功能 / Capabilities

- 部门、角色数据范围、资源与发件邮箱授权、共享发送策略。  
  Departments, role-bound scope, resource/sender grants and shared policies.
- 联系人、标签、邮件模板、营销任务、计划、订阅自动化和发信 API。  
  Contacts, tags, templates, campaigns, schedules, subscription automation and send APIs.
- 公司/产品资料、文件识别、审核发布和版本管理。  
  Company/product materials, extraction, approval and versioning.
- 公开来源证据、联系人验证、AI 开发信、询盘与 CRM 跟进。  
  Public-source evidence, contact verification, AI outreach, inquiries and CRM follow-up.
- 行业包、14 项业务技能和 6 类持久化智能情报工作流。  
  Industry packs, 14 skills and 6 durable intelligence workflows.
- 多账号、多语言社媒内容、人工审核、API/人工发布模式与官方指标。  
  Multi-account/localized social content, review, API/manual modes and official metrics.
- 统一 AI 模型注册中心、凭据加密、能力筛选和预算。  
  Unified AI registry, encrypted credentials, capability selection and budgets.

## 技术与项目结构 / Technology and Structure

Go / GoFrame、Vue 3 / TypeScript、PostgreSQL、Redis、Docker Compose，以及邮件协议与 Webmail 组件。后端是模块化单体；业务技能不是独立部署的微服务。  
Go/GoFrame, Vue 3/TypeScript, PostgreSQL, Redis, Docker Compose, mail protocol and Webmail components. The backend is a modular monolith; skills are not separate microservices.

- `core/frontend/`：管理台 / management UI
- `core/internal/controller/`：接口层 / API controllers
- `core/internal/service/`：业务、队列、权限与集成 / domain services, work queues, authorization and integrations
- `Dockerfiles/`、`docker-compose*.yml`：部署构建 / deployment definitions
- `docs/handbook/`：架构、操作指南、截图、HTML 和 PDF / architecture, manuals, screenshots, HTML and PDFs
- `scripts/docs/`：文档与演示截图生成 / documentation and demonstration capture tools

## 部署与发布边界 / Deployment and Publication

**先阅读运维指南，不要将源码克隆当作安装完成。** 按发布要求，本仓库不提交 SQL、环境凭据、私钥、运行数据、应用安装包或编译产物；后端 `go:embed` 依赖的 SQL 需通过私有渠道补齐。现有生产配置与主密钥不能覆盖。

**Read the Operations Guide before deployment.** SQL, runtime credentials, private keys, operational data, application installers and build output are excluded. Supply matching private SQL required by backend embeds. Preserve existing production configuration and encryption keys.

- [源码发布与敏感信息规则](docs/github-publishing.md)
- [.env.example：无凭据的配置模板](.env.example)
- 提交前检查 / Before committing：`node scripts/check-publish.cjs`
- 本地保护钩子 / Local guard：`git config core.hooksPath .githooks`

截图、草稿保存和配置成功不代表真实邮件收件、平台授权、公开发布或付费模型调用已通过验收。  
Screenshots, saved drafts and saved settings do not prove final mail receipt, platform approval/publication or paid-model operation.

## 许可证 / License

许可证与第三方版权声明按 [LICENSE](LICENSE) 及源码中的声明保留。  
See [LICENSE](LICENSE) and retained source notices for licensing and third-party rights.
