# 概览统计、模板预览与邮箱授权修复 / Overview, preview and mailbox grants

日期 / Date：2026-11-14。本文记录代码修复，不表示已更新生产服务。

## 1. 已定位的问题

### 概览

- 日志实际按倒序扫描，同一时间戳应保留首先读取的较新行；数据库冲突更新原先使用严格 `>`，跨轮扫描时同秒发生的 `deferred` → `bounced` 更新会被忽略。新增集成测试覆盖倒序和同秒重放，防止状态退回旧值。
- 日志游标在写入完成前推进，数据库写入失败可能造成后续不再处理该段日志。
- 营销链接追踪仅识别双引号小写 `href`，遗漏单引号、大小写和未加引号的链接；HTML 实体应在编码追踪目标前解码，资源链接和退订不能被重写。
- 部分旧/先到达的回执 Message-ID 带尖括号，日志映射不带，导致关联失败。真实回执也不能因为后来修改任务/模板开关而被当作没有可计量样本。
- Docker/Postfix 队列读取失败、返回为空对象或 JSON 无效时，被误当成队列为空。现在采用不可用状态，并保留普通成员看不到全局队列的边界。

### 模板预览

- iframe 的 `height` 属性不支持 `calc()` / `min()`，仅给属性会回退到浏览器默认高度。
- 预览缺少统一文档视口，固定弹窗宽度在窄屏越界；模板 CSS、表格和文本应保持，同时不能执行模板脚本。

### 邮箱授权下拉框

- 列表有缓存后不再刷新，新增邮箱或重新打开编辑仍可能显示旧选项。
- 旧搜索词、分页失败后的页码推进、上下文切换后的迟到响应会造成缺项或混入旧标签。

## 2. 修复策略

- 按日志时间保留最新状态，倒序扫描时相同时间戳保留先读取的行；写入成功后才推进游标。数据库允许同秒的最终状态更新。
- 保持现有队列键统计表结构，不变更数据库 schema，不删除或重发邮件。多收件人队列的历史表设计仍是一条队列记录，不宣称已改成逐收件人数据库模型。
- 仅追踪 HTTP/HTTPS 的锚点链接，支持常用 HTML 属性写法，保留资源/退订/已追踪链接。
- Message-ID 关联兼容尖括号，真实回执作为历史可观察性的证据；缺少测量显示 `—`，真实 0 仍显示 0。
- 读取延迟队列失败时显示 `—` 与检查提示，查询有超时、成功结果短期缓存；不向普通成员暴露全局队列。
- 统计查询在局部事务中关闭 JIT 并恢复调用方设置，避免复杂相关探测触发高开销 JIT 编译；不修改生产 PostgreSQL 的全局配置。
- 模板预览使用 CSS 长度、视口和脚本禁用策略，保留作者样式；支持桌面/移动宽度，弹窗不超出视口，不修改保存内容。
- 每次打开下拉框重新读取第一页，清除旧搜索，保留已选值；分页成功后才推进，并丢弃过期或跨上下文响应。服务端现有启用状态和部门授权过滤保持不变。

## 3. 验证与使用

前端：

```sh
cd core/frontend
npm run test:ui-regressions
npm run test:repair
npm run test:workspace
npm run i18n:check
```

测试包含真实 Vue 组件的非零指标、刷新下拉框、45 条邮箱分页/失败重试、上下文隔离、模板 iframe，以及真实 Edge 浏览器在桌面和窄屏中的预览与脚本阻断。

Go 纯函数测试可单独执行：

```sh
cd core/internal/service/maillog_stat
GO111MODULE=off GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off go test -buildvcs=false -v \
  latest_record.go deferred_queue.go tracked_links.go statistics_helpers_test.go
```

最初验证时本地 Docker 未运行；阿里云部署准备期间已启动本地隔离环境，完成统计、概览控制器、部门权限、邮箱服务与系统设置的 PostgreSQL 回归，以及 Linux 正式入口构建。新增测试也覆盖 JIT 设置不泄漏到调用方事务。前端生产构建通过，保留现有非阻断性 lint 警告。隔离验证仍不等于生产最终投递验收。

部署构建还修正了统计文件的未使用导入，以及邮箱导入预校验对旧默认密码常量的引用。导入与服务层统一使用私有密码策略，不恢复硬编码密码。

## 4. 上线与历史数据注意

发布应使用匹配的后端和前端版本，并先保存备份。此修复不会改动现有邮箱密码、部门授权、数据库结构或生产业务数据。

- 新发邮件的链接追踪修复不会给已经发出的未追踪链接补上历史点击。
- 已丢失的历史退信状态需根据仍保留的日志，在备份后重解析受影响时段；不能清库或通过重发邮件补统计。
- 当前 Postfix 队列是实时值，不是所选历史时间范围的积累值。没有延迟邮件时为 0 是正确结果。
- 没有真实点击、退信或可计量样本时，不生成虚假的非零数据。

## English summary

Confirmed code defects included strict timestamp updates across log scans, premature log cursor advancement, incomplete anchor tracking, legacy Message-ID matching, queue failures reported as zero, iframe CSS-length handling and stale remote options. The log scanner reads backwards; ties must retain the first (newer) line, as covered by disposable-database integration tests.

The fix preserves the existing queue-keyed schema and authorization boundaries. It does not claim per-recipient storage migration, alter production data, reset mailbox credentials, replay external actions or fabricate metrics.

The dropdown reloads on every opening, preserves selection, resets searches, retries pagination correctly and ignores late responses from another context. Preview supports responsive desktop/mobile widths with preserved styles and blocked scripts. Queue unavailability is displayed separately from a measured empty queue.

Frontend/component/browser regressions, disposable PostgreSQL integration suites and the Linux production build passed during deployment preparation. The statistics transaction disables JIT locally and preserves an enclosing transaction's setting; server-global configuration is unchanged. Mailbox import now uses the same private password policy as service writes. Production validation/deployment is not implied by these isolated checks. Historical missing events require controlled log reconciliation; untracked links in already-sent mail cannot retrospectively generate click events.
