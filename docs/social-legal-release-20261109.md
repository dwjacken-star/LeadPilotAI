# BESTMAPLE 公开政策页面发布记录

联系邮箱：autobestmaple@gmail.com

版本：`billionmail/maplemail-ali:20261109-legal-v1`

备份：`/opt/BillionMail/backup/legal-20261109T141245Z`

线上验收时间：2026-11-09T14:20:46Z

- 隐私政策：https://mail.best-maple.com/legal/privacy
- 服务条款：https://mail.best-maple.com/legal/terms
- 数据删除说明：https://mail.best-maple.com/legal/data-deletion

三个页面中英双语，适配手机与桌面，无需登录。匿名、Meta/LinkedIn 爬虫请求、HEAD、静态资源、CSP、删除说明 POST 405、robots 检查通过。Meta 后台使用“数据删除说明网址”选项；删除申请通过邮箱人工受理，需持续关注该邮箱。审核结果由平台决定。

数据库记录基线、账号/邮箱/SMTP/AI/Serper/发信身份配置、其他容器 ID 检查通过。原有后台、邮箱 HTTPS 与邮件 TLS 服务正常。证书定时器 active。社媒接口仍关闭，等待平台审批及配置。

后端公开页面及入口包测试、Linux 静态构建、前端构建、目标页面 ESLint、多语言一致性检查通过。在线手机端资源均返回 200，语言切换及邮箱链接正确，无控制台错误。完整服务器验收记录：`.tools/legal-acceptance.json`。
