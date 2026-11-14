# 模板预览与邮箱授权界面调整 / Preview theme and mailbox binding

## 修复范围

- 模板预览的桌面/移动标签改用实际存在的 `campaignComposer.desktop/mobile` 翻译键，并使用 Naive UI 的显式 label 属性；六种语言均验证非空。
- 预览外框、iframe 默认背景和普通文本跟随系统明暗主题。作者在邮件模板中指定的背景和文字颜色仍然保留，不反色、不修改保存内容。
- 部门邮箱授权编辑器使用管理员专用库存视图，最新创建的启用邮箱优先显示；显示已加载数量/总数，并支持显式加载更多。普通成员的发件箱列表仍按已有部门授权过滤。
- 邮箱业务权限编辑器显示账号所属部门、账号/部门可用状态和已有发信授权，支持刷新已选账号的实时状态。
- 未获部门发信授权时显示明确提示，保存按钮要求管理员勾选“同时授予所在部门使用此邮箱的发信权限”。这是整个部门的授权，不是静默扩权。
- 后端仅允许系统管理员执行此联动操作；部门授权、账号关联和审计在同一事务内保存。请求必须携带已确认的部门 ID，账号调部门后要求重新确认。
- 不解除原有 `SenderAllowed` 校验；撤销授权、停用账号或部门后，业务请求仍会被拒绝。解除账号关联不会自动删除部门共享发信授权。

## 订阅与退订页面

- 语言选择器移到页面内品牌栏，电脑端右侧、手机端独立一行，不再成为 body 居中 flex 布局的第二个并排元素。
- 英文品牌统一为 LeadPilot AI，中文为领客AI；副标题使用 LeadPilot Smart Mail / 领客智邮。已有旧页面文字保留兼容翻译别名，真实域名、邮箱和自定义分组名称不变。
- 旧订阅模板移除外部 Tailwind/图标 CDN 依赖，保留 CSRF token、group_ids 和原提交方式。
- 两个仅含公开文字/布局的脚本可匿名加载，不开放管理 API 或整个静态资源目录。
- 浏览器验证九类公开页面、中英文、1440/390/320 三种宽度共 54 个视图；无横向溢出，语言栏位于卡片内，切换语言不改变表单 token。测试不执行真实订阅/退订。

## 数据边界

部署不根据截图自动修改真实账号关联或部门授权，不重置邮箱密码，不批量授权给所有部门。需要授权时，由管理员在界面明确勾选后保存。

## 验证

- 真实 Naive UI Radio 的六语言名称渲染。
- 主题切换、普通文本和作者邮件样式保留。
- 管理员库存最新邮箱排序与普通成员隔离。
- 默认不授权、显式授权、重复保存、过期部门确认、审计失败回滚、撤权后阻断及停用关联清理。
- 线上验证只读取接口、查看弹窗和勾选测试选项，不点击真实授权保存；截图和会话凭据保存在本地私有工具目录，不提交。

## English summary

Preview labels now use existing translations and explicit radio labels. UI/default preview colors follow the application theme while preserving authored email colors. The administrator grant inventory shows newest active mailboxes first, with counts and explicit pagination.

Mailbox business access exposes account department, eligibility and sender authorization. An explicit administrator checkbox can create/reactivate that department's sender grant atomically with the binding and audit. Confirmed department IDs prevent stale-consent grants after an account moves. Existing role/resource checks remain enforced; deployment does not silently modify live permissions or associations.
