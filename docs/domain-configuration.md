# 控制台及邮箱域名配置

2026-11-04 已更新华为云服务器 `124.243.169.53` 的运行配置。

| 项目 | 当前值 |
| --- | --- |
| 控制台域名 | `autorets.best-maple.com` |
| 管理入口 | `https://autorets.best-maple.com/bm` |
| 邮箱反向代理/公开地址 | `https://mail.best-maple.com` |
| 邮箱登录入口 | `https://mail.best-maple.com/roundcube/` |

保留用户已设置的 `SafePath=bm`，没有恢复早期部署时的随机入口。

运行 `.env` 中 `BILLIONMAIL_HOSTNAME=autorets.best-maple.com`、`PUBLIC_BASE_URL=https://mail.best-maple.com`；数据库 `bm_options.reverse_proxy_domain=https://mail.best-maple.com`。Webmail 容器的 `BESTMAPLE_TRACKING_BASE_URL` 同步为邮箱域名，新发送邮件的跟踪地址不再使用 IP。

两个域名 A 记录均已解析到 `124.243.169.53`。默认浏览器证书扩展为同时覆盖两个域名、新服务器 IP 和过渡旧 IP；邮箱域名原有 SNI 证书保留。现有证书续期 timer 继续运行，续期脚本增加配置域名匹配校验，避免可选标识被移除时安装不匹配的证书。

配置更新前备份位于 `/opt/BillionMail/backup/domain-update-20261104T104832Z/`，包含数据库逻辑备份、配置、证书及续期脚本。仅重新创建 Core、Webmail 应用容器，原 PostgreSQL 容器、数据目录和全部已有凭据保留。更新前后逐一比对账号及邮箱凭据指纹一致。

已通过可信 TLS 校验、控制台安全入口和 6 个首页脚本/样式请求、无管理 Cookie 的邮箱页面及资源、公开跟踪路由检查。未发送外部测试邮件。旧邮件正文中的既有地址不会因本次配置更新被改写，原 IP 入口继续保留。
