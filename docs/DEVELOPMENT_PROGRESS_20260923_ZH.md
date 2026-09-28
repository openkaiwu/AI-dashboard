# AI Hub 开发进度（截至 2026-09-28 15:46，北京时间）

> 这是早期快照。M1/M2 与账户绑定的当前源码进度见 [2026-09-28 实施记录](M1_M2_IMPLEMENTATION_20260928_ZH.md)；下面的运行态描述不代表新分支已部署。

本文是当前开发快照，区分“源码已实现”和“正在运行的 exe 已部署”。公网地址与个人额度数据已脱敏；示例地址不代表实际服务入口。它不代表公网发布或手机真机验收完成。历史设计和完整验证记录见 [M0 验收清单](M0_Cursor_Dual_Target_Acceptance.md)、[验证记录](VALIDATION.md)、[Codex 提醒说明](CODEX_ADVISOR_ZH.md) 和 [部署说明](DEPLOYMENT_ZH.md)。

## 一句话结论

**源码已从纯 Codex 版本推进到 Codex + Cursor 双目标；本地旧 exe 尚未重新编译，因此当前 8080 运行态仍是纯 Codex。**

Web TypeScript 已通过，Cursor 本机 API 探测已成功，移动端同步 API 已通过本地联测；下一步关键是准备 Go 1.24、重新编译 server/bridge、替换运行包，再做真实 Cursor 上报和手机真机验收。

## Git 与代码状态

- 已初始化 Git，并保留三段基础历史：
  - `dcadb0e`：Codex 基线
  - `2234690`：M0 Cursor 双目标
  - `0428a9d`：边界检查修复
- 当前工作区还有未提交的 Phase 3 改动：约 26 个已修改文件和 5 个新文件。
- 当前没有配置远程仓库。
- Web `npx tsc --noEmit` 已通过。
- `git diff --check` 仍发现 `server/internal/connector/cursor.go` 的换行/尾随空白问题，提交前需要格式化。

## 功能进度

| 模块 | 当前状态 | 说明 |
| --- | --- | --- |
| Codex 基础工作台 | 源码完成，旧 exe 可运行 | Codex 采集、advisor、五页工作台、规则和便笺同步均保留；当前本地服务 `/ready` 正常。 |
| 通用 provider 架构 | 源码完成 | 已有 `codex` / `cursor` provider 与 connector，Cursor 快照复用 provider account、quota bucket、usage snapshot。 |
| Cursor 本机采集 | 源码完成，已实际探测 | Windows 只读 `%APPDATA%\\Cursor\\User\\globalStorage\\state.vscdb`，调用 `GetCurrentPeriodUsage`，失败回退 `auth/usage`；token 不进入上传 payload。 |
| Cursor 双桶解析 | 源码完成，未进入旧 exe | 支持 `Cursor Models` 与 `Other Models` 的 `autoPercentUsed` / `apiPercentUsed`，并保留总用量和重置时间。 |
| Cursor 上报 API | 源码完成，运行态未部署 | 源码有 `POST /api/v1/cursor/snapshot`；当前运行的 8080 旧 exe 对该接口返回 404。 |
| Web 模式切换 | 源码完成，开发检查通过 | Codex/Cursor 模式、Dashboard、Rules、Inbox、连接页和 provider-aware 强提醒已接入。 |
| 桌面模式切换 | 源码完成，旧安装包未更新 | 桌面端默认入口不再硬编码 `/codex`，但安装目录中的二进制仍是旧版。 |
| Flutter 模式切换 | 源码完成，APK 未重打 | Cursor 页面、通知中心和强提醒轮询已加入；移动端真机尚未重新安装验证。 |
| Rules 账户绑定 | 源码完成 | 支持规则绑定具体 `(provider, account)`，服务端 PATCH 已支持更新。 |
| stale/unknown 处理 | 源码完成，未部署 | 失败不制造额度数字；旧快照标记 `stale`，无历史时使用 `unknown` / `unavailable`，并阻止错误的额度告警。 |
| 重置雷达 3 小时 | 源码完成，自动任务待调整 | 服务端和 Web 过期窗口已改为 3 小时；本机 `tibo-codex` 自动任务本身仍需从每日任务调整为每 3 小时。 |

## 已完成的实际验证

### Cursor API

本机 Cursor 登录态读取成功，实际返回结果与 Plan & Usage 截图一致：

- 套餐：`pro`
- Cursor Models、Other Models：已验证解析；具体个人用量已脱敏。
- 当前计费周期结束时间：已脱敏。

