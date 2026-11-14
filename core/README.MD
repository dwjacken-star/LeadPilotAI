# LeadPilot AI Backend

Go / GoFrame 后端：业务 API、部门权限、邮件调度、CRM、资料中心、智能获客、社媒与模型集成。  
Go/GoFrame backend for APIs, authorization, mail scheduling, CRM, materials, prospecting, social and model integrations.

- [中文技术架构](../docs/handbook/zh-CN/technical-architecture.md)
- [English technical architecture](../docs/handbook/en/technical-architecture.md)
- [中文运维指南](../docs/handbook/zh-CN/operations-guide.md)
- [English operations guide](../docs/handbook/en/operations-guide.md)

正式入口为 `main.go`。构建前准备匹配的私有 SQL 和运行配置；公开仓库不包含所有 `go:embed` SQL 依赖。不要将开发入口与正式入口一起编译。  
Use `main.go` as the production entry. Supply matching private SQL dependencies/configuration before building; a public clone does not contain every embedded SQL input. Do not combine development and production entry points.

生产部署使用匹配的自定义镜像、前端资源和数据库迁移；不要覆盖已有环境、邮箱数据或主密钥。  
Deploy matching custom images, frontend assets and migrations. Preserve environment, mail data and encryption keys.
