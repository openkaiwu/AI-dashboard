# AI Hub 完成度与设计文档（2026-09-30）

本文是截至 2026-09-30 的**完整设计说明与完成度记录**，覆盖 M0–M6 全部里程碑、G1/R3 地基，以及每张 Linear 票的实现细节、关键文件与验证证据。配合文档：架构契约见 [CONTRACTS.md](CONTRACTS.md)；运维手册见 [OPERATIONS_ZH.md](OPERATIONS_ZH.md)；历史轮次记录见 [M1_M2](M1_M2_IMPLEMENTATION_20260928_ZH.md)/[M3_M4](M3_M4_IMPLEMENTATION_20260929_ZH.md)/[M5_M6](M5_M6_IMPLEMENTATION_20260929_ZH.md) 实施记录与 [PROGRESS](PROGRESS_20260929_ZH.md) 总览。

分支：`codex/m1-m2-account-binding`；最新提交：`9bd4428`；远端：github.com/openkaiwu/AI-dashboard。

---

## 1. 系统组成与架构基线

| 组件 | 技术 | 规模 | 职责 |
| --- | --- | --- | --- |
| server | Go 1.24 单模块（stdlib net/http + pgx/PostgreSQL） | 约 8,500 行 + 19 个 SQL 迁移 | 唯一事实源；REST API；迁移；采集上传；规则引擎；telemetry |
| apps/web | React 19 + Vite 7 | 约 2,300 行 | 管理台 + 业务 UI（额度/提醒/会话/配置/协作/情报） |
| apps/mobile | Flutter（drift/sqlite3 + secure storage） | 约 3,400 行 | 移动客户端；离线便笺同步；本地通知 |
| apps/desktop | Electron 44（沙箱渲染 + 托管 Go 进程） | 约 450 行 | 桌面外壳；托盘；桥接生命周期 |
| server/cmd/aihub-bridge | Go | 约 900 行 | 桌面采集器：Codex/Cursor/official/config-scan 四条采集线 |
| extension | Chrome MV3 | 约 40 行 | 固定页面脱敏样本 PoC（回环 + 配对码） |
| deploy/selfhost | systemd / Compose / Dockerfile | 模板 | 自部署打包（R3/INH-542） |

**关键 ADR（已实现并验证）**：Flutter 首发；Go 模块化单体；PostgreSQL 唯一事实源；SQLite/Drift local-first；capability-based Connector；Event Log + 增量同步（非全局 CRDT）；Secrets 与配置分离；无公开 API 的 Provider 走 Bridge/Extension/Import/Manual；外部事实显式 Provenance + Confidence；append-only 会话图；配置版本只追加。

## 2. 数据模型（19 个迁移）

| 迁移 | 内容 |
| --- | --- |
| 001–002 | 用户/设备/会话/provider 账户/额度/便笺同步/任务队列基础 |
| 003–008 | Codex 快照/ advisor /连接器/设备绑定/套餐/快照到达 |
| 009–011 | 手工快照幂等操作、连接器样本、账单事件 |
| 012 | 通知动作（已读/稍后/忽略） |
| 013 | **M3**：projects/conversations/branches/messages/imports/raw_snapshots |
| 014 | **M4**：config_assets/config_versions/config_bindings/config_discoveries |
| 015 | **M5**：workspaces/members/invites/comments + 资源共享列 |
| 016 | **M6**：promotion_sources/promotions/observations/watchlists/notifications |
| 017 | M3 导入 warnings |
| 018 | M1 账单完整字段（invoice_ref/plan_code/period） |
| 019 | M5 workspace_events 活动流 |

## 3. API 面（按域，均为 `/api/v1` 前缀）

- **认证/账户**：login/refresh/logout、admin/users CRUD + status/password/devices/unbind（web_admin 会话不占设备名额）
- **同步**：sync/push（幂等 + 冲突）、sync/pull（cursor 分页）
- **额度**：provider-accounts CRUD、quota-buckets/manual-snapshot（幂等）、snapshots 历史、dashboard
- **Codex/Cursor 采集**：codex/snapshot、cursor/snapshot、codex/bridges、codex/overview、plan、preferences、alerts
- **M3 会话**：projects CRUD、conversations/import（三来源，202 异步）、conversations CRUD + export（archive/markdown/jsonl）、imports 状态、imports/raw
- **M4 配置**：config-assets CRUD/versions/diff/rollback/transform/bindings、config-assets/import（claude_desktop/codex_cli）
- **M5 协作**：workspaces、members、invites（+invites/mine）、workspace-comments、workspaces/{id}/events、change-hints（SSE）
- **M6 情报**：promotion-sources CRUD/ingest、promotions/submit、promotions feed、promotion-watchlist CRUD
- **G1/R3**：admin/telemetry（观察报表）、admin/operations（运维快照）
- **采集**：official/snapshot（official_api）、connectors/sample（固定页 PoC）、bridge/config-scan

