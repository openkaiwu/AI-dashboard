# AIDASH / AI Hub 功能体检记录

> 日期：2026-09-24（Asia/Shanghai）  
> 历史公网地址已脱敏；文中的示例域名和文档 IP 不代表实际服务入口。
> 范围：本机候选运行环境 + 公网入口只读探活；不改代码、不重启服务。  
> 联调脚本：`scripts/Verify-Clients.ps1` → **15/15 PASS**

## 1. 总览

本机服务、公网入口、登录、Codex bridge 实时上报、桌面端、APK 下载校验当前正常。未登录访问受保护接口返回 401，符合鉴权门禁预期。

下列两项仅为设计文档、**不计入已上线功能**：

- Cursor 双目标自动采集（见 `M0_Cursor_Dual_Target_Acceptance.md`）
- 桌面 ↔ 手机一对一设备绑定（见 `DEVICE_BINDING_1TO1_ZH.md`）

## 2. 运行时快照

| 项 | 观测 |
|----|------|
| 本机 API | `aihub-windows-amd64` 监听 `127.0.0.1:8080` |
| 本机 DB | PostgreSQL 监听 `127.0.0.1:5432` |
| 桌面端 | 进程「AI Hub」在跑；安装于当前用户的本地应用目录 |
| Bridge | 2 个 `aihub-bridge-windows-amd64` 进程在跑（脚本按 count=2 计为 PASS） |
| 公网 | `https://hub.example.com`；另有 `:8888` HTTP 入口 |
| 版本 | `/api/v1/meta` → `protocol=1`，`schema=1`，`version=0.2.0-m0` |

## 3. 分项结果

| 功能 | 状态 | 证据 |
|------|------|------|
| 本机 API 服务 | 正常 | `/api/v1/health`、`/ready` 返回 `status=ok`；`/api/v1/meta` 版本如上 |
| 本机数据库 | 正常 | `127.0.0.1:5432` 监听 |
| 公网入口 | 正常 | `/`、`/ready`、`/api/v1/health`、`/login`、`/codex`、`/connections` 均 HTTP 200 |
| 鉴权门禁 | 正常 | `POST /api/v1/auth/login` 空体 → 400（路由存在）；无 token 访问 devices / dashboard / codex → 401 |
| 登录 / Codex 概览 | 正常 | Verify-Clients：公网登录 PASS；`codex/overview` 可见 alerts；`codex/bridges` count=2 |
| Codex bridge 采集上报 | 正常 | `.runtime/bridge.log` 近期周期性 `synced: status=ok buckets=1`；live-upload PASS（约 13:05 +08） |
| Bridge 进程 | 正常（备注） | 双进程：其一配置 `.runtime\bridge.json`，其一在桌面 AppData 配置目录 |
| Web UI | 正常 | 本机与公网页面入口 200，返回 AI Hub 前端 |
| 桌面端 | 正常 | 已安装并在跑；Desktop IPC 单测 PASS |
| Android APK 产物 | 正常 | 可下载且 SHA256 匹配 |
| 提醒 / 通知 API | 门禁正常 | `/api/v1/notifications` 无 token → 401；登录后 overview 可见 alerts |
| Sync / 设备 API | 门禁正常 | `/api/v1/sync/pull`、`/api/v1/devices` 无 token → 401 |
| 公网 SSH 巡检 | 降级 | `Verify-Public.ps1` SSH：`Permission denied (publickey)`；不影响 HTTP/客户端联调 |
| 本机 `:8888` | 未监听 | 公网 `http://203.0.113.10:8888/` 可通；本机 8888 无法连接 |
| Cursor 自动采集 | 未上线 | 设计/文档阶段；不计入本次已实现功能 |
| 桌面↔手机 1:1 绑定 | 未上线 | 仅文档 `DEVICE_BINDING_1TO1_ZH.md` |

## 4. 脚本证据

### 4.1 Verify-Clients（通过）

路径：`scripts/Verify-Clients.ps1`  
摘要（2026-09-24）：

- [PASS] `/ready`、`/api/v1/meta`
- [PASS] 公网登录、本机 demo 登录
- [PASS] `codex/overview`、`codex/bridges`
- [PASS] bridge live-upload
- [PASS] APK 下载与 SHA256
- [PASS] Desktop installed / running、Bridge running
- [PASS] Local `/ready`、Desktop IPC unit test  
- **Summary: 15/15 passed**

完整输出曾落在：`.runtime/verify-clients-out.txt`（本机运行产物，勿打包进发布）。

### 4.2 Verify-Public（部分降级）

路径：`scripts/Verify-Public.ps1`  
HTTP/HTTPS 入口可通；SSH 到 `203.0.113.10` 公钥认证失败，属运维通道问题。

## 5. 备注与后续可选动作

1. **双 bridge 进程**：联调脚本按 2 计为通过；若非刻意双开，建议确认是否重复上报。  
2. **历史日志**：`.runtime/bridge.log` 更早曾出现 `Upload unavailable` / `Codex quota unavailable`，最近周期已恢复 `synced ok`。  
3. **公网 SSH 公钥**：仅影响运维脚本，不影响用户侧 Web/桌面/手机经 HTTP(S) 使用。  
4. 可选下一步：登录后做一次 Web Codex 页界面点检；或梳理双 bridge 配置来源。

## 6. 相关文档

- `docs/VALIDATION.md` — 既有 M0 验证记录  
- `docs/M0_Cursor_Dual_Target_Acceptance.md` — Cursor 双目标 M0（设计）  
- `docs/DEVICE_BINDING_1TO1_ZH.md` — 桌面↔手机一对一绑定（设计）  
- `docs/CODEX_CONNECTION_ZH.md` — Codex bridge 连接说明  