探测脚本：

```powershell
powershell -ExecutionPolicy Bypass -File scripts/Probe-Cursor.ps1
```

脚本只输出消毒后的 snapshot，不打印 token。

### 本地服务与移动端 API

最近一次本地联测结果：

- `/ready`、`/api/v1/meta`：通过
- demo 登录：通过
- `sync/pull`、`sync/push` 往返：通过
- `devices`、`codex/overview`：通过
- 移动端风格的 `/me`、`sync/pull`、`dashboard`、`devices`：通过

本地访问地址：

```text
http://127.0.0.1:8080
```

演示账户：

```text
demo@aihub.local / demo1234
```

### Web

- `apps/web` 的 TypeScript 检查已通过。
- Vite 开发预览曾成功运行在 `http://127.0.0.1:5173`。
- 5173 需要后端允许 `AIHUB_ALLOWED_ORIGINS=http://127.0.0.1:5173`。

## 当前运行态

### 本地 server

- 8080 正在运行，`/ready` 返回 `status=ok`。
- 当前进程来自 `artifacts/aihub-m0/aihub-windows-amd64.exe`。
- 通过 `POST /api/v1/cursor/snapshot` 返回 404，确认当前运行二进制仍未包含 Cursor 上报路由。

### 本地 bridge

- `%APPDATA%\\AI Hub\\runtime\\bridge.json` 已包含 `cursor_enabled: true`，但当前配置指向本地 8080。
- bridge 进程记录文件中的 PID 已失效，当前没有可确认运行中的 bridge。
- 最近日志仍主要是旧 Codex 采集失败记录，不能据此证明 Cursor 已上报。

### 云端

- 目标服务器仍为 `https://hub.example.com`。
- 当前从本机访问时 TLS 握手失败或返回空响应，SSH 连接也未成功。
- 因此公网服务版本、云端 Cursor 路由、手机跨网络登录和云端同步目前都不能标记为已验收。
- 不应把本地 token 直接复制到云端；云端应重新创建连接凭据。

## 数据与安全边界

电脑采集器只上传消毒后的额度窗口、百分比、重置时间、采集状态和提醒所需摘要。Cursor access token 只在本机用于访问 `api2.cursor.sh`，不进入 AI Hub 上传 payload；采集器不监听公网端口。

AI Hub 会持久化账户、连接 token 哈希、额度摘要、历史和提醒状态。这里的“只中转”表示服务器不运行 Codex、不代持 Codex/Cursor 登录凭据，并不表示服务器完全无状态。

## 当前阻塞

1. Windows 没有 Go；WSL 只有 Go 1.18，而项目 `go.mod` 要求 Go 1.24。
2. 新源码尚未编译成 server/bridge exe，旧运行包仍是纯 Codex。
3. `server/internal/connector/cursor.go` 需要先做 Go 格式化和换行清理。
4. bridge 当前未运行，且旧 bridge 无法证明支持 Cursor。
5. 公网 HTTPS/TLS 链路尚未恢复，手机真机跨网无法验收。
6. 3 小时重置雷达的实际自动任务调度仍需调整；源码过期判定已完成。
7. Flutter APK 尚未用当前源码重打，手机后台推送仍未实现。

## 下一步顺序

1. 准备 Go 1.24，并运行 `gofmt`、`go test`、服务端构建检查。
2. 修复 `git diff --check` 的格式问题，确认 Cursor collector、connector、notification 测试。
3. 编译新的 `aihub-server.exe` 和 `aihub-bridge.exe`，替换本地运行版本。
4. 重启本地 server，确认 `/api/v1/cursor/snapshot` 不再返回 404。
5. 使用 Cursor bridge 执行 `--probe cursor` 和 `--once`，确认两个额度桶写入 Dashboard。
6. 将 `tibo-codex` 自动任务改为每 3 小时，并验证雷达过期状态。
7. 修复公网 HTTPS 后，再重新创建云端 bridge 连接，验证连续上传。
8. 用重新构建的 Flutter APK 在真机登录同一账户，验证 Dashboard、通知和便笺同步。

## 相关文件

- [M0 Cursor 双目标验收清单](M0_Cursor_Dual_Target_Acceptance.md)
- [Cursor 提醒与采集说明](CODEX_ADVISOR_ZH.md)
- [部署说明](DEPLOYMENT_ZH.md)
- [验证记录](VALIDATION.md)
- [Bridge 说明](../bridge/README.md)
