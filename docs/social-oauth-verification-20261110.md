# Facebook / LinkedIn 实际授权核验

发布版本：`billionmail/maplemail-ali:20261110-facebook-oauth-v3`

备份：`/opt/BillionMail/backup/facebook-oauth-20261109T154033Z`

共同回调地址：`https://mail.best-maple.com/social/callback`

## Facebook

- 真实 App ID：1051949041240596；应用名 bestmaple。通过 Meta Application API 验证密钥与应用匹配，应用域名已登记 best-maple.com。
- 原始域名错误的实际配置问题：企业版 Facebook 登录的有效 OAuth 跳转 URI 列表为空。补齐完整地址、保存、重新载入后，Meta URI 验证工具返回有效。
- 保持 OAuth 网页登录及严格模式。Meta 数据删除配置使用说明网址，已补齐并重新载入确认 `https://mail.best-maple.com/legal/data-deletion`。
- 后端增加脱敏的供应商错误码及 OAuth 阶段日志；兼容新版 PROFILE_PLUS_FULL_CONTROL / PROFILE_PLUS_CREATE_CONTENT 主页任务，保留发布权限限制。
- 实测 Meta `/me/accounts` 返回空列表。因此增加受限补充发现：从 Meta 验证的令牌读取明确授权的 granular_scopes.target_ids，校验应用和用户身份，仅查询有内容发布授权的主页，取得并再次验证 PAGE 令牌，再加密保存；不搜索任意主页，不把分析/广告权限视为发布权限。
- 真实授权成功，保存主页 Hangzhou Bestmaple Auto Parts Co., Ltd.，状态 connected。
- 实际授予发布、图片、统计、评论、反应及事件通知权限。使用数据库中的已保存令牌，只读查询主页资料和 published_posts，均返回 HTTP 200。未创建公开帖子。

## LinkedIn

- 真实 Client ID：867g1h0uhphy3e；开发者应用 264244318。
- 原先 Authorized redirect URLs 为空。已添加共同回调地址，保存并重新载入确认；原 redirect_uri 不匹配错误已解决。
- 真实授权后返回 unauthorized_scope_error，说明 w_organization_social 未向本应用开放。
- 开发者后台已添加 Share on LinkedIn、OpenID Connect、Events Management API；Community Management API 仍在 Available products，尚未开通。现有个人发布权限 w_member_social 不能替代企业主页权限。
- 下一步由应用管理员在 Products 申请 Community Management API，获批后核对 Auth 页的实际组织权限，再验证主页授权。没有将个人账号授权冒充企业主页授权。

## 验证及数据保护

- 新旧主页任务判断、令牌身份/失效/跨主页权限隔离、错误信息中凭证脱敏测试通过；控制器测试及 Linux 静态构建通过。
- 线上 20 项回归检查通过：发布文件哈希、原有业务数据基线、账号/邮箱/SMTP/AI/Serper/发信身份、共享服务容器、公开政策页、后台和邮箱 HTTPS、邮件 TLS、CRM/拓客接口等。
- 只更新应用容器；数据库及邮件卷保留。真实授权新增 1 个 Facebook 主页连接；没有创建营销计划或公开发布测试内容。
- 线上证据位于备份目录内 acceptance.json 和 real-authorization.json；内容不含令牌或密钥。临时 QA 凭证文件在验收后清理。

官方参考：

## 2026-11-13：Meta 一期受限接入（代码准备，未声称线上发布验收）

