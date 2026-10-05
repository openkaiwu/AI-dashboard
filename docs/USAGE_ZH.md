# AI Hub 使用文档（自部署版）

本文面向自行部署 AI Hub 的使用者与服务器管理员：从零开始完成服务端部署、管理员初始化、客户端接入，并了解全部功能与配置。架构与开发细节见各专项文档（文末索引）。

> 仓库与文档中的 `127.0.0.1`、`hub.example.com`、`203.0.113.10` 均为占位地址。你自己的服务器地址只需在**客户端设置**和**部署配置**里填写，永远不要写入源码或构建参数。

---

## 1. AI Hub 是什么

自部署的 AI 工作空间 + Codex/Cursor 额度伴侣。一个 Go 单二进制服务器内嵌 Web/PWA，配合桌面采集器（Electron + Go bridge）与 Flutter 手机 App，实现：

| 组件 | 说明 |
|---|---|
| **服务器** `aihub` | Go 单二进制，内嵌 Web UI 与全部 API；PostgreSQL 为唯一外部依赖 |
| **Web / PWA** | 浏览器直接访问服务器即用；支持离线便笺 |
| **桌面端**（Windows/Linux/macOS） | Electron 壳 + 本地额度采集桥（读取本机 Codex / Cursor），可选内嵌本地服务端（本地开发模式） |
| **手机 App**（Android/iOS） | Flutter；与桌面共享账户、规则与便笺 |
| **管理台** | 管理员 Web 控制台：账户/设备/邀请码/注册审核/雷达令牌/运维报表 |

核心能力：Codex 额度窗口实时采集（5 小时 + 周窗口，按套餐自适应）、节奏建议与强提醒、重置卡到期提醒、重置雷达（外部消息）、每日/每周/每月消耗分析、跨端便笺、邀请码注册与管理员审核。

## 2. 快速开始

### 2.1 本地演示（Windows，5 分钟）

```powershell
powershell -ExecutionPolicy Bypass -File scripts/Start-Local.ps1
# 打开 http://127.0.0.1:8080
# 演示账户：demo@aihub.local / demo1234（额度为演示数据）
powershell -ExecutionPolicy Bypass -File scripts/Stop-Local.ps1   # 停止；数据保留
```

### 2.2 服务器部署（Ubuntu）

**方式 A：一键脚本（推荐）**

```bash
# 在仓库根目录先产出服务器运行包（内嵌 Web 的单二进制）
bash scripts/build.sh
# 在目标 Ubuntu 服务器上执行（需公网 IPv4）：
sudo bash deploy/install-ubuntu-ip.sh <公网IPv4> artifacts/aihub-m0-runtime.zip
```

脚本完成：系统依赖（PostgreSQL/nginx/python3）、`/opt/aihub` 安装、`/etc/aihub.env` 配置、systemd 服务、nginx 443（自签证书）与 `/downloads/` 下载目录。

**方式 B：Docker Compose**

```bash
cd deploy/selfhost
POSTGRES_PASSWORD=强密码 docker compose up -d     # 服务 8080 端口
```

**方式 C：手动**

1. 安装 PostgreSQL 15+，创建库与账户；
2. 放置 `aihub` 二进制与 `/etc/aihub.env`（见第 6 节）；
3. `systemctl enable --now aihub`（单元样例 `deploy/aihub.service`）；
4. 前置 nginx 终结 HTTPS（样例 `deploy/nginx-aihub.conf`），或直接暴露 8080（仅限内网）。

> 公网部署必须 HTTPS：客户端对非本机地址强制 HTTPS。证书可用 Let's Encrypt（域名）或自签 + 客户端信任。

### 2.3 初始化管理员

服务器上执行（首次仅一个管理员；密码从 stdin 读入）：

```bash
sudo -u aihub aihub-admin --email you@example.com <<< '你的强密码'
```

### 2.4 客户端接入

