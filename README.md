# AI Hub · M0 Foundation

公开仓库中的 `hub.example.com` 与 `203.0.113.10` 均为脱敏示例地址。实际服务器地址、账户和连接凭据只应在本机配置，不能写入源码或构建参数。

电脑与手机共享的自部署 AI 工作空间。以 AI-dashboard 的 Go/React 额度看板为基础，
参考 codex-quota-band 的设备同步与隐私边界，按 Linear M0 实现 PostgreSQL + Flutter/Drift + 增量同步。
不包含小米手环功能。

## Codex 智能提醒

首页已特化为 Codex 额度节奏助手：多用/慢用建议、重置卡到期提醒、近 6 小时趋势、跨端规则、免打扰与 Tibo 消息。见 [规则与通知说明](docs/CODEX_ADVISOR_ZH.md)。

## Codex 本地连接

已实现电脑 Codex 真实额度采集、专用可撤销连接、Web/PWA 与 Flutter 展示和缓存。详见 [Codex 连接说明](docs/CODEX_CONNECTION_ZH.md)。运行包含 Windows/Linux 采集器及 Windows 启停脚本。

## 现在可用

- **跨端便笺**：离线保存、持久队列、版本冲突处理、删除同步、重试幂等。
- **设备与账户**：独立设备授权/撤销、access/refresh 轮换、审计。
- **服务器配置**：可切换自部署服务器，外网强制 HTTPS。
- **现有额度功能**：手工登记、在线额度看板、提醒规则、通知、历史记录。
- **客户端**：可运行 Web/PWA；Flutter Android/Windows/Linux 源码及 Drift 核心测试。
- **部署**：Windows/Linux 服务端单二进制（已内嵌 Web），PostgreSQL；可选 Docker + Caddy。

M0 增量事件流同步的是便笺；Codex 自动采集作为后续增量，使用独立快照接口。其他 Provider 自动采集、额度领域离线事件同步、手机后台通知及多人协作仍属后续阶段。

## 在这台电脑启动

~~~powershell
powershell -ExecutionPolicy Bypass -File scripts/Start-Local.ps1
~~~

打开 **http://127.0.0.1:8080**。
本地演示账户：**demo@aihub.local / demo1234**。演示额度不代表真实 Provider 数据。
可注册自己的本地账户。

~~~powershell
# 停止本次本地服务；数据库和便笺保留
powershell -ExecutionPolicy Bypass -File scripts/Stop-Local.ps1
~~~

本机启动脚本复用 WSL PostgreSQL，并保持 WSL 存活，避免系统空闲回收导致原生服务失去数据库连接。
其他机器请使用 [部署说明](docs/DEPLOYMENT_ZH.md)，不要依赖本机 .runtime 配置。

## 文档与产物

- [当前开发进度（2026-09-23）](docs/DEVELOPMENT_PROGRESS_20260923_ZH.md)
- [开发思路及 M0/M1/M2 边界](docs/DEVELOPMENT_PLAN_ZH.md)
- [冻结协议、模块所有权、威胁模型](docs/CONTRACTS.md)
- [运行、公网 HTTPS、自部署与备份](docs/DEPLOYMENT_ZH.md)
- [验证记录与尚未验证的内容](docs/VALIDATION.md)
- [上游版本与授权来源](docs/UPSTREAM.md)

运行包：artifacts/aihub-m0-runtime.zip。完整源码/部署包：artifacts/aihub-m0-source.zip。
运行包不含数据库、账号凭据或示例用户数据；请配置自己的 PostgreSQL。
Android APK 和 Windows Flutter 原生客户端未在本机打包；可运行桌面产物为服务端 + Web/PWA。

## 验证与构建

本机一键完整 Gate：
~~~powershell
powershell -ExecutionPolicy Bypass -File scripts/Gate.ps1
~~~

Linux/CI：
~~~bash
npm ci --prefix apps/web
npm test --prefix apps/web
npm run build --prefix apps/web
cp -R apps/web/dist/. server/webassets/dist/
export AIHUB_TEST_DATABASE_URL='postgres://USER:PASSWORD@127.0.0.1:5432/TEST_DB?sslmode=disable'
bash scripts/gate.sh
~~~

Gate 在独立 PostgreSQL schema 验证 Go/Auth/Sync/jobs，并启动临时服务，
用两个独立 Drift 客户端做真实 HTTP 联测。没有数据库的普通 go test 会跳过集成测试，
不能作为 M0 Gate 已通过的证据。

构建完整运行包见 scripts/Build-Release.ps1（本机）或 scripts/build.sh（Linux）。
