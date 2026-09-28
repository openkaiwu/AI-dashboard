# 运行与跨网络部署

## 当前本地电脑

在项目根目录运行：

~~~powershell
powershell -ExecutionPolicy Bypass -File scripts/Start-Local.ps1
~~~

打开 http://127.0.0.1:8080 。本地演示账户 demo@aihub.local / demo1234。
也可注册独立账户。演示数据为手工 fixture，不是真实 Provider 额度。
本地数据库是 WSL PostgreSQL；凭据仅在 .runtime/server.env，不进入包。

完整本地 Gate：
~~~powershell
powershell -ExecutionPolicy Bypass -File scripts/Gate.ps1
~~~
需要本次准备的 WSL Go/Flutter/PostgreSQL 环境；其他机器使用下述通用步骤。

## 通用单二进制方式（无需 Docker）

1. 安装 PostgreSQL 14+，创建专用数据库和非超级用户账户。
2. 解压运行包，选择 aihub-windows-amd64.exe 或 aihub-linux-amd64。
3. 配置 AIHUB_DATABASE_URL。Web/PWA 已嵌入二进制，无需 Node 运行时。
4. 服务启动会在事务与 advisory lock 保护下执行 migration，再监听 HTTP。

Linux：
~~~bash
export AIHUB_DATABASE_URL='postgres://aihub:YOUR_PASSWORD@127.0.0.1:5432/aihub?sslmode=disable'
export AIHUB_ADDR='127.0.0.1:8080'
./aihub-linux-amd64
~~~

Windows PowerShell：
~~~powershell
$env:AIHUB_DATABASE_URL = 'postgres://aihub:YOUR_PASSWORD@127.0.0.1:5432/aihub?sslmode=disable'
$env:AIHUB_ADDR = '127.0.0.1:8080'
.\aihub-windows-amd64.exe
~~~

默认不开启演示账户。仅本地试用时设置 AIHUB_DEMO=true。
数据库错误不打印连接串。/health 是存活检查，/ready 检查数据库。
systemd 示例见 deploy/aihub.service；AIHUB_DATABASE_URL 放 /etc/aihub.env，限制文件权限。
PostgreSQL 不在同机/隔离网络时使用 sslmode=verify-full 并配置信任证书。

## 公网 HTTPS（域名方式）

准备域名，将 A/AAAA 记录指向服务器，开放 80/443。
复制 deploy/.env.example 为 deploy/.env，替换域名和随机十六进制密码。
从完整源码包执行：

~~~bash
docker compose --env-file deploy/.env -f deploy/compose.yaml up -d --build
~~~

Caddy 自动签发证书。只发布 Caddy 的 80/443，不发布 PostgreSQL 或 app 端口。
公网禁止启用演示账户。首次注册后可通过反向代理限制注册路由。
如不用 Docker，把 Caddyfile 的 reverse_proxy 改为 127.0.0.1:8080，并配置你的域名。

两端打开同一 HTTPS 地址或在 Flutter 中添加同一 Server Profile，登录同一账户即可。
客户端不接受外网 HTTP、自签名证书跳过验证、含凭据的 URL 或 HTTP 重定向。
Web 使用不同站点连接远程 API 时，AIHUB_ALLOWED_ORIGINS 必须列出该 Web 的准确 origin，以逗号分隔；推荐同源部署。

## 仅有公网 IPv4 的 Ubuntu 部署

Ubuntu 24.04 新实例可用运行包中的 `install-ubuntu-ip.sh`，无需域名或 Docker。脚本安装 PostgreSQL、Nginx 和 Certbot 5.4+，只将 80/443 发布到公网；数据库与应用 HTTP 仅监听本机。它为公网 IP 申请 Let's Encrypt 的短期证书，并建立每天两次的续期检查。执行前须在云防火墙放行 TCP 80/443；Certbot 的 HTTP 验证需要从公网访问 80。不要将本机 `.runtime`、数据库或 bridge token 上传到服务器。

把 `aihub-m0-runtime.zip` 和 `install-ubuntu-ip.sh` 上传到服务器后运行：

~~~bash
sudo bash install-ubuntu-ip.sh YOUR_PUBLIC_IPV4 aihub-m0-runtime.zip
~~~

脚本不覆盖已有数据库凭据；证书申请暂时失败时可在修复 80 端口的公网访问后重跑。完成后，在电脑、手机浏览器访问同一个 `https://YOUR_PUBLIC_IPV4/` 并登录同一账户，电脑端 Codex 采集器的 server 地址也改为该 HTTPS 地址。IP 证书有效期约六天，必须保持服务器在线以便自动续期。运行后还需从外部网络验证 `/ready`、证书信任链和手机端实际登录同步。

## 手机端

- 本次直接可用的界面是响应式 Web/PWA；部署 HTTPS 后在手机浏览器打开，可使用“添加到主屏幕”。
- Flutter 源码位于 apps/mobile，包含 Android/Windows/Linux 工程；登录凭据由 flutter_secure_storage 保存，Drift 保存本地数据。
- 构建 Android 需要 Flutter、Java 17+ 和 Android SDK；本机未配置 Android SDK，本次不把源码测试等同于 APK 或真机验收。
~~~bash
cd apps/mobile
flutter pub get
flutter build apk --debug
~~~
当前 release 工程尚未配置发行签名，正式发行前必须替换 Flutter 默认 debug signing。
Android 仅申请 INTERNET，不包含小米 SDK、手环服务、联系人或定位权限。
Android production manifest 拒绝明文网络；本地模拟器测试可使用 adb reverse 与专用 debug 配置。
Windows Flutter 构建另需 Windows Flutter SDK 和 Visual Studio C++ 工具链；此次 Windows 可运行产物为服务端 + Web/PWA。

## 备份、恢复和升级

- 使用 pg_dump 备份 PostgreSQL；在独立库验证 pg_restore，再切换连接串。
- 备份期间保留原数据库，不自动覆盖；升级前记录现有 binary 与 schema_migrations。
- M0 migration 仅向前，不提供危险的自动降级。回滚用旧 binary + 对应数据库备份。
- 恢复较旧数据库后，在维护窗口更新 sync_streams 的 epoch（每个 workspace 新唯一值），让客户端重建缓存。不要裁剪 applied_operations 或 sync_events 后继续使用旧 epoch。
- 当前没有事件日志压缩、队列容量治理或公网压测；长期生产需要后续 hardening。
- 本地离线缓存不是数据库备份。撤销设备无法远程清除其已下载数据。

## 证据边界

本地验证涵盖真实 PostgreSQL、真实 Drift/SQLite、HTTP 联测、服务进程重启、浏览器桌面和手机宽度。
公网服务器已开始安装，但 HTTPS 和跨公网访问尚未完成；现场状态见[开发进度快照](DEVELOPMENT_PROGRESS_20260923_ZH.md)。尚未进行蜂窝网络/异地设备联测、Android/iOS 真机测试或后台通知可靠性观察。
Docker/CI 配置提供为可复跑入口；没有宣称在本次机器上执行过 Docker 或远端 GitHub Actions。

## Codex 电脑采集器

运行包现含独立 Windows/Linux 采集器。按 [Codex 连接说明](CODEX_CONNECTION_ZH.md) 在电脑端配对。无需将 Codex 凭据放到服务器；移动端只读取 AI Hub 上的摘要。
