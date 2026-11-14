# 47.237.67.184 部署记录

安装目录：`/opt/BillionMail`。系统为 Alibaba Cloud Linux 4、x86_64，约3.5GB内存、40GB磁盘；保留原宝塔面板。Docker使用系统软件仓库安装，版本29.5.1，Compose 2.26.1。

定制镜像：`billionmail/department-workspace:20261104`。实际部署包含7个服务：Core、PostgreSQL、Redis、Postfix、Dovecot、Rspamd、Roundcube。数据库和Redis的宿主机映射只绑定127.0.0.1。

## 管理与更新

```sh
cd /opt/BillionMail
./deploy.sh ps
./deploy.sh logs --tail 60 core-billionmail
./deploy.sh restart core-billionmail
```

本次采用本地编译的Linux二进制和前端构建产物构建轻量运行镜像，避免在小内存服务器上编译。运行镜像Dockerfile为 `Dockerfiles/core/Dockerfile.deployed`，编排覆盖文件为 `docker-compose.deploy.yml`。

更新前需要同步源码、重新编译二进制并更新 `core/billionmail-department`，更新前端产物至 `core/public/dist`，随后执行：

```sh
./deploy.sh build core-billionmail
./deploy.sh up -d --no-deps core-billionmail
```

也可以采用仓库中的多阶段 `Dockerfile.enterprise` 从完整源码重新构建。不要用上游 `bm update` 替换定制版本。

管理员使用独立生成的密码；登录入口和初始凭据保存在服务器 `/root/billionmail-panel-access.json`，权限600。SSH密码不写入本记录或源码。

## 备份与验证

更新前的数据库、配置、环境文件和原二进制保存在 `backup/deploy-*`，备份目录权限700。现有数据卷持续保留。

已验证公网HTTPS页面、管理员登录、部门工作台接口、匿名访问拦截、两个临时部门账号的登录与隔离、跨部门修改/删除及旧API绕过拦截。临时账号、角色、部门和分组测试后清理，保留操作审计。

2026-11-04补充修复：工作台列表和远程选择器兼容空数组，旧策略/周期计划/自动化配置的空值规范化；顶栏移除提交需求按钮，版本号保留为普通文本。更新时先执行只读pg_dump并验证备份目录，再只重建Core；保留原数据库容器、数据卷和旧哈希静态资源。

管理入口现使用Let's Encrypt可信IP证书。证书目录`/etc/letsencrypt/live/billionmail-console-ip/`，应用读取`/opt/BillionMail/ssl/cert.pem`和`key.pem`。该证书有效期约6天，`billionmail-ip-cert.timer`每天两次执行`/usr/local/sbin/billionmail-renew-ip-cert`；在Core网络命名空间内使用60880端口响应现有ACME反代，续期不占用宿主机80端口。仅在证书更新后重启Core加载，现有数据库不重启。续期检查命令：`systemctl status billionmail-ip-cert.timer`、`journalctl -u billionmail-ip-cert.service`。

最终更新备份：`backup/ui-final-20261104T060046Z/database.dump`。数据库容器和持久化目录未重建或删除。浏览器验证包括工作台空列表、任务/计划/自动化/成员/角色/策略新增弹窗、部门编辑保存、临时联系人组新增和修改保存，以及发件API列表和新建弹窗；临时联系人组测试后清理。补充修复了Naive UI传送弹窗丢失scoped样式的问题，限制宽度和高度并允许滚动。前端空值回归测试：`cd core/frontend && npm run test:workspace`。续期脚本和systemd模板位于`deploy/`。

首次运行发现并修复：SMTP中继迁移的标量Scan及旧表重命名条件问题，以及DDL迁移后ORM字段缓存未刷新的问题。后者增加了预热旧表元数据的回归测试。

邮箱授权后续修复：发件邮箱列表先使用过滤条件执行`COUNT(*)`，再应用`username,domain,active`字段和分页，避免PostgreSQL不支持的多参数COUNT。增加按用户名排序，保证分页稳定；部门授权和邮箱启用条件保持在计数与查询共用的过滤器上。`TestDepartmentBusinessIsolation/SenderOptionsCountAndScope`覆盖管理员分页、搜索、已选邮箱回填、禁用邮箱、失效授权及部门隔离。完整回归通过。部署备份位于`backup/sender-fix-20261104T064737Z/database.dump`，只重建Core并核对数据库容器ID保持不变。浏览器已验证授权邮箱下拉正常展示现有5个邮箱。

## 邮件功能待配置项

- 首次部署时曾使用临时主机名 `mail.invalid`。邮箱授权修复检查时已存在5个邮箱，更新保留现有域名、邮箱和其他业务数据，不重复初始化。
- IP管理入口已配置可信证书及自动续期；邮件服务域名和相应证书仍需随实际发信域名配置。
- 从服务器到Gmail和QQ的IPv4 TCP 25连接均超时。正式直投前需要核实阿里云出站25政策并申请解封，或配置获准使用的SMTP中继。
- SPF、DKIM、DMARC、MX、PTR和真实公网收件测试尚未完成，不能将当前后台部署成功视为真实邮件投递验收完成。
