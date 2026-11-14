# 华为云部署与旧数据迁移记录

2026-11-04，原服务器 `47.237.67.184` 的项目和业务数据已迁移到 `124.243.169.53`。

当前入口：`https://autorets.best-maple.com/bm`，邮箱入口：`https://mail.best-maple.com/roundcube/`。后台账号及密码沿用原系统，没有重置账号或邮箱密码。服务器已有的宝塔面板保留。后续域名配置及核验见 [域名配置](domain-configuration.md)。

## 运行环境

安装目录 `/opt/BillionMail`，CentOS Linux 8 x86_64，约3.6GB内存、40GB磁盘。Docker 26.1.3、Compose 2.27.0。运行 Core、PostgreSQL、Redis、Postfix、Dovecot、Rspamd、Roundcube 七个服务，数据库与Redis宿主机端口绑定127.0.0.1。

定制镜像 `billionmail/department-workspace:20261104`。迁移时版本包含工作台空值、弹窗样式及邮箱授权COUNT查询修复；随后已部署 BESTMAPLE Mail 登录、实际账号密码更新、默认邮箱密码和跟踪统计修复，最新后端 SHA256 为 `57373d0951af35c5b7f148b2211c94aac4a2deb5c3a1eab35a3579152e0aa00a`，详细范围与验证见 [登录与统计更新](bestmaple-login-and-tracking.md)。

```sh
cd /opt/BillionMail
./deploy.sh ps
./deploy.sh logs --tail 60 core-billionmail
```

## 迁移与验证

在旧机停止应用、发信及邮件写入后，保存Redis快照，执行PostgreSQL逻辑备份，并同步 `.env`、配置、证书、应用持久化目录、邮箱邮件、队列、Webmail、Rspamd与日志。源机原目录和数据卷继续保留。

旧机一致性备份：`/opt/BillionMail/backup/huawei-migration-20261104T082211Z/`。

新机导入备份：`/opt/BillionMail/backup/source-import-20261104T083021Z/`。

迁移包SHA256：`ed589708715e633d975c6269dbc11217c3099b9388e89488fc319fa1deac2d68`。导入前校验包内文件，启动应用前比对77张公共表的行数及逐行内容聚合摘要，结果一致。迁移时共有1个后台账号、1个部门、6个邮箱，未创建新的权限或营销任务。旧机队列为空。

已验证原管理员账号在新入口登录、工作台加载、邮箱授权下拉显示6个邮箱、发件API页面、可信HTTPS、邮件端口连接，以及旧机全部邮件端口转发后的服务响应。新机到Gmail、QQ的TCP 25连接可建立；本次未发送实际测试邮件。

## HTTPS与续期

新机使用Let's Encrypt可信证书，临时同时覆盖新旧两个IP。`billionmail-ip-cert.timer`每天两次运行容器版Certbot 5.4.0，通过Core网络内的60880端口响应HTTP-01验证。仅证书变化后重启Core加载。

安装脚本 `/usr/local/sbin/billionmail-renew-ip-cert` 对应仓库 `deploy/renew-console-ip-cert-container.sh`；支持证书续期保留可验证的IP，旧IP退役后不会强制继续保留旧标识。

```sh
systemctl list-timers billionmail-ip-cert.timer
journalctl -u billionmail-ip-cert.service
```

## 域名切换和旧机过渡

迁移检查时，阿里云DNS中的 `mail.best-maple.com` A记录仍指向旧IP，域名SPF也仍授权旧IP。迁移时要求的 DNS 变更如下；后续域名配置时已确认 `mail.best-maple.com` 和 `autorets.best-maple.com` A 记录均指向新 IP，SPF 不属于本次域名配置核验范围：

| 记录 | 修改 |
| --- | --- |
| `mail.best-maple.com` A | `124.243.169.53` |
| `best-maple.com` 现有SPF TXT | 将 `ip4:47.237.67.184` 替换为 `ip4:124.243.169.53` |

旧机当前只提供过渡转发，80、443和邮件端口均转向新机。因为旧机直连出站TCP 25不可用，入站SMTP通过仅允许本机SMTP目标的受限SSH隧道转发；临时迁移上传权限已移除。SSH密钥不允许执行远程命令。

旧应用七个容器已经停用，并关闭容器自动重启，避免两个独立数据库继续处理任务。旧机原证书续期定时器停用。过渡服务为 `billionmail-transition@80`、`@443`、`@465`、`@587`、`@110`、`@143`、`@993`、`@995`，以及 `billionmail-transition-smtp`、`billionmail-transition-tunnel`。

DNS切换和缓存过期前，保留旧服务器及过渡服务运行。后续退役旧机时保留迁移备份。若需要回退，应先备份并处理新机迁移后产生的数据，再切换处理入口，不能直接同时启动两份旧、新应用。
