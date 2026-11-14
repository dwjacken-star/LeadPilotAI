# LeadPilot AI 文档中心 / Documentation Hub

**版本 / Version：1.0 · 2026-11-14**  
源码分析基线 / Source baseline：`cf57fb3`

## 下载 PDF / Download PDF

- [中文版：技术架构、功能架构与操作运维指南](pdf/LeadPilotAI-User-and-Operations-Guide-zh-CN.pdf)
- [English: Architecture, User and Operations Handbook](pdf/LeadPilotAI-User-and-Operations-Guide-en.pdf)

PDF 含中英文各 16 张真实前端组件的功能界面截图，以及各 3 张架构/流程图。截图使用虚构演示数据并带明显标识，不是生产截图，也不代表实际邮件、AI 或社媒验收。

Each edition includes 16 real-frontend interface captures and 3 architecture/lifecycle diagrams. Captures use clearly labeled fictional data, not production records or evidence of live delivery/provider acceptance.

## Markdown 指南 / Markdown Guides

| 内容 / Content | 中文 | English |
|---|---|---|
| 技术架构 / Technical architecture | [技术架构](zh-CN/technical-architecture.md) | [Technical architecture](en/technical-architecture.md) |
| 功能架构 / Functional architecture | [功能架构](zh-CN/functional-architecture.md) | [Functional architecture](en/functional-architecture.md) |
| 用户操作 / User manual | [操作手册](zh-CN/user-manual.md) | [User manual](en/user-manual.md) |
| 部署、维护与排障 / Operations | [运维指南](zh-CN/operations-guide.md) | [Operations guide](en/operations-guide.md) |

## 可打印 HTML / Printable HTML

- [中文 HTML](html/LeadPilotAI-User-and-Operations-Guide-zh-CN.html)
- [English HTML](html/LeadPilotAI-User-and-Operations-Guide-en.html)

下载 HTML 时需保留 `assets` 相对目录。For offline HTML viewing, preserve the relative `assets` directory.

## 图源、来源和验证 / Sources and Validation

- [技术架构 Mermaid](assets/diagrams/technical.mmd)
- [功能架构 Mermaid](assets/diagrams/functional.mmd)
- [截图来源与前端指纹 / Screenshot provenance](assets/screenshot-provenance.json)
- [生成结果 / Build manifest](build-manifest.json)
- [PDF、链接与图片验证 / Validation report](validation-report.json)

## 阅读建议 / Suggested Reading

- 管理员 / Administrators：运维指南 → 权限、邮箱与 AI 配置 → 操作手册。
- 业务人员 / Operators：操作手册 → 功能架构。
- 开发与维护 / Developers：技术架构 → 运维指南 → 对应源码。

SQL、运行配置、私钥和凭据不在公开仓库中；仅克隆仓库不能完成后端构建，详情见运维指南。  
SQL and runtime secrets remain private. A public clone alone cannot build the backend; see the Operations Guide.
