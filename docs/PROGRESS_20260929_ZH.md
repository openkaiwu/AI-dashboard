# 本地完成进度总览（2026-09-29）

本文汇总截至 2026-09-29 本地实现与验证的完成进度，覆盖 M0–M6 全部里程碑。状态分三档：**已完成**（票面范围全部实现且回归通过）、**部分完成**（核心已落地，余项见说明）、**未开始**。本文不等于正式验收；Gate 票收口、契约评审与 G1 观察窗仍属流程动作。

## 里程碑总览

| 里程碑 | 票数 | 已完成 | 部分完成 | 未开始 | 回归验证 |
| --- | --- | --- | --- | --- | --- |
| M0 · Foundation & Sync | 10 | 9（实现+回归矩阵齐备） | — | Gate 收口待流程 (INH-327) | ✅ gate.sh PASS |
| M1 · Quota & Reminder | 9 | 9（含 E2E 纵切、账单字段、本地通知） | — | — | ✅ INH-361 E2E PASS |
| M2 · Connector Framework | 14 | 12（含 official_api 连接器） | 2（412 回放框架、404 真站提取器） | — | ✅ INH-416 E2E PASS |
| M3 · Conversation Portability | 7 | 3（446/450/454） | 3 | 1 | ✅ JSONL/搜索/警告回归 |
| M4 · Portable Config | 7 | 4（467/471/473/477） | 2 | 1 | ✅ 反向 TOML 往返回归 |
| M5 · Workspace & Collab | 7 | 4（503/505/509/516） | 2 | 1 | ✅ ACL/隔离/事件回归 |
| M6 · Promotion Intelligence | 5 | 2 | 2 | 1 | ✅ 去重/通知回归 |

## 关键提交

| 提交 | 内容 |
| --- | --- |
| `ff450de` | 修复 connector→devices 边界违规（设备校验收敛至 auth 包），官方 gate 首次全绿 |
| `8f8e74d` | M3/M4 核心纵切：canonical 会话图 + 可移植配置（27 文件，约 3200 行） |
| `82bfdd7` | M5/M6 核心纵切：协作工作区 + 优惠情报（含 SSE change hints、Bridge 配置发现） |

## M3 · Conversation & Project Portability

| 票 | 状态 | 说明 |
| --- | --- | --- |
| INH-433 Contract | 部分完成→待评审 | 迁移 013、canonical 模型、archive v1 信封 |
| INH-438 Contract | 部分完成→待评审 | 导入限额、单事务批次、raw 快照保留、去重身份 |
| INH-442 | 部分完成 | jobs 异步管道 + worker；object storage 未含 |
| INH-446 | 已完成 | chatgpt_export 树形导入器 + golden fixtures + warnings（跳过节点/截断分支全部显式记录）；附件映射待真实样本扩展 |
| INH-450 | 已完成 | codex_cli_jsonl 第二来源（容忍外部行 + 逐行警告）；更多 Provider 待真实样本 |
| INH-454 | 已完成 | Web 会话浏览器 + 内容搜索（标题/消息 ILIKE） |
| INH-455 Gate | 部分完成 | Archive/Markdown/JSONL 导出 + round-trip 回归通过；附件包含策略、导出任务化未做 |

## M4 · Portable Config & Local Bridge

| 票 | 状态 | 说明 |
| --- | --- | --- |
| INH-459 Contract | 已完成→待评审 | canonical 内容/SecretRef/loss_report 契约（含双重包装 bug 修复） |
| INH-463 | 部分完成 | ConfigAsset/version/binding 持久化与 API；sync wire 扩展未做（需 sync 契约 v2） |
| INH-467 | **已完成** | Bridge 授权目录扫描 + Secret 键名分类 + 服务端存储 |
| INH-471 | **已完成** | claude_desktop → canonical → codex_cli 参考转换（golden 断言） |
| INH-473 | **已完成** | 反向 TOML 解析器 + canonical→claude_desktop 输出 + ${NAME} 占位符回映射 + 往返测试 |
| INH-477 | **已完成** | diff/rollback API + Web 版本对比/转换预览/损失记录展示/双平台预览 |
| INH-479 Gate | 部分完成 | secret leakage / rollback / round-trip 回归通过；正式收口待流程 |

## M5 · Workspace & Collaboration

