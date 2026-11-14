'use strict'
const fs = require('node:fs')
const path = require('node:path')
const out = path.resolve('docs/handbook/assets/diagrams')
fs.mkdirSync(out, { recursive: true })
const esc = s => String(s).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
function svg(title, boxes, lines, width=1280, height=720) {
 const parts=[`<svg xmlns="http://www.w3.org/2000/svg" width="${width}" height="${height}" viewBox="0 0 ${width} ${height}" role="img" aria-label="${esc(title)}"><defs><marker id="arrow" markerWidth="8" markerHeight="8" refX="7" refY="4" orient="auto"><path d="M0,0 L8,4 L0,8" fill="#5d7591"/></marker></defs><rect width="100%" height="100%" rx="16" fill="#f4f7fb"/><style>text{font-family:Arial,"Microsoft YaHei","Noto Sans CJK SC",sans-serif} .title{font-size:28px;font-weight:700;fill:#152b45}.label{font-size:21px;font-weight:700;fill:#14314c}.detail{font-size:17px;fill:#4b637a}</style><text x="32" y="48" class="title">${esc(title)}</text>`]
 for(const l of lines)parts.push(`<path d="${l}" fill="none" stroke="#5d7591" stroke-width="2.5" marker-end="url(#arrow)"/>`)
 for(const b of boxes){const [x,y,w,h,label,details,color]=b;parts.push(`<rect x="${x}" y="${y}" width="${w}" height="${h}" rx="12" fill="${color||'#ffffff'}" stroke="#cedbe8" stroke-width="1.5"/><text x="${x+18}" y="${y+33}" class="label">${esc(label)}</text>`);details.forEach((d,i)=>parts.push(`<text x="${x+18}" y="${y+63+i*26}" class="detail">${esc(d)}</text>`))}
 parts.push('</svg>');return parts.join('\n')
}
for(const locale of ['zh','en']) {
 const zh=locale==='zh',t=(a,b)=>zh?a:b
 const boxes=[
  [30,100,275,120,t('用户界面','User interfaces'),['Vue 3 / TypeScript / Pinia',t('管理台 · 多语言工作区','Console · Localized workbench')]],
  [30,260,275,115,t('Webmail 与邮箱客户端','Webmail / mail clients'),['Roundcube · IMAP / SMTP',t('独立邮箱认证','Separate mailbox authentication')]],
  [355,100,540,145,t('Core：Go / GoFrame 模块化单体','Core: Go / GoFrame modular monolith'),[t('API · JWT/RBAC · 部门与资源边界','APIs · JWT/RBAC · scope and grants'),t('邮件 · CRM · 资料 · AI · 社媒 · 情报','Mail · CRM · materials · AI · social · intelligence'),t('定时器 · 租约 · 预算 · 检查点','Timers · leases · budgets · checkpoints')],'#e6f4f4'],
  [965,100,285,120,'PostgreSQL 17.4',[t('业务数据 · 队列 · 额度','Records · queues · quotas'),t('快照 · 审计 · 租约','Snapshots · audit · leases')]],
  [965,260,285,105,'Redis 7.4.2',[t('缓存与现有邮件组件状态','Cache and mail-component state')]],
  [355,290,540,140,t('外部服务（需配置与授权）','External services (configured/authorized)'),['AI providers · Serper search/scrape','Facebook Pages · LinkedIn organizations',t('可选 WhatsApp · 不默认开通','Optional WhatsApp · not enabled by default')]],
  [30,475,390,150,t('邮件协议服务','Mail protocol services'),['Postfix · Dovecot · Rspamd',t('SMTP 交接、队列、收件与反垃圾','SMTP handoff, queue, inbox, spam checks'),t('accepted ≠ 最终收到','Accepted ≠ final receipt')]],
  [465,475,435,150,t('持久化与私有配置','Persistence / private configuration'),['core-data · vmail-data · queue data',t('.env · SSL 私钥 · .ai-master-key','.env · SSL private keys · .ai-master-key'),t('数据库与主密钥必须配套恢复','Restore database and key together')]],
  [945,475,305,150,t('运维与信任边界','Operations / trust boundary'),['Docker Compose · Fail2ban',t('Core 的 Docker socket 为高权限','Privileged Docker socket in Core'),t('SQL 通过私有渠道配套','Private SQL dependencies')]],
 ]
 const lines=['M305 160 L355 160','M895 155 L965 155','M895 205 L930 205 L930 310 L965 310','M625 245 L625 290','M30 315 L15 315 L15 550 L30 550','M465 550 L420 550','M680 475 L680 430','M1040 475 L1040 390 L850 390']
 fs.writeFileSync(path.join(out,`technical-${locale}.svg`),svg(t('技术架构 · LeadPilot AI','Technical Architecture · LeadPilot AI'),boxes,lines))
 const cards=[
  [30,100,285,140,t('治理与组织','Governance'),[t('部门 · 角色 · 数据范围','Departments · roles · scope'),t('邮箱授权 · 共享发送策略','Sender grants · shared policies'),t('审计 · 实时权限复核','Audit · live permission checks')]],
  [340,100,285,140,t('可信业务资料','Approved business materials'),[t('公司 · 产品/服务 · 文件','Company · offerings · files'),t('导入 · 识别 · 人工复核','Import · extraction · human review'),t('发布 · 私有原件 · 版本','Publication · private originals · versions')]],
  [650,100,285,140,t('公开证据与线索','Public evidence / leads'),[t('搜索 · 采集 · 来源证据','Search · scrape · source evidence'),t('邮箱验证 · 过滤 · 评分','Verification · filters · scoring'),t('未知结果保留复核','Uncertain results remain reviewable')]],
  [960,100,285,140,t('AI 注册中心','AI model registry'),[t('服务商 · 协议 · 能力','Providers · protocols · capabilities'),t('加密凭据 · 预算 · 用量','Encrypted secrets · budgets · usage'),t('实际能力需独立验证','Actual capability requires testing')]],
  [30,300,390,145,t('邮件营销与 AI 拓客','Email marketing / prospecting'),[t('联系人 · 模板 · 草稿 · 计划','Contacts · templates · drafts · plans'),t('冻结内容 · 审批 · 退订/抑制','Snapshots · review · suppression'),t('受控 SMTP 交接与统计','Governed SMTP handoff and metrics')]],
  [445,300,390,145,t('Growth Master 智能情报','Growth Master intelligence'),[t('行业包 · 14 技能 · 6 工作流','Industry packs · 14 skills · 6 workflows'),t('证据 · 候选 · 内容 · 检查点','Evidence · prospects · content · checkpoints'),t('人工门槛 · 预算 · 审计','Human gates · budgets · audit')]],
  [860,300,385,145,t('社媒营销','Social marketing'),[t('官方授权 · 多账号/多版本','Official grants · accounts/variants'),t('审核 · API/人工模式 · 排期','Review · API/manual mode · schedule'),t('回查 · 互动 · 官方统计','Reconciliation · interactions · metrics')]],
  [250,510,765,145,t('客户管理与持续改进','CRM and continuous improvement'),[t('询盘分类 · 邮件会话 · AI 草稿 · 人工/受控回复','Inquiry triage · threads · AI drafts · reviewed replies'),t('客户阶段 · 跟进 · 效果分析 · 成本与审计','Customer stages · follow-up · effectiveness · cost/audit'),t('不可把未知数据或演示截图解释为业务实绩','Unknown metrics and demo captures are not business results')]],
 ]
 const arrows=['M485 240 L485 275 L225 275 L225 300','M790 240 L790 275 L640 275 L640 300','M1100 240 L1100 300','M225 445 L225 560 L250 560','M640 445 L640 510','M1050 445 L1050 560 L1015 560']
 fs.writeFileSync(path.join(out,`functional-${locale}.svg`),svg(t('功能架构 · 发现—审核—触达—跟进','Functional Architecture · Discover—Review—Engage—Follow up'),cards,arrows))
 const steps=[
  [35,110,350,115,t('1. 已批准资料与配置','1. Approved sources/configuration'),[t('公司/产品版本 · 目标 · 授权','Company/product versions · targets · grants')]],
  [465,110,350,115,t('2. 发现、验证与评分','2. Discover / verify / score'),[t('真实公开证据 · 不猜邮箱','Public evidence · no guessed addresses')]],
  [895,110,350,115,t('3. 内容与人工门槛','3. Content / human gates'),[t('具体内容、目标和版本核查','Review exact content, target and version')]],
  [895,340,350,130,t('4. 受控队列与外部交接','4. Governed queue / handoff'),[t('额度 · 时段 · 退订 · 权限复核','Quotas · windows · suppression · rights'),t('SMTP / 社媒 / 可选渠道','SMTP / social / optional channels')]],
  [465,340,350,130,t('5. 回执与不确定结果','5. Receipts / ambiguous outcomes'),[t('accepted、delivered、unknown 分开','Separate accepted, delivered, unknown'),t('未知先核查，禁止盲目重试','Reconcile unknown; no blind retry')]],
  [35,340,350,130,t('6. CRM、跟进与审计','6. CRM / follow-up / audit'),[t('询盘 · 会话 · 效果 · 成本','Inquiries · threads · metrics · cost'),t('历史快照与来源保留','Preserve source and history snapshots')]],
  [160,555,950,90,t('持久化保护贯穿整个过程','Persistent controls across the lifecycle'),[t('PostgreSQL 事务 · 幂等键 · 租约 · 检查点 · 版本 · 审计','PostgreSQL transactions · idempotency keys · leases · checkpoints · versions · audit')],'#e6f4f4'],
 ]
 const links=['M385 167 L465 167','M815 167 L895 167','M1070 225 L1070 340','M895 405 L815 405','M465 405 L385 405']
 fs.writeFileSync(path.join(out,`workflow-${locale}.svg`),svg(t('业务执行与可靠性交接','Execution Lifecycle and Reliable Handoff'),steps,links))
}
fs.writeFileSync(path.join(out,'technical.mmd'),`flowchart TB
  UI[Vue management console] --> CORE[Go / GoFrame modular monolith]
  CORE --> DB[(PostgreSQL: records, durable work, quotas, audit)]
  CORE --> REDIS[(Redis: cache and mail-component state)]
  CORE --> EXTERNAL[AI providers / Serper / Social / optional WhatsApp]
  CORE --> MAIL[Postfix / Dovecot / Rspamd]
  WEBMAIL[Roundcube / mail clients] --> MAIL
  CORE --> FILES[Private files / environment / encryption master key]
  CORE --> DOCKER[Privileged local Docker API]
`)
fs.writeFileSync(path.join(out,'functional.mmd'),`flowchart LR
  GOV[Departments, roles, resource grants] --> MATERIALS[Reviewed materials and versions]
  MATERIALS --> DISCOVERY[Public evidence and verified contacts]
  MODELS[AI registry and budgets] --> DISCOVERY
  DISCOVERY --> REVIEW[Content and human review]
  REVIEW --> EMAIL[Governed email]
  REVIEW --> SOCIAL[Authorized social / manual publishing]
  REVIEW --> MASTER[Growth Master workflows]
  EMAIL --> CRM[Inquiries, CRM and follow-up]
  SOCIAL --> METRICS[Official metrics and coverage]
  MASTER --> CRM
  CRM --> AUDIT[Analytics, costs and audit]
  METRICS --> AUDIT
`)
console.log('Generated six SVG architecture/lifecycle diagrams and two Mermaid sources.')