1. **桌面端**：安装包（见第 7 节下载或自行构建）→ 启动 → 选择「登录服务器」→ 输入 `https://你的服务器地址` → 用管理员或成员账户登录。每个账户绑定 **1 台电脑 + 1 台手机**。
2. **新成员**：管理员在管理台生成邀请码 → 成员在登录页「使用邀请码注册」（邀请码 + 邮箱 + 密码 + 留言）→ 管理员批准 → 登录。
3. **手机 App**：安装 APK/ipa → 同一账户登录 → 自动完成手机侧绑定。

## 3. 功能使用

### 3.1 额度总览

- 采集器每 5 分钟读取本机 Codex（`codex app-server` 的 rateLimits）与 Cursor，按账户上传；
- 窗口卡片按套餐自适应：**Plus/Pro** 显示 5 小时 + 周窗口；**Pro Lite** 实测仅返回周窗口（卡片会注明"Codex 暂未返回该窗口"）；
- 每张卡片：剩余百分比、距离重置、均匀使用预算、近期消耗速率、本窗口趋势图。

### 3.2 消耗分析

「消耗分析」tab：四个粒度——**当前窗口**（自窗口起点的原始曲线，重置处分段）、**每日**（30 天，可选 7/14/30/90）、**每周**（12 周）、**每月**（12 月）。含均匀参考线、超速高亮、重置标记 ▲、无样本空档与覆盖小时数。数据由服务器小时级 rollup 聚合（原始样本保留 7 天，聚合保留 400 天）。

### 3.3 使用计划 / 提醒中心 / 强提醒

- 「使用计划」：按剩余时间与预留额度试算本轮可用预算；
- 提醒规则跨端同步（阈值/免打扰/冷却），触发进入提醒中心：稍后 1 小时 / 忽略 / 恢复；
- 强提醒：桌面弹窗 + 手机弹窗（Android 全屏 intent、iOS 时间敏感通知），动作按钮（已读 / 1 小时后提醒 / 忽略）三端同 ID。

### 3.4 重置雷达

追踪 @thsottiaux 的 Codex 重置/额外额度/重置卡公开消息，结果**全员可见**。两种上报方式：

1. **桥接顺带上传**：自动化任务把白名单 JSON 写入 `codex-news.json`（安装版 `%APPDATA%\aihub-desktop\runtime\`），桥接器随快照上传；
2. **直传（推荐）**：管理台「重置雷达令牌」生成令牌 → 自动化任务检查后执行：

```bash
curl -s -X POST https://你的服务器/api/v1/radar/news \
  -H "Authorization: Bearer <雷达令牌>" \
  -H "Content-Type: application/json" \
  -d @codex-news.json
```

新鲜度 45 分钟，超期显示"检查记录已过期"；白名单校验（仅 `x.com/thsottiaux/status/`、≤10 条、≤600 字）服务端强制执行；旧检查不会回滚新雷达。

### 3.5 跨端便笺

离线优先：本地 Drift/SQLite 保存 → 持久队列 → 增量事件流同步；冲突保留版本；删除与重试幂等。

## 4. 管理员手册（`https://你的服务器/admin`）

管理员账户登录网页版即进入管理台（`⚙ 账户管理`）。

| 区块 | 能力 |
|---|---|
| 创建账户 | 绕过邀请码直接开户 |
| 注册审核 | 待审列表 → 批准 / 拒绝（原因留审计）；被拒邮箱由「账户」区块重新授权 |
| 邀请码 | 生成（备注/有效天数/可用次数）、吊销/恢复；明文仅显示一次，服务端只存哈希 |
| 账户 | 停用/授权、重置密码、查看与解绑设备 |
| 重置雷达令牌 | 生成/轮换/吊销自动化直传令牌 |
| G1 报表 / 运维状态 | 采集新鲜度、失败分类、账户设备量、迁移与备份状态 |

## 5. 配置参考

### 5.1 服务端环境变量（`/etc/aihub.env` 或 compose）