## 4. 逐里程碑完成度明细

### M0 · Foundation & Local-first Sync（10 张）

| 票 | 实现内容 | 关键位置 | 状态 |
| --- | --- | --- | --- |
| INH-303 仓库边界 | check-boundaries.py 模块/表所有权检查（后扩展 read-grants 机制） | scripts/check-boundaries.py | Done |
| INH-306 Go skeleton | cmd/aihub + config + slog + httpx 中间件链 | server/cmd/aihub | Done |
| INH-308 PG 迁移/事务/jobs | db.Migrate（单事务+advisory lock）、jobs 租约队列 | server/internal/db, internal/jobs | Done |
| INH-311 Auth 契约 | bcrypt、access 15min/refresh 30d、SHA-256 哈希存储、刷新轮换+重用撤销 | server/internal/auth | Done |
| INH-314 账户/凭据/撤销 | 管理员创建/禁用/重置/解绑；解绑级联撤销 Bridge | server/internal/auth/admin.go | Done |
| INH-317 Flutter shell | Drift 本地库 + Server Profile + secure storage | apps/mobile | Done |
| INH-318 Sync 契约 | protocol=1、operation 幂等、base_version 冲突、cursor 分页 | docs/CONTRACTS.md | Done |
| INH-321 Sync 服务端 | sync_events/streams/applied_operations、push/pull | server/internal/sync | Done |
| INH-324 Flutter 同步状态机 | pending_ops、离线恢复、冲突解决 UI | apps/mobile/lib/sync_engine.dart | Done |
| INH-327 M0/Gate | 回归矩阵：两设备收敛、重试幂等、冲突、分页、进程重启、租户隔离 | internal/api/foundation_test.go | 实现齐备（看板 In Progress） |

### M1 · Quota & Reminder MVP（9 张）

| 票 | 实现内容 | 关键位置 | 状态 |
| --- | --- | --- | --- |
| INH-332 Quota 契约 | QuotaBucket/UsageSnapshot/Entitlement 语义 | CONTRACTS.md | Done |
| INH-336 Registry/API | provider-accounts CRUD + entitlements | internal/api/accounts.go | Done |
| INH-339 持久化 | 快照/历史 + **账单完整字段**（invoice_ref/plan_code/period，迁移 018） | internal/api/billing.go, dashboard.go | Done |
| INH-342 手工录入 | manual-snapshot 幂等（operation_id+request_hash，重放/409） | internal/api/dashboard.go | Done |
| INH-345 Reminder 契约 | 规则类型/时间语义/去重/抑制 | internal/notification | Done |
| INH-349 Rule Engine | 5 类默认规则 + 逐用户求值循环 + 去重状态 | internal/notification/engine.go | Done |
| INH-353 通知 Inbox | 通知表 + read/snooze/dismiss + **安卓本地通知**（flutter_local_notifications 22.x，强提醒进系统托盘） | api/notify.go, apps/mobile/lib/local_notifications.dart | Done |
| INH-357 Dashboard | 额度总览/状态计算（stale 不推断为零） | api/dashboard.go, web Dashboard | Done |
| INH-361 M1/Gate | **纵切 E2E**：手工额度→规则求值→通知→历史（跨设备+重放幂等） | internal/api/gates_test.go | Done（回归 PASS） |

### M2 · Connector Framework（14 张）