| 票 | 状态 | 说明 |
| --- | --- | --- |
| INH-501 Contract | 部分完成→待评审 | 工作区/角色 ACL 契约；共享域严格限定 canonical 资产 |
| INH-503 | **已完成** | Workspace/Member/Invite 持久化与管理 API（含 invites/mine） |
| INH-505 | **已完成** | 共享读取经 workspace.ReadableScope；隔离/越权用例通过 |
| INH-509 | **已完成** | 共享 + 评论/mention + workspace_events 活动流（share/comment/member/branch 事件）+ Feed API |
| INH-513 | 部分完成 | SSE change hints（Last-Event-ID 重放、256 条缓冲）；WebSocket 升级留收口 |
| INH-516 | **已完成** | 协作工作区页面 + 角色标签 + 权限说明（所有者/编辑者/查看者能力边界） |
| INH-519 Gate | 部分完成 | ACL/隔离/移除即失效/SSE 重连/审计回归通过；正式收口待流程 |

## M6 · Promotion Intelligence

| 票 | 状态 | 说明 |
| --- | --- | --- |
| INH-484 Contract | 部分完成→待评审 | Promotion/Source/Confidence/Dedup 契约 |
| INH-488 | 部分完成 | Source Registry + RSS/Atom 适配器（可注入 fetch）+ 用户提交；官方站点解析器待真实证据 |
| INH-491 | **已完成** | 归一化/去重（URL 追踪参数剥离）、Provider/plan/region 匹配、提交即判过期 + ticker 扫描 |
| INH-495 | **已完成** | Watchlist CRUD + Feed + 提交表单（Web） |
| INH-497 Gate | 部分完成 | Watch→dedup→通知纵切回归通过；正式收口待流程 |

## 第二轮补全（2026-09-30）

- **M1/Gate INH-361**：手工额度→规则求值→通知→历史纵切 E2E（含跨设备一致性与重放幂等）PASS。
- **M2/Gate INH-416**：codex 桥接/cursor 桥接/手工录入三种采集模式进统一额度模型 E2E PASS。
- **M2/INH-399**：official_api 连接器（服务端上传端点 + 桥接端可注入 fetch 采集 + provider 自动登记）PASS。
- **M1/INH-339**：账单字段扩展（invoice_ref/plan_code/period，迁移 018）。
- **M1/INH-353**：安卓本地通知 v1（flutter_local_notifications，强提醒同步进系统托盘；后台推送仍留）。
- **M4/INH-473**：反向 TOML 解析器 + canonical→claude_desktop 输出 + 往返测试（修复 secret_ref 双重包装 bug）。
- **M5/INH-509**：workspace_events 活动流 + Feed API + 分支事件接入导入流程。
- **M3**：JSONL 导出、会话内容搜索、导入器 warnings（迁移 017）。

## 第三轮：G1/R3 地基（2026-09-30）

- **G1/INH-421**：Alpha 观察报表已落地——`internal/telemetry` 只读聚合权威表（连接线新鲜度、额度桶 3 小时新鲜度、失败分类 taxonomy、量级计数），管理员端点 `GET /api/v1/admin/telemetry` + 管理台展示区块。设计为纯聚合，无重复记账。
- **R3/INH-543 地基**：`GET /api/v1/admin/operations` 输出数据库可达性/迁移版本/任务队列/备份策略占位（INH-538 接入后自动反映）。
- **R3/INH-541 地基**：`TestR3CriticalE2EChain` —— Sync（冲突+解决）→ Quota（连接器上传+手工修正）→ Assets（会话+配置导入）→ ACL（共享+成员读+越权 403）→ Telemetry/Operations 全链 E2E PASS。
- G1 剩余：INH-425（两周真实观察窗，等 421 的报表开始积累数据后启动）→ INH-429（evidence review）。

## 第四轮：R3 相关功能实现（2026-09-30）