| 变量 | 说明 |
|---|---|
| `AIHUB_ADDR` | 监听地址，默认 `127.0.0.1:8080`（公网用 `0.0.0.0:8080` + 反代） |
| `AIHUB_DATABASE_URL` | PostgreSQL DSN（必需） |
| `AIHUB_ALLOWED_ORIGINS` | 允许的来源（HTTPS 部署填写你的域名/IP） |
| `AIHUB_DEMO` | `true` 启用演示模式（演示账户与数据） |
| `AIHUB_BACKUP_DIR` | 备份目录 |

### 5.2 桥接配置（桌面端 `bridge.json`，一般自动生成）

| 字段 | 说明 |
|---|---|
| `server` | 上报服务器地址 |
| `device_id` | 绑定的电脑设备 ID |
| `codex_path` | Codex 可执行文件绝对路径（留空则自动发现） |
| `cursor_enabled` | 是否同时采集 Cursor |
| `interval_seconds` | 采集周期，默认 300 |

### 5.3 客户端默认

桌面与手机的服务器地址默认 `http://127.0.0.1:8080`（本机模式）；远程部署在客户端设置里改为你的 HTTPS 地址即可，地址只保存在本机。

## 6. 构建与发布

```bash
# Web + 服务器（Linux/Windows 单二进制，内嵌 Web）
bash scripts/build.sh

# 桌面端
cd apps/desktop && npm ci
npm run dist                                  # Windows NSIS
npx electron-builder --linux                  # AppImage + deb
npx electron-builder --mac                    # dmg + zip（需 macOS）

# 移动端
flutter build apk --release                   # Android
flutter build ios --release --no-codesign     # iOS 未签名（侧载见 docs/IOS_BUILD_AND_SIDELOAD_ZH.md）
```

打 `v*` 标签触发 GitHub Actions `Release Packages`：Linux AppImage/deb、macOS dmg、iOS 未签名 ipa 自动出包（Windows 与 APK 用上述本地命令构建）。

## 7. 安全模型

- 密码 bcrypt；会话 access 15 分钟 + refresh 30 天轮换；设备独立授权/撤销；
- 桥接与雷达令牌只存 SHA-256 哈希，可随时撤销；
- 邀请码只存哈希、原子扣减次数、注册接口按 IP 限速（20 次/小时）；
- 注册产生 pending 账户，管理员审核是登录必经闸门；
- 全部管理操作与注册/雷达事件写审计表；
- 客户端凭据用系统凭据库保护（Windows DPAPI / macOS Keychain / Linux libsecret）。

## 8. 故障排查

| 现象 | 处理 |
|---|---|
| 额度卡片"等待新数据" | 桌面端桥接采集失败：确认本机 Codex 已登录；用 `aihub-bridge.exe --probe <codex路径>` 直读排查 |
| 雷达"检查记录已过期" | 自动化任务未按时上报：恢复任务或检查雷达令牌 |
| 登录提示"等待管理员审核" | 注册尚未批准，联系管理员 |
| Linux 凭据保护不可用 | 安装 `gnome-keyring` + `libsecret`（无钥匙环环境无安全存储） |
| 消耗分析"积累中" | rollup 自上线日起积累，周/月视图需时间 |
| 公网访问被拒 | 客户端对非本机地址强制 HTTPS；检查证书与 `AIHUB_ALLOWED_ORIGINS` |

## 9. 文档索引

| 文档 | 内容 |
|---|---|
| `docs/CODEX_ADVISOR_ZH.md` | 节奏建议/提醒/重置雷达规则与验收 |
| `docs/CODEX_CONNECTION_ZH.md` | Codex 连接与采集细节 |
| `docs/INVITE_REGISTRATION_ZH.md` | 邀请码注册与管理员审核 |
| `docs/QUOTA_CONSUMPTION_DESIGN_ZH.md` | 消耗分析数据口径与设计 |
| `docs/CROSS_PLATFORM_QA_ZH.md` / `docs/IOS_BUILD_AND_SIDELOAD_ZH.md` | 跨平台 QA / iOS 侧载 |
| `docs/OPERATIONS_ZH.md` / `docs/DEPLOYMENT_ZH.md` | 运维与部署细节 |