| 票 | 实现内容 | 关键位置 | 状态 |
| --- | --- | --- | --- |
| INH-366 Connector 契约 | Manifest/Capability/UploadPath/Provenance | internal/provider | Done |
| INH-369 SDK/Registry | provider.LocalConnectors() 注册 + /api/v1/connectors | internal/provider | Done |
| INH-372 Runner | 桥接上传回退重试（指数退避，401 退出由桌面重启） | cmd/aihub-bridge | Done |
| INH-376 Scheduler/新鲜度 | 每分钟循环 + 失败退避 + staleness 语义 | internal/codex/advisor | Done |
| INH-380 Extension 契约 | 数据最小化：固定页脱敏数值、无 cookie | extension/ | Done |
| INH-383 Extension PoC | MV3 + 回环 47831 + 配对码 + 常量时间比较 | cmd/aihub-bridge/extension.go | Done |
| INH-388 Bridge 契约 | 配对/文件权限/Secret 边界 | CONTRACTS.md | Done |
| INH-390 Local Bridge | 采集循环/health/退避/-probe 模式 | cmd/aihub-bridge/main.go | Done |
| INH-396 Spike | Codex/Cursor 采集可行性（app-server JSON + api2） | internal/codex, internal/cursor | Done |
| INH-399 official_api | **official 连接器全链路**：模型校验 + 上传端点 + provider 自动登记 + 桥接可注入 fetch 采集 | internal/official, connector/official.go, cmd/aihub-bridge/officialapi.go | Done |
| INH-407 file_import/manual | 手工快照 source_type + CSV/JSON 导入预览（Web） | api/dashboard.go, web FileImport | Done |
| INH-412 回放框架 | 会话导入 golden fixtures + API 回归（连接器级回放待扩展） | internal/api/testdata | 部分完成 |
| INH-416 M2/Gate | **纵切 E2E**：三种采集模式进统一额度模型 | internal/api/gates_test.go | Done（回归 PASS） |
| INH-404 official_web_ui | 真实站点提取器 | — | 未开始（需真实站点） |

### M3 · Conversation Portability（7 张）