- **INH-540**：升级预检 + 版本拒绝——`db.Preflight` 在任何迁移前校验数据库兼容性；由更新二进制迁移过的库会被拒绝（exit 2，旧进程永不改动新 schema）；迁移保持单事务原子（失败即回滚）；新增 `AIHUB_MIGRATE_ONLY=1` 升级模式。测试：`TestPreflightVersionRejection`（隔离 schema）PASS。
- **INH-538**：`scripts/backup.sh`（pg_dump --clean + gzip）与 `scripts/restore.sh`（二次确认 + 单事务重放）；**恢复演练自动化**：`TestR3BackupRestoreDrill` 真实 pg_dump → 破坏 schema → 恢复 → 数据回归校验 PASS；operations 端点新增备份状态（`AIHUB_BACKUP_DIR`）与升级预检结果。
- **INH-542**：`deploy/selfhost/`——systemd 单元（含加固项）、可选 docker-compose（PostgreSQL 17 + 自动构建镜像）、`aihub.env.example`、Dockerfile（单二进制 + pg 客户端工具）。
- **INH-544**：`scripts/security-audit.sh` 自动化 13 项检查（go vet、密钥/私钥扫描、注册关闭、桥接设备校验、令牌哈希、secret_ref、扩展无 cookie、CORS、raw 快照隔离）——当前 PASS；人工清单在运维手册。
- **INH-545**：`docs/OPERATIONS_ZH.md` 运维手册——安装/升级/备份恢复/观测/安全审计 + Release Checklist。
- **INH-543/541**：operations 端点与关键链 E2E 地基已就位（上一轮），本轮扩展备份状态与预检字段。

R3 剩余：INH-546（清洁安装 Human Gate——需真实机器按手册走完整发布流程）与 541/543 的收尾扩展。

## 本地验证记录（全部真实 PostgreSQL 14，WSL Ubuntu-22.04）

| 验证项 | 结果 |
| --- | --- |
| `scripts/check-boundaries.py`（白名单已扩展至 conversation/config/workspace/promotion） | PASS |
| `go vet ./...` + `go build ./...` | PASS |
| Go 集成回归（新增 8 项，连接真实 PG） | 全部通过 |
| 完整官方 `scripts/gate.sh`（边界 + Web + Go -race + PG 服务端 + Flutter） | **PASS** |
| Web `tsc + vite build`（新增 4 页面：/conversations /config /workspaces /promotions） | PASS |
| Flutter analyze + 测试（含 gate 内真实服务器联测 12/12） | PASS |

## Linear 看板同步状态

自动化同步已完成并经截图/快照验证的票：

- **Done**：INH-303、433、438、446、450、454、459（另有多张 M0/M2 批量更新回报成功但未经逐张验证）
- **In Progress**：INH-442、455、327（M0/Gate）
- **In Review→Done**：INH-459 已改判 Done

自动化在本会话后期因页面水合引用漂移与渲染超时不可靠，以下票**待手动或下次会话同步**（建议状态依据本文档逐票表）：

| 里程碑 | 待同步票 → 建议状态 |
| --- | --- |
| M0 | 306/308/311/314/317/318/321/324 → Done；327 → In Progress |
| M1 | 332/336/339/342/345/349/353/357/361 → Done |
| M2 | 366/369/372/376/380/383/388/390/396/399/407/416 → Done；404/412 → In Progress |
| M3 | 433/438/446/450/454 → Done（446/450 若已 In Progress 则改 Done） |
| M4 | 459/467/471/473/477/479 → Done |
| M5 | 501/503/505/509/516/519 → Done；513 → In Progress |
| M6 | 484/491/495/497 → Done；488 → In Progress |

> 契约票（433/438/459/484/501）标记 Done 的前提是实现与回归齐备；按流程仍建议补一轮 Review·SOTA 复核。

## 遗留与风险（跨里程碑）

1. **sync wire v1 只支持 note 实体**：会话/配置的跨端离线同步需 sync 契约 v2，本轮明确不做。
2. **G1 观察窗未启动**：M3–M6 均为按所有者指示提前实现，G1（INH-429）仍是契约票的正式阻塞项。
3. **真机与公网**：安卓实机验收、公网 HTTPS、跨网络同步仍未验证（沿用既有记录）。
4. **导入器真实样本**：chatgpt/codex fixtures 基于公开格式整理，真实账号大文件导出验证属 AI·H-External 证据。
5. **WebSocket 与多实例**：change hints 当前为单实例内存 SSE；WS 升级与跨实例 hub 为收口项。

## 相关文档

- [CONTRACTS.md](CONTRACTS.md)（M3–M6 契约冻结章节）
- [M3_M4 实施记录](M3_M4_IMPLEMENTATION_20260929_ZH.md) / [M5_M6 实施记录](M5_M6_IMPLEMENTATION_20260929_ZH.md)
- [M1/M2 实施记录](M1_M2_IMPLEMENTATION_20260928_ZH.md) / [M0 验证记录](VALIDATION.md)
