# 项目部署、维护与故障处理指南

**领客AI · LeadPilot AI｜中文运维指南｜版本 1.0｜2026-11-14**

本指南提供操作步骤，不表示本次已执行生产部署、备份或恢复。示例在项目根目录执行，服务名保留源码中的实际 Compose 名称。修改生产前必须确认目标、备份、停机窗口和回滚负责人。

## 1. 发布文件与私有依赖

GitHub 包含应用源码、文档、演示截图和本次明确交付的 PDF。普通安装包、压缩包、编译产物、运行数据、SQL、私钥和凭据保持忽略。PDF 是本次用户授权的文档交付物，不是应用安装包。

**仅克隆不能完成后端构建。** 下面的 Go 源码通过 `go:embed` 引用被忽略的私有 SQL：

- `core/internal/service/governance/`：`migration.sql`、`growth.sql`、`delivery.sql`、`statistics.sql`、`social.sql`、`master.sql`。
- `core/internal/testsupport/`：`base.sql`。

另外，初始化脚本、环境、证书和目标实例已有运行配置应由管理员私有管理。取得与版本配套的文件后放回原路径，不将 SQL 改名隐藏提交。文档与源码归档不能替代完整可运行备份。

## 2. 环境与配置准备

建议先在隔离 Linux/Docker 环境安装：Docker Engine/Compose、Node.js 22、与锁文件匹配的 pnpm，以及 Go 1.23.2。Windows 原生 Go 构建不等于完整 Linux 邮件服务可运行。

1. 克隆自己的仓库并确认版本：

```sh
git clone https://github.com/dwjacken-star/LeadPilotAI.git
cd LeadPilotAI
git log -1 --oneline
```

2. 按私有流程补齐数据库脚本和必要配置，检查每个 `go:embed` 引用。
3. 将 `.env.example` 复制为 `.env`，在私有环境中填写数据库/Redis/管理员配置；生产已有 `.env` 不覆盖。
4. 设置主机名、SafePath、HTTPS、真实域名和受控邮件出口；不要沿用示例域名。
5. 给 `.env`、私钥、资料主密钥和备份设置限制访问的文件权限。
6. 检查数据库/Redis 端口只绑定本地，且 Core 的 Docker socket 权限符合信任模型。

新增邮箱的空密码后端回退由私有 `DEFAULT_NEW_MAILBOX_PASSWORD` 控制，未配置时拒绝空密码创建。不要在源码重新加入默认密码。

## 3. 本地构建与验证

### 3.1 前端

```sh
cd core/frontend
pnpm install --frozen-lockfile
pnpm build
pnpm i18n:check
pnpm test:workspace
pnpm test:repair
cd ../..
```

前端产物在 `core/frontend/dist`，部署时由 Dockerfile 复制到 Core 公共资源目录。`build:git` 含 Git 拉取和复制行为，`build:deploy` 含部署脚本；仅验证构建时使用普通 `build`，不要误触部署快捷入口。

### 3.2 后端

先补齐私有 SQL，再在 `core` 中明确构建正式入口：

```sh
cd core
go mod download
go build -mod=readonly -trimpath -o ../.tools/leadpilot-core main.go
go test ./internal/service/mail_boxes ./internal/controller/mail_boxes ./internal/controller/settings
cd ..
```

仓库存在开发入口，不应未经核查使用 `go build .` 把多个 main 一起编译。集成测试在设置 `BM_TEST_PG_HOST` 等变量后会创建/清理测试库；只允许指向具有创建/删除权限的**隔离测试实例**，不指向生产。未配置时部分测试会跳过，跳过不代表完整验证通过。

### 3.3 文档保护检查

```sh
node --test scripts/check-publish.test.cjs
git ls-files -ci --exclude-standard
node scripts/check-publish.cjs
```

发布检查以暂存区为准，不是全面安全审计。代码内的简单字符串密码、业务隐私或图片信息仍需人工审查。

## 4. Compose 部署

默认编排引用上游 Core 镜像。部署定制代码应使用定制覆盖：

```sh
docker compose -f docker-compose.yml -f docker-compose.enterprise.yml config --quiet
docker compose -f docker-compose.yml -f docker-compose.enterprise.yml build core-billionmail
```

在已批准的变更窗口、完成备份并确认脚本版本后启动/更新：

```sh
docker compose -f docker-compose.yml -f docker-compose.enterprise.yml up -d
```

这些是部署示例，不是本次已执行的命令。**不要把生产升级误当作全新安装**：保留现有 `.env`、数据库、邮箱数据、Postfix 队列、资料目录、主密钥、镜像及服务授权。不要执行 `down -v` 清空持久化卷，也不要用上游更新脚本替换定制 Core。

升级时应避免新旧邮件调度器同时运行；按审核计划切换 Core，验证迁移和任务租约。单纯启动多个实例不等于高可用。

## 5. 上线检查与健康诊断

只读检查示例：

```sh
docker compose ps
docker compose logs --tail=120 core-billionmail
docker compose logs --tail=80 pgsql-billionmail
docker compose logs --tail=80 postfix-billionmail
docker compose logs --tail=80 dovecot-billionmail
docker compose logs --tail=80 rspamd-billionmail
```

日志可能包含内部邮箱、文件路径或请求信息；提供给外部前先脱敏，不直接放入 GitHub。

上线验收至少确认：