| 票 | 实现内容 | 关键位置 | 状态 |
| --- | --- | --- | --- |
| INH-433 Canonical 契约 | append-only 图 + archive v1 信封（迁移 013） | internal/conversation/model.go | Done（建议 SOTA 复核） |
| INH-438 Import 契约 | 900KiB/50 会话/20 分支/4000 消息限额、单事务、raw 保留、去重身份 | CONTRACTS.md | Done（建议 SOTA 复核） |
| INH-442 异步管道 | jobs（conversation_import）+ worker ticker；object storage 未含 | internal/conversation/service.go | 部分完成 |
| INH-446 首个导入器 | chatgpt_export 树形（分支重建）+ **warnings 全程记录**（元数据节点/空内容/截断分支/坏行）+ golden fixtures | internal/conversation/importers.go | Done |
| INH-450 第二/三导入器 | codex_cli_jsonl（容忍外部行 + 逐行警告）；archive round-trip；更多 Provider 待真实样本 | 同上 | Done（v1 范围） |
| INH-454 会话浏览器 | Web 列表/分支/详情/项目归属/**内容搜索**（标题+消息 ILIKE） | apps/web/src/pages/Conversations.tsx | Done |
| INH-455 Gate | Archive/Markdown/**JSONL** 导出 + round-trip 去重回归；附件包含策略未做 | archive.go + 测试 | 部分完成 |

### M4 · Portable Config（7 张）

| 票 | 实现内容 | 关键位置 | 状态 |
| --- | --- | --- | --- |
| INH-459 Config 契约 | canonical 三类资产、SecretRef（键名分类、值即弃）、loss_report | internal/config/model.go | Done（建议 SOTA 复核） |
| INH-463 持久化 | assets/versions（只追加）/bindings + API + 管理台展示；sync wire 扩展留 v2 | internal/config/service.go | Done |
| INH-467 Bridge 配置发现 | scan_directories 显式授权、白名单文件名、深度/数量/大小限额、**只上传密钥键名** | cmd/aihub-bridge/configscan.go | Done |
| INH-471 Reference Transform | claude_desktop → canonical → codex_cli（golden 断言） | internal/config/transform.go | Done |
| INH-473 反向 Transformer | TOML 解析器（${NAME}→secret_ref 回映射）+ canonical→claude_desktop 输出 + 往返测试 | transform.go | Done |
| INH-477 Diff/Rollback/UI | 语义 JSON diff、回滚=追加旧内容新版本、Web 版本对比/转换预览/损失展示 | diff.go, ConfigAssets.tsx | Done |
| INH-479 Gate | secret 泄漏回归（全响应扫描）/rollback/round-trip 测试 | internal/api/config_test.go | Done（回归 PASS） |

### M5 · Workspace & Collaboration（7 张）

| 票 | 实现内容 | 关键位置 | 状态 |
| --- | --- | --- | --- |
| INH-501 Workspace 契约 | 角色三级、共享域仅限 canonical 资产、Provider 账号不共享 | internal/workspace, CONTRACTS.md | Done（建议 SOTA 复核） |
| INH-503 成员/邀请 | 按邮箱邀请、匹配账户接受、即时撤销、invites/mine | workspace/service.go | Done |
| INH-505 ACL 执行 | ReadableScope 统一读取片段 + 写权限仅资源所有者 + 隔离/越权测试 | workspace + conversation/config 接入 | Done |
| INH-509 分享/评论 | 资源绑定 + 评论/mention + **workspace_events 活动流**（share/comment/member/branch 事件）+ Feed API | workspace/service.go | Done |
| INH-513 Change hints | SSE（Last-Event-ID 重放、256 条缓冲、心跳）；WebSocket 升级留收口 | workspace/hub.go | 部分完成 |
| INH-516 协作 UI | 工作区页：成员/邀请/接受/移除/评论 + 角色与权限说明 | apps/web/src/pages/Workspaces.tsx | Done |
| INH-519 M5/Gate | ACL/隔离/撤销即时生效/SSE 重连重放/审计 纵切测试 | internal/api/workspace_test.go | Done（回归 PASS） |

### M6 · Promotion Intelligence（5 张）

| 票 | 实现内容 | 关键位置 | 状态 |
| --- | --- | --- | --- |
| INH-484 Promotion 契约 | 内容指纹（URL 归一去追踪参数）、时间精度三档（unknown 不伪造）、来源可信度 | internal/promotion/model.go, CONTRACTS.md | Done（建议 SOTA 复核） |
| INH-488 Source Registry | 来源白名单（5 类）+ 管理员注册 + RSS/Atom 适配器（可注入 fetch）+ 用户提交；官方站点解析器待真实证据 | promotion/service.go, rss.go | 部分完成 |
| INH-491 归一/去重/过期 | 跨来源同活动单条 + 逐来源观察计数、watchlist 匹配（空=通配）、提交即判过期 + ticker 扫描 | promotion/service.go | Done |
| INH-495 Watchlist/Feed | 订阅 CRUD + 情报流（按状态/Provider 过滤）+ 提交表单 + 来源展示（可信度渲染） | Web Promotions.tsx | Done |
| INH-497 M6/Gate | Watch→dedup→通知纵切（每用户每活动至多一条，PK 保证）+ RSS 去重回归 | internal/api/promotion_test.go | Done（回归 PASS） |

### G1/R3 地基（2026-09-30 新增）

| 票 | 实现内容 | 关键位置 | 状态 |
| --- | --- | --- | --- |
| INH-421 telemetry | 只读聚合观察报表（四条采集线新鲜度、额度 3 小时新鲜度、五类失败 taxonomy、12 项量级）+ 管理员端点 + 管理台区块 | internal/telemetry/telemetry.go | Done |
| INH-543 运维页地基 | admin/operations：DB 可达性/迁移版本/任务队列/升级预检/备份状态 | telemetry.Operations + server.go | 部分完成（页面展示已做） |
| INH-541 Critical E2E | Sync（冲突→解决）→Quota（连接器+手工）→Assets（会话+配置）→ACL（共享/越权）→Telemetry/Operations 全链一条测试 | internal/api/gates_test.go TestR3CriticalE2EChain | 部分完成（链已通，套件扩展留收口） |

## 5. 契约冻结清单（CONTRACTS.md）

| 契约 | 冻结内容 |
| --- | --- |
| M0 contracts v1 | 模块所有权、auth/sync 安全边界、sync wire v1、jobs at-least-once |
| Conversation portability v1 | canonical 图、去重身份、三来源、限额、raw 保留、异步管道 |
| Portable config v1 | 三类资产、只追加版本、SecretRef（键名分类）、双平台 transform、绑定声明式 |
| Boundary read-grants | 写入锁 owner；读取走显式 read_grants 白名单（workspace_members→conversation/config；promotions 等→telemetry） |
| Workspace & collaboration v1 | 角色语义、共享域边界、邀请流程、SSE 语义 |
| Promotion intelligence v1 | 活动指纹、时间精度三档、来源可信度、去重保证 |

## 6. 测试与验证矩阵

### 自动化测试（全部连接真实 PostgreSQL 14，WSL Ubuntu-22.04）

| 套件 | 数量 | 覆盖 |
| --- | --- | --- |
| internal/api（13 个测试文件） | 30+ 用例 | M0 同步矩阵、M1/M2 Gate 纵切、M3 round-trip、M4 secret 卫生、M5 ACL、M6 去重、R3 恢复演练 + 关键链 |
| internal/db | 1 | 升级预检版本拒绝 |
| internal/codex / cursor / jobs / notification / quota | 单元 | 采集校验、规则求值、租约、状态计算 |
| Web vitest | 9 | 预算/强提醒/认证/同步 |
| Flutter | 12（gate 内含 2 条真实服务器联测） | 同步/雷达/规则预览/手工契约 |

### 官方 Gate（scripts/gate.sh，最近一次 2026-09-30）

```
边界检查（含 read_grants）→ Web tsc+vitest+build → Go test -race（7 包）
→ 真实 PostgreSQL 服务端 → Flutter pub get + analyze + test
→ M0 PostgreSQL + Drift + real HTTP gate PASS
```

### 脚本

- `scripts/check-boundaries.py`：模块导入 + 表写读所有权（read_grants）
- `scripts/backup.sh` / `restore.sh`：备份/恢复（--clean 替换式）
- `scripts/security-audit.sh`：13 项自动化安全检查（当前 PASS）

## 7. 部署与运维（R3）

- `deploy/selfhost/`：aihub.service（systemd，含加固）、docker-compose.yml（可选）、Dockerfile、aihub.env.example
- `docs/OPERATIONS_ZH.md`：安装/升级（Preflight→迁移→版本拒绝）/备份恢复/观测/安全审计/Release Checklist
- 升级路径：Preflight 拒绝旧二进制触碰新 schema；迁移单事务原子；`AIHUB_MIGRATE_ONLY=1` 支持先迁移后上线

## 8. Linear 看板同步状态

已验证标注：INH-303、421（Done）、543/541（In Progress）、442/446/450/455/327/404/412/488/513（In Progress）、433/438/459 等多张（批量标记，部分经截图验证）。

**待同步清单**（建议状态）：

| 里程碑 | 票 → 状态 |
| --- | --- |
| M0 | 306/308/311/314/317/318/321/324 → Done；327 → In Progress |
| M1 | 332/336/339/342/345/349/353/357/361 → Done |
| M2 | 366/369/372/376/380/383/388/390/396/399/407/416 → Done；404/412 → In Progress |
| M3/M4/M5/M6 | 433/438/446/450/454/459/467/471/473/477/479/484/491/495/497/501/503/505/509/516/519 → Done；442/455/488/513 → In Progress |

> 同步方式：看板上按里程碑过滤 → 行首复选框全选 → 批量拖状态（每里程碑一两次批量操作）。

## 9. 遗留与风险

| 编号 | 项 | 影响 |
| --- | --- | --- |
| L1 | sync wire v1 仅支持 note 实体 | 会话/配置跨端离线同步需 sync 契约 v2（payload 泛化 + 客户端过滤） |
| L2 | G1 观察窗未启动 | INH-425 两周观察 + INH-429 review 是 M3+ 正式放量的唯一流程阻塞 |
| L3 | 真机/公网未验证 | 安卓实机、公网 HTTPS、跨网络同步（沿用既有记录） |
| L4 | 导入器真实样本 | chatgpt/codex fixtures 基于公开格式；真实账号大文件验证属外部证据 |
| L5 | change hints 单实例内存实现 | 多实例部署需 hub 外置；WebSocket 升级为传输替换 |
| L6 | M3 附件策略 / object storage | INH-455 附件包含策略、INH-442 存储抽象未做 |
| L7 | R3 Human Gate | INH-546 清洁安装验收需真实机器按运维手册执行 |

## 10. 建议下一步

1. **看板同步**（上表，约 5 分钟手动批量操作）。
2. **契约 SOTA 复核**：433/438/459/484/501 五张 Contract·Freeze 票。
3. **INH-425 观察窗启动**：telemetry 报表已就位，部署后即开始积累两周证据。
4. **R3 收尾**：INH-546 清洁安装验收（唯一 Human Gate）；541/543 按发布需要扩展。
5. **sync 契约 v2 立项**：解锁会话/配置离线同步（M3/M4 的最后一块）。