- 新平台默认关闭。Instagram 只提供管理员明确选择的 Unified Login（Meta App ID/密钥、已核实的 `config_id`、业务资产及 `instagram_basic` / `instagram_content_publish`）；不会把旧 Facebook 登录或 Instagram Login 令牌混用。Threads 使用独立 App ID/密钥及 `threads_basic` / `threads_content_publish`。
- 应用角色测试资格确认是管理员声明，不代表后端验证了 Meta Live 模式、Advanced Access 或 App Review；外部客户可用性始终标注未核实。Unified 配置如不能提供最小授权，应保持关闭，不使用默认配置顺带启用私信或广告。
- 本次有限发布闭环为 Instagram 图片/轮播、Threads 文本/图片/轮播，每帖最多 10 张；IG 不支持纯文本，图片需 JPEG、8 MB 以内、320–1440 像素宽及 4:5–1.91:1。Threads 支持 JPEG/PNG。已有 Facebook/LinkedIn 流程保持不变。
- 已知 `ai_result` 图片的 AI 来源属性冻结在审核媒体清单中；IG 使用 `is_ai_generated`，Threads 使用 `is_made_with_ai`，仅在父容器披露，不对轮播子项设置该参数。
- 已实现默认关闭的 IG Reels / Threads 视频容器、Facebook 普通视频独立 resumable 上传与 `graph-video.facebook.com` 发布、Facebook Reels start / rupload / status / finish 阶段。所有远端变更先持久化 pending fence；保存容器、视频 ID、上传 session、offset 与 handle，收据丢失不盲重试。仅已知上传 ID 且平台 GET 确认 offset 前进时恢复余下字节；已知 Reels ID 的不明 finish 仅读取状态核对。创建或普通视频发布 ID 丢失仍为 unknown，须人工核实。
- 视频由超时 20 秒、输出 8 MB 上限的真实 ffprobe 检查，仅读本地文件、禁用网络输入。有限兼容范围为 MP4 ≤128 MB、moov 在 mdat 前且无 edit list、H.264/yuv420p、无 B 帧、固定 24–60 FPS、闭合 GOP ≤5 秒及视频码率 ≤25 Mbps；不自动转码。IG 3–900 秒，Threads 3–300 秒，Facebook 普通视频本阶段 ≤600 秒，Reels 3–90 秒且 ≥540×960、9:16。可无音轨；有音轨需 AAC 48kHz、1–2 声道、≤128kbps，Facebook Reels 为双声道 128kbps。平台配置、应用角色测试确认、发布 scopes、签名媒体就绪、ffprobe 或账户权限缺失均阻止视频发布；工具存在不会自动开启。
- Facebook 视频需启用准备后重新授权，方将 User 上传令牌按原凭据机制加密保存；现有 Page token 不能替代 User token。每次视频变更先核对 User token 的当前 app/user/scopes 和 Page CREATE_CONTENT task。Page task 查不到仍为 unverified，非 denied。Reels 公开受众需明确同意并随审核快照冻结；已知 AI 视频对 IG/Threads/Reels 使用官方披露字段，普通 Facebook 视频 AI 披露尚未验证时明确阻止。
- 私有媒体仅通过绑定到投递、图片序号、内容哈希和有效期的签名 HTTPS GET/HEAD 提供，支持 Range，不开放素材目录。签名链接两小时有效；素材、账号权限或任务状态变化后拒绝抓取。
- 共同路由集成：应用主路由须在 JWT 组之外仅绑定 `GET,HEAD /api/social/media` 到 `social.MetaMediaFetch`；保留其他社交接口原有权限。先验证该端点，再保存“已验证媒体端点”；更换地址会撤销该确认。
- 全局安全入口 / safe-path 检查也只豁免上述准确方法与路径；不可开放 `/api/social/*` 或 `/social/*`。OAuth 沿用现有 `/social/callback`，无需新增回调。`meta_readiness` 权限在 social controller/service 内执行，使用现有 `social:settings` 及管理员检查；enterprise.LegacyBoundary 已将准确 `/api/social/action` 交给服务层，无额外共享 action-map 变更、无新增可分配权限。签名视频同一 GET/HEAD 端点流式返回并支持 Range；无效、过期、篡改或权限失效时为 404，无 JSON 凭据或媒体内容。
- 管理员：`POST /api/social/action {"action":"meta_readiness"}` 返回 external_access/dashboard_verification、signed_media_configured/confirmed、video_inspector_available、每平台 authorization/publishing/video blockers 与不含凭据的 account readiness。普通有社交访问权限用户：同一路径 `{"action":"availability"}` 只返回每平台 enabled/publishing/videos/reels、blockers/video_blockers 及分析启用状态，不返回配置或账户凭据。视频上传仍为 JWT 下 `POST /api/social/upload`，purpose=post_video、variant_id、file、ai_generated；save_variant 保存 video/media_type/public_reels_confirmed 后才可审核与排期。delivery_control 的 reconcile 操作只允许已知可核对上传，不能清除不明发布 fence。
- 可选实际工具契约测试名：`TestMetaVideoInspectorContract`，环境 `BM_TEST_META_VIDEO_FILE` 指向现有合成文件，未设置则跳过。合成 H.264/yuv420p 720×1280、24fps、4 秒、-bf 0、闭合 GOP ≤5 秒、正码率、fast-start MP4、`-use_editlist 0`，可无音轨。测试真实调用本地 ffprobe，再验证四种发布限制；不发网络请求、不生成生产内容。
- `bm_social_provider_operations` 持久化子容器、父容器、已发布 ID、处理次数及提交围栏。可确认的处理状态可恢复；未确认的远端写入结果进入 unknown，不重复提交。
- `core/scripts/meta-capability-probe.go` 是只读诊断，不调用数据库初始化、迁移或会创建密钥的 Protect。复制后的独立可执行文件应传 `--root` 指向实际 core 运行目录；可用 `--env-file`、`--db-host`、`--key-file` 覆盖现有文件/套接字位置。仅输出脱敏阶段错误、应用匹配、实际授权和资产匹配；Page token 无法读取任务列表时只标记未核实。
- 如果容器没有 PostgreSQL 套接字或 `.env` 挂载，可使用 `--stdin --key-file <现有主密钥文件>`，从安全管道输入 `{"config":<social_config JSON 对象>,"accounts":[{"id":1,"external_id":"…","token":"<数据库原始密文>","target_type":"page"}]}`。该模式不读取数据库或环境文件；不要把 JSON、密文或密钥放入命令行参数、日志或公开输出。
- 官方依据：https://developers.facebook.com/documentation/instagram-platform/instagram-api-with-unified-login/ 、https://developers.facebook.com/documentation/instagram-platform/content-publishing/ 、https://developers.facebook.com/documentation/threads/posts/ 、https://developers.facebook.com/documentation/video-api/guides/reels-publishing/ 。

- https://developers.facebook.com/documentation/facebook-login/security
- https://developers.facebook.com/docs/graph-api/reference/page/assigned_users/
- https://developers.facebook.com/docs/facebook-login/guides/%20access-tokens/debugging/
- https://developers.facebook.com/docs/graph-api/reference/page/
- https://learn.microsoft.com/en-us/linkedin/shared/authentication/authorization-code-flow
- https://learn.microsoft.com/en-us/linkedin/marketing/increasing-access?view=li-lms-2026-10
