# GitHub 源码同步与敏感信息

目标仓库：https://github.com/dwjacken-star/LeadPilotAI.git

## 提交范围

提交应用源码、测试代码、依赖清单及锁文件、Dockerfile、通用部署脚本、静态资源和文档。

`.gitignore` 默认排除打包产物、安装包、编译产物、依赖缓存、运行数据、日志、私钥和凭据文件，以及 SQL 脚本和数据库备份。`env_init` 是包含私有配置的本地文件，不提交；无凭据的配置模板位于 `.env.example`。

安装脚本源码（如 `install.sh`）和 Dockerfile 不是安装包，仍然提交。

## 邮箱密码

源码和前端不再提供固定邮箱密码。界面创建单个或批量邮箱时需要填写密码。

后端创建、批量创建和导入接口收到空密码时，依次读取进程环境变量和项目私有 `.env` 中的 `DEFAULT_NEW_MAILBOX_PASSWORD`。进程变量优先，显式设置为空会禁用文件回退。默认密码至少需要 8 个字符；没有有效配置时，后端拒绝创建，不会使用源码内置密码。

已存在邮箱的密码和更新时留空保留现有密码的行为不变。变更不修改服务器或数据库中的现有密码。若旧固定密码曾用于真实账号，应另行更换这些账号的密码。

## 数据库脚本不发布的影响

根据发布要求，所有 `.sql` 文件不进入此仓库，包括数据库迁移和测试夹具。以下 Go 源码仍然通过 `go:embed` 引用这些私有文件：

- `core/internal/service/governance/`：`migration.sql`、`growth.sql`、`delivery.sql`、`statistics.sql`、`social.sql`、`master.sql`。
- `core/internal/testsupport/`：`base.sql`。

因此，**仅克隆 GitHub 仓库不能直接完成后端构建或测试**。构建者必须通过私有渠道取得匹配版本的数据库脚本，放回对应路径；这些文件仍被 Git 忽略。本地原始文件不删除，也不将 SQL 伪装成其他扩展名提交。

## 提交前检查

```sh
git status --short
git ls-files -ci --exclude-standard
git diff --cached --stat
node scripts/check-publish.cjs
```

第二条命令正常情况下应无输出。忽略规则不会自动移除已经被 Git 跟踪的文件。

本项目提供 `.githooks/pre-commit`，可通过以下命令在本地启用：

```sh
git config core.hooksPath .githooks
```

钩子检查暂存文件是否违反忽略规则，并检测常见私钥、访问令牌和带密码的连接字符串。钩子可被绕过，模式检测也不能发现所有凭据；提交前仍需要审查源码和配置中的硬编码值。若敏感信息曾进入历史，仅新增忽略规则无效，必须撤销凭据并另行清理历史。