- Core 与前端版本配套，管理登录、语言和静态文件正常。
- 数据库迁移无错误，历史记录与凭据没有被重置。
- 普通成员权限、部门列表、私有下载与邮箱授权正确。
- 用自己的测试邮箱验证提交、最终投递、收件和回复。
- 退订、暂停、取消、未知状态核查和发送窗口有效。
- AI/搜索能力与预算经过真实但最小范围测试。
- 社媒真实操作另行使用获批应用验收；无凭据时不能宣称完成授权/发布。

## 6. 备份策略

备份必须覆盖：PostgreSQL、`.env` 和配置、SSL 私钥/证书、`core-data`（含资料与 `.ai-master-key`）、邮箱数据、相关队列、原应用镜像和版本信息。

**示例仅展示数据库逻辑备份；它不是完整系统备份：**

```sh
umask 077
BACKUP_DIR="backup/manual-$(date +%Y%m%d-%H%M%S)"
mkdir -p "$BACKUP_DIR"
docker compose exec -T pgsql-billionmail sh -c \
  'exec pg_dump -Fc -U "$POSTGRES_USER" "$POSTGRES_DB"' \
  > "$BACKUP_DIR/database.dump"
test -s "$BACKUP_DIR/database.dump"
docker compose images > "$BACKUP_DIR/images.txt"
git rev-parse HEAD > "$BACKUP_DIR/source-version.txt"
```

文件/邮箱/队列的备份需按实例实际持久化路径执行，并考虑正在写入数据的一致性。采用受控维护窗口或一致性快照；不要把运行中目录的随意拷贝当作完整一致备份。

- 加密或严格控制备份访问；私钥与数据库副本不能发布。
- 保留多版本备份并在隔离环境定期恢复演练。
- 记录主密钥版本及配套数据；不要恢复时自动生成新密钥覆盖旧密钥。
- 备份成功至少需检查文件完整性和实际可恢复性，不只看命令退出码。

## 7. 升级与回滚

升级流程：确认目标版本 → 完整备份 → 停止旧 Core 交接新任务 → 更新匹配前后端和私有 SQL → 检查追加迁移 → 验证权限/数据/队列 → 受控试发 → 恢复业务。

回滚先暂停外发任务，记录不确定的提交。恢复上一版本应用与匹配的数据/配置需使用已演练方案；若只回退旧应用而保留新部门账号，旧代码可能缺少新的访问边界，必须先控制相关账号与 API 权限。不可未经批准自动清库或执行破坏式数据库恢复。

停止任务不撤回已提交邮件/帖子，回滚也不会自动撤销外部动作。unknown 记录要单独核查，以防恢复后重复发送。

## 8. 故障处理路径

| 问题 | 检查顺序 | 处理原则 |
|---|---|---|
| 启动失败 | 私有 SQL/配置 → 数据库连接 → 迁移错误 → Docker socket → 日志 | 保存错误证据，不绕过失败迁移启动 |
| 登录/权限问题 | HTTPS/SafePath → 账号状态 → Token → 部门/角色 → LegacyBoundary | 不通过解除全部权限解决 |
| 前端白屏 | 入口与异步资源是否配套 → 浏览器错误 → API 信封 → 缓存/版本 | 保留可复现请求；不当作登录成功 |
| 邮件停滞 | 状态/授权 → 上层额度 → 窗口 → SMTP/队列 → 退信 → 外网限制 | accepted 与 delivered 分开核对 |
| AI/搜索失败 | 协议/模型 → 端点 → 私有密钥 → 额度 → 解析/来源 | 不伪造返回或无限重试 |
| 社媒失败 | 应用审批 → 账号模式 → 授权/Token → 内容审核 → 429/unknown | 遵循平台退避；未知先核查 |
| 恢复后解密失败 | 数据库与原 `.ai-master-key`、资料路径、访问权限 | 不覆盖主密钥，不公开密钥 |
| 磁盘异常 | 数据/日志/队列/上传目录与保留策略 | 先分析占用，不能删除真实邮箱或队列救急 |

## 9. 日常、周期与变更管理

- 每日：服务状态、磁盘、unknown、待审批、队列、退信、预算、过期授权与备份。
- 每周：部门授权与角色、退订/抑制、模型配置、平台指标覆盖和错误趋势。
- 每月/按政策：恢复演练、主密钥配套检查、证书续期、依赖安全更新、平台权限/保留政策复核。
- 每次升级：记录版本、范围、审批、备份、回滚、迁移和受控验收结果。

## 10. 文档再生成与发布

在已安装 Playwright Core、浏览器、前端依赖和已有构建时执行：

```sh
node scripts/docs/generate-diagrams.cjs
node scripts/docs/capture-ui.mjs
node scripts/docs/build-handbook.cjs
python scripts/docs/validate-handbook.py
```

`PLAYWRIGHT_MODULE` 可以指定 Playwright Core 的模块文件；`DOC_BROWSER` 指定本机 Chromium/Edge 可执行文件。默认工具路径是当前文档生成环境的本地路径，新机器应显式设置，不提交依赖目录或浏览器二进制。

截图脚本只允许本地界面资源并拦截虚构 API 返回；文档不调用生产。Markdown 和图源可独立阅读，即使没有 PDF 生成工具也可使用。

发布前运行链接/图片/PDF验证和敏感信息检查；确认图中无真实人员、客户、Token、私钥、邮件正文或财务数据，再提交并推送。
