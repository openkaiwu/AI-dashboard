# AI 聚合平台开发路线图

> 文档版本：v0.1  
> 基线日期：2026-08-22  
> 架构基线：[AI 聚合平台技术架构设计书](./AI_Aggregation_Platform_Technical_Architecture.md)  
> 本文定义开发顺序、里程碑、交付物、验收条件与风险退出条件。所有 `ARCH-*`、`ADR-*` 引用均来自架构设计书。

---

## 0. 路线图总原则

路线图按“先验证最危险假设，再做最有价值闭环，再扩展数据资产”的顺序推进。

不应从“统一聊天 UI”起步，因为该方向竞争激烈，而且无法验证本产品最独特的价值。第一阶段必须证明：不同平台的额度/套餐信息可以被稳定统一、可信追踪，并能通过自部署服务和 App 形成高质量提醒。

第二阶段才验证 Connector Framework 是否真的能覆盖 API 不一致的平台。只有 Connector 体系稳定后，对话迁移、项目迁移、Skill/MCP/插件迁移才值得扩展。

整个 v1 建议以单人全职或一名主开发 + AI 辅助为估算基准。完整 v1 约 20~28 个有效开发周；如果只做“额度 + 提醒 + 少量 Provider”，8~12 周可以形成可公开测试的产品。

---

## 1. 里程碑关系

```text
M0 架构骨架
 │
 ▼
M1 额度追踪 MVP
 │
 ▼
M2 Connector Framework
 │
 ├─────────────┐
 ▼             ▼
M3 对话/项目迁移   M6 优惠情报
 │
 ▼
M4 配置迁移
 │
 ▼
M5 多人协作
 │
 ▼
M7 多端与生态
```

M1 是产品价值验证点。

M2 是平台扩展能力验证点。

M3/M4 是“AI 工作资产中枢”定位验证点。

M5/M7 不应在前三项未稳定前投入大量开发。

---

## 2. Release 分层

建议将产品划分为四个外部版本。

```text
R0 Internal Prototype
M0 + 部分 M1

R1 Private Alpha
M1 + M2

R2 Public Beta
M3 + M4 + M6

R3 v1.0
M5 + M7 的核心部分
```

---

## 3. M0：架构骨架与技术 PoC

建议周期：1~2 周。

目标不是做漂亮 UI，而是验证 `ADR-001` 到 `ADR-006` 可以一起工作。

### 3.1 服务端骨架

对应：`ARCH-S01`、`ARCH-S02`、`ARCH-S03`、`ARCH-D01`。

完成 Go 项目骨架、配置系统、数据库迁移、结构化日志、Health Check 和基本 Auth。

最低 API：

```text
POST /auth/register
POST /auth/login
GET  /me
POST /devices
POST /sync/push
GET  /sync/pull
```

### 3.2 App 骨架

对应：`ARCH-C01`、`ARCH-D03`。

完成 Flutter 项目、登录页、Server Profile、本地 SQLite/Drift、Repository Layer 与同步状态机。

App 必须支持配置自部署地址，例如：

```text
https://aihub.example.com
https://192.168.x.x
```

本地开发环境可以允许 HTTP，但 Release 默认要求 HTTPS。

### 3.3 Sync PoC

实现一个最小实体 `note` 或 `notification_rule`，验证：

```text
Device A offline edit
Device A push
Server assign seq
Device B pull
Device B update
Device A pull
```

必须测试重复 Push 不会重复执行。

### 3.4 M0 验收

M0 完成标准：

```text
[ ] Server 可以单二进制运行
[ ] PostgreSQL migration 可自动初始化
[ ] App 可以连接不同 Server Profile
[ ] 两台模拟设备能完成增量同步
[ ] sync cursor 重启后不会丢失
[ ] 所有写请求有 request_id
[ ] 基础 CI 能执行 unit + integration test
```

### 3.5 M0 不做

不做正式 Provider，不做推送通知，不做复杂 Workspace，不做聊天 UI。

---

## 4. M1：额度追踪 MVP

建议周期：3~4 周。

这是第一条真正产品闭环。

对应架构：`ARCH-S10`、`ARCH-S11`、`ARCH-S12`、`ARCH-S17`。

### 4.1 Provider Registry

先建立 Provider Registry，但不急着支持很多平台。

首期只需要：

```text
Provider
Provider Capability
Provider Account
Entitlement
Quota Bucket
Usage Snapshot
Billing Event
```

提供管理接口和 Seed Data。

### 4.2 手工额度录入

必须先支持手工录入，因为它是所有 Provider 的最终 fallback。

用户可以创建：

```text
平台
套餐
总额度
剩余额度
重置时间
过期时间
刷新备注
```

手工数据必须标记 `source_type = user_manual`。

### 4.3 额度 Dashboard

App 首屏建议优先展示“状态”，而不是传统 Provider 列表。

每个账户卡片至少显示：

```text
Provider
Plan
Remaining
Reset / Expire
Last Updated
Source
```

状态需要统一计算：

```text
healthy
low
reset_soon_unused
expire_soon_unused
stale
unknown
```

### 4.4 Reminder Rule Engine

完成至少五类规则：

```text
低额度
即将重置但剩余过多
即将过期但仍有剩余
即将续费
数据过久未刷新
```

加入 Rule Preview，用户编辑规则时显示：

```text
如果按当前数据求值：
将触发 / 不会触发
预计触发时间
```

### 4.5 通知

Android 首先实现系统本地通知 + Server Notification Inbox。

iOS 推送可以在 Alpha 后补齐，但数据库模型从第一版就区分：

```text
notification_event
delivery_attempt
delivery_channel
```

### 4.6 历史曲线

保存 Usage Snapshot，提供最近 7/30 天趋势。

不需要第一版做复杂预测，只需要显示消耗速度和历史重置点。

### 4.7 M1 验收

```text
[ ] 可以添加至少 5 个手工 Provider Account
[ ] 每个 Account 可以有多个 Quota Bucket
[ ] Reset 与 Expire 可以分别建模
[ ] 一次额度变化会立即重新求值提醒
[ ] 时间型规则在无新快照时仍可触发
[ ] 同一提醒不会重复轰炸
[ ] App 离线仍可查看最后快照
[ ] 数据页面能显示来源和更新时间
```

达到这里即可发布 `R0 Internal Prototype`。

---

## 5. M2：Connector Framework

建议周期：4~5 周。

这是整个项目最关键的技术阶段。

对应：`ARCH-W01`、`ARCH-W02`、`ARCH-C03`、`ARCH-C04`、`ADR-005`、`ADR-008`、`ADR-010`。

### 5.1 Connector SDK

先实现 Manifest、Capability、Result、Error、Provenance 五个核心协议。

伪接口：

```go
type Connector interface {
    Manifest() Manifest
    Execute(ctx context.Context, req Request) (Result, error)
}
```

Result 中禁止直接写数据库，只返回规范化对象，由领域服务验证后写入。

### 5.2 Connector Runner

Runner 负责：

```text
timeout
retry
backoff
rate limit
run log
result validation
provenance
metrics
```

Connector 本身不负责这些横切逻辑。

### 5.3 第一批 Provider

只选 2~3 个“获取机制明显不同”的 Provider，用来验证框架，而不是追求数量。

理想组合：

```text
Provider A：官方 API 可取额度
Provider B：只能在网页 UI 看到额度
Provider C：主要依靠手工/导出文件
```

如果三个都能稳定进入同一 Quota Model，框架才算通过。

### 5.4 Browser Extension PoC

扩展必须实现：

```text
site permission whitelist
content script
structured extractor
signed device auth
manual sync button
last extraction status
```

第一版不要做自动模拟点击或复杂浏览器自动化。

### 5.5 Local Bridge PoC

Bridge 首先只实现本地配置文件发现和上报，不碰高风险凭据。

接口示例：

```text
GET /health
GET /capabilities
POST /scan
POST /sync-result
```

Bridge 与 App/Server 通过一次性配对码或 Device Token 绑定。

### 5.6 Scheduler

根据 Provider 特性允许不同刷新频率：

```text
5 min
15 min
1 h
6 h
manual
```

若 Provider 数据源容易触发风控，默认必须低频，并允许 Connector 声明最短刷新周期。

### 5.7 Connector 可维护性测试

为每个网页类 Connector 保存匿名化 Fixture。

解析器应可以在 CI 中对 Fixture 回放，不依赖实时网站。

### 5.8 M2 验收

```text
[ ] 至少 3 种 acquisition_mode 跑通
[ ] Connector 故障不影响 API 进程
[ ] Connector 有统一 timeout/retry
[ ] 任何结果都可追溯 connector_run_id
[ ] 浏览器扩展不会上传 Cookie
[ ] 网页结构变化能在 Fixture Test 中暴露
[ ] 数据过期会自动标记 stale
[ ] Provider Capability Matrix 可在 App 中展示
```

达到这里即可进入 `R1 Private Alpha`。

---

## 6. Private Alpha 验证窗口

建议至少观察 2 周真实使用，不需要停止开发，但 M3/M4 的范围应根据这里的数据调整。

重点收集：

```text
每个 Provider 每周 Connector 失败次数
额度数据与网页实际值偏差
Reset 时间错误率
通知误报率
通知重复率
用户手工修正次数
用户最常看的 Dashboard 字段
```

退出条件建议：

```text
Connector 成功率 > 95%
核心 Provider quota freshness 达标
没有高频重复通知
没有凭据泄漏
用户愿意保留通知权限
```

如果这些指标不达标，应优先修 M2，不要急着扩展聊天迁移。

---

## 7. M3：对话与项目迁移

建议周期：3~4 周。

对应：`ARCH-S14`、`ADR-006`。

### 7.1 Canonical Conversation Schema

先冻结 v1 schema：

```text
Project
Conversation
Branch
Message
Message Revision
Attachment Metadata
Provider Origin
Raw Snapshot Ref
```

必须支持 Provider 不具备分支概念的情况。

### 7.2 第一批 Importer

选择真实使用量最高的 2~3 种导出格式。

Import Pipeline：

```text
upload
detect
parse
normalize
validate
deduplicate
persist
index
```

Import Job 必须是异步任务。

### 7.3 Conversation Browser

App 中只做“浏览与搜索”，不要一开始做新的统一 AI Chat Composer。

核心视图：

```text
Project list
Conversation list
Branch tree
Message timeline
Origin Provider
Imported At
```

### 7.4 Export

实现平台无关的 AI Conversation Archive。

至少支持：

```text
Canonical ZIP
Markdown
JSONL
```

### 7.5 跨平台映射报告

每次导出到目标 Provider 格式时返回：

```text
fully_supported
partially_supported
unsupported
```

并显示损失字段。

### 7.6 M3 验收

```text
[ ] 同一对话重复导入不会大量复制
[ ] Provider Raw 被保留
[ ] 分支结构可正确表示
[ ] 附件缺失不会导致整个导入失败
[ ] 大导出包不会阻塞主 API
[ ] 导出的 Canonical Archive 可重新导入并保持核心结构
```

---

## 8. M4：Skill / MCP / Plugin / Agent 配置迁移

建议周期：3~4 周。

对应：`ARCH-S15`、`ADR-007`。

### 8.1 Portable Config v1

先只支持三类：

```text
MCP Server
Prompt Template
Agent Profile
```

Skill 与 Plugin 等平台语义更复杂，可在 schema 稳定后追加。

### 8.2 Local Bridge Scanner

Bridge 扫描用户允许的目录和已知配置文件。

任何 Scan 结果分成：

```text
Config Metadata
Canonical Config
Secret References
Unsupported Fields
```

Secret 默认永不上传明文。

### 8.3 Target Transformer

至少实现两个真实平台之间的配置转换。

例如：

```text
Source A → Canonical → Target B
Target B → Canonical → Source A
```

重点验证 Loss Report，而不是追求“全自动”。

### 8.4 Versioning

Config Asset 每次修改产生版本。

用户可以：

```text
查看 diff
恢复旧版本
复制
绑定到目标 Provider
```

### 8.5 M4 验收

```text
[ ] 任何 Config Asset 都有 schema_version
[ ] Secret 不进入普通同步 payload
[ ] 两个平台之间至少一种 MCP 配置可转换
[ ] 不支持字段会进入 loss_report
[ ] 配置变更可回滚
[ ] Bridge 扫描目录必须由用户显式授权
```

---

## 9. M5：多人协作

建议周期：3~4 周。

对应：`ARCH-S16`。

这一阶段开始前，单用户同步必须已经非常稳定。

### 9.1 Workspace

加入：

```text
Workspace
Member
Role
Invite
Resource ACL
```

首期只做四角色：Owner、Admin、Member、Viewer。

### 9.2 项目共享

优先共享 Canonical Project / Conversation，而不是共享第三方账号。

任何外部 Provider 执行都记录：

```text
executed_by
provider_account_id
provider
timestamp
```

### 9.3 评论与协作事件

先实现低冲突协作：

```text
comment
mention
branch create
tag
share
```

再实现 Presence。

### 9.4 WebSocket

服务端发送资源事件，App 使用 Sync API 获取最终数据。

WebSocket 只用于“知道有变化”，不能成为唯一事实来源。

### 9.5 M5 验收

```text
[ ] Workspace ACL 不可绕过
[ ] Viewer 无法修改资源
[ ] 成员移除后设备权限立即失效
[ ] WebSocket 断线不会导致数据丢失
[ ] Conversation 分支不会因多人创建而覆盖
[ ] 审计日志可追踪共享操作
```

---

## 10. M6：平台优惠情报

建议周期：2~3 周，可以与 M3/M4 部分并行。

对应：`ARCH-S13`。

### 10.1 Watchlist

用户选择关注：

```text
已绑定 Provider
收藏 Provider
指定计划
指定关键词
```

### 10.2 Source Registry

第一版只接入高质量源：

```text
官方公告
官方定价页
官方博客/RSS
用户转发链接
```

社区信息只作为 discovery signal。

### 10.3 Promotion Normalize

统一抽取：

```text
provider
title
type
discount
benefit
region
eligible_plan
start
end
source
confidence
```

### 10.4 Deduplication

同一活动可能同时出现在官网、社媒、社区。

建立 fingerprint：

```text
provider + normalized title + time window + benefit signature
```

### 10.5 Notification

优惠提醒复用 `ARCH-S17 Notification Rule Engine`，不新造一套通知系统。

### 10.6 M6 验收

```text
[ ] 同一优惠多来源不会重复通知
[ ] 用户可以只看已绑定 Provider
[ ] 优惠有来源与可信度
[ ] 过期优惠自动归档
[ ] 未确定时间必须显示“未知/推测”，不能伪造精确日期
```

达到 M3 + M4 + M6 后可以发布 `R2 Public Beta`。

---

## 11. M7：多端与生态

建议周期：4~6 周，且可长期演进。

### 11.1 Desktop

优先复用 Flutter 客户端。

Desktop 额外提供：

```text
Bridge 管理
拖入导出包
本地配置扫描
快速打开第三方平台
```

### 11.2 Web

Web 主要定位：

```text
团队管理
Workspace
配置查看
分享链接
服务器管理
```

不强求完全复制 App。

### 11.3 Browser Extension 产品化

加入：

```text
Provider 页面识别
Quota Extractor
Conversation Export Helper
一键发送到自部署 Server
Connector Version Update
```

### 11.4 Connector Marketplace

不要在 v1 早期开放任意代码执行。

第一版 Marketplace 可以只分发 Manifest + 声明式 Extractor；Server-side executable Connector 后续再设计签名与隔离机制。

### 11.5 Schema Registry

公开：

```text
Portable Config Schema
Conversation Archive Schema
Connector Manifest Schema
```

这样社区可以写 Importer/Exporter，而不用直接耦合服务端数据库。

---

## 12. Cross-cutting Workstream A：安全

安全工作不能等到 M7。

M0：身份认证、Password Hash、TLS 假设、基础审计。

M1：敏感字段分类。

M2：Credential Vault、Browser Extension 权限、Connector SSRF 防护。

M3：Zip Bomb、Path Traversal、恶意附件。

M4：Secret Ref 与 Config 分离。

M5：ACL 与 Invite Security。

M7：Connector Marketplace 签名与权限模型。

安全发布门槛：

```text
任何日志不得出现 token/cookie/password
Import 不允许 ../ 路径逃逸
外部 URL 访问有 allowlist / 网络策略
Connector 权限可被用户查看
设备可以单独撤销
```

---

## 13. Cross-cutting Workstream B：Schema 与 Migration

核心原则：所有可同步、可导出的资源必须版本化。

必须维护：

```text
database schema version
sync protocol version
portable config schema version
conversation archive schema version
connector manifest version
```

服务端升级前数据库自动备份或至少进行 compatibility check。

客户端需要设置 `min_supported_server_version` 与 `min_supported_client_version`，避免长期版本漂移造成静默数据损坏。

---

## 14. Cross-cutting Workstream C：测试

建议从 M0 就设置四层测试。

`Unit Test`：领域规则、Normalizer、Rule Engine。

`Integration Test`：PostgreSQL、Sync、Job、Import。

`Golden Fixture Test`：网页/导出文件 Connector。

`End-to-End`：App → Server → DB → Notification。

核心回归场景必须自动化：

```text
额度下降触发提醒
额度重置后不重复触发旧提醒
离线修改规则后同步
重复同步幂等
Connector 失败后恢复
导出后重新导入
Secret 不出现在 archive
```

---

## 15. Cross-cutting Workstream D：可观测性

M0 即接入结构化日志。

M1 增加 quota / notification 指标。

M2 增加 Connector 成功率、staleness 和执行耗时。

M3/M4 增加 import/transform loss 指标。

M5 增加 WebSocket 连接与 sync lag。

Public Beta 前至少建立一个系统健康页：

```text
Server version
DB status
Worker status
Queue backlog
Connector failures
Push status
Storage status
Last backup
```

---

## 16. Git 与仓库工作流

推荐单仓库，理由是 Schema、App、Server、Connector 之间版本耦合很强。

主分支策略保持简单：

```text
main
feature/*
fix/*
```

每个 PR 必须标注影响范围：

```text
APP
SERVER
SCHEMA
CONNECTOR
BRIDGE
EXTENSION
DOCS
```

任何修改 Canonical Schema 的 PR 必须同时包含 migration、fixture 和文档更新。

---

## 17. 推荐 Issue/Epic 切分

建议在项目管理工具中创建以下 Epic。

```text
EPIC-001 Platform Foundation
EPIC-002 Local-first Sync
EPIC-003 Provider Registry
EPIC-004 Quota & Billing
EPIC-005 Notification Engine
EPIC-006 Connector SDK
EPIC-007 Browser Extension
EPIC-008 Local Bridge
EPIC-009 Conversation Archive
EPIC-010 Portable Config
EPIC-011 Collaboration
EPIC-012 Promotion Intelligence
EPIC-013 Security
EPIC-014 Observability
EPIC-015 Deployment
```

每个 Epic 的 Definition of Done 都必须引用架构文件中的组件或 ADR。

---

## 18. 版本优先级

### P0：必须稳定，否则项目不成立

```text
自部署 Server
Auth / Device
Local-first Sync
Provider Registry
Quota Model
Reminder Rule Engine
Connector SDK
Provenance
Credential Security
```

### P1：形成差异化价值

```text
Conversation Import/Export
Project Archive
Portable Config
Local Bridge
Browser Extension
Promotion Watch
```

### P2：扩大使用场景

```text
Workspace
多人评论/分支协作
Desktop
Web
Connector Marketplace
```

### P3：长期增强

```text
可选 E2EE
智能额度预测
自动套餐优化建议
跨 Provider Agent Routing
社区 Connector 生态
```

---

## 19. 明确不应提前做的功能

不要在 M2 前做完整统一聊天 Composer。

不要在真实 Connector 数量小于 3 时设计复杂 Marketplace。

不要在单机 PostgreSQL 出现真实瓶颈前引入 Kafka/NATS/Redis 集群。

不要为了“同步”而把所有数据改成 CRDT。

不要把第三方 Cookie 上传服务端作为快捷方案。

不要把“优惠情报”做成无边界互联网爬虫。

不要先设计 50 个 Provider 的统一 Schema，再找真实数据验证。

---

## 20. 风险登记表

### RISK-001 Provider 无公开额度 API

影响：高。概率：高。

缓解：`ADR-005` + `ADR-008`。Browser Extension、Local Bridge、Import、Manual 四级 fallback。

退出条件：至少 3 类 Provider 可稳定进入统一 Quota Model。

### RISK-002 网页 Connector 易碎

影响：高。概率：高。

缓解：Fixture Replay、字段级选择器、版本化 Connector、Staleness 告警。

退出条件：结构变化能够在用户大面积报错前被 CI/健康监控发现。

### RISK-003 Reset 语义不一致

影响：高。概率：高。

缓解：`reset_policy`、`rolling_window_seconds`、`confidence`、Raw Snapshot。

### RISK-004 配置迁移丢语义

影响：中高。概率：高。

缓解：Canonical + Provider Extension + Loss Report。

### RISK-005 多人协作过早导致复杂度爆炸

影响：高。概率：中。

缓解：M5 后置；先稳定单用户 Event Log。

### RISK-006 用户不信任凭据处理

影响：极高。概率：中。

缓解：Local-only Secret 优先；权限面板；公开数据策略；Bridge。

### RISK-007 自部署升级破坏数据

影响：高。概率：中。

缓解：Schema Version、自动 Migration、升级前备份、Compatibility Check。

---

## 21. 12 周最小可发布路线

如果目标不是直接做完整 v1，而是尽快上线一个真正有用的版本，可按以下节奏压缩。

```text
Week 1-2
M0：Server / App / Auth / Sync

Week 3-5
M1：Quota / Manual / Reminder / History

Week 6-9
M2：Connector SDK + 2~3 Providers + Extension PoC

Week 10
Hardening：通知、重试、staleness、日志

Week 11
Self-host Deployment + Backup + Upgrade

Week 12
Private Alpha Release
```

这个版本已经具备独立产品价值，不需要等对话迁移和多人协作完成。

---

## 22. 20~28 周完整 v1 路线

```text
Week 1-2      M0
Week 3-6      M1
Week 7-11     M2
Week 12-15    M3
Week 16-19    M4
Week 20-22    M6
Week 23-26    M5
Week 27-28+   M7 core + release hardening
```

M3/M4/M6 可根据人力部分并行。

如果开发者只有一人，建议不要真正并行编码，只并行进行调研、Schema 设计和 Fixture 收集。

---

## 23. 每个 Milestone 的发布门槛

任何 Milestone 进入下一阶段前都执行五项检查。

```text
Architecture Check
是否违反 ADR？

Data Check
是否产生不可迁移的新 Schema？

Security Check
是否新增 Secret / 权限 / 外部输入？

Operational Check
失败后系统是否能恢复？

User Value Check
用户是否能实际感知这一阶段的价值？
```

未通过 Data/Security Check 的任务不允许以“Beta 后再修”为理由合并。

---

## 24. Definition of Done

任何领域功能的 DoD 至少包含：

```text
功能实现
数据库 migration
API contract
App state handling
error state
structured log
metrics
unit test
integration test
sync behavior
permission check
documentation
```

Connector 额外要求：

```text
manifest
fixture
provenance
timeout
retry policy
rate limit
secret policy
staleness policy
```

---

## 25. 第一阶段实际开发顺序

如果下一步立即开始编码，建议依次创建：

```text
1. monorepo 与 server skeleton
2. PostgreSQL migration framework
3. User / Device / Server Profile
4. sync_events + pending_ops
5. Flutter Drift 本地库
6. notification_rule 作为 Sync PoC
7. Provider Registry
8. Provider Account
9. Quota Bucket
10. Usage Snapshot
11. Rule Engine
12. Notification Inbox
13. Manual Quota
14. 第一版 Dashboard
15. Connector Manifest
16. Connector Runtime
17. 第一批真实 Provider
```

这个顺序与架构设计书中的 `M0 → M1 → M2` 映射完全一致。

---

## 26. v1 成功标准

技术成功不是“支持了多少 Provider”，而是以下能力形成稳定闭环：

```text
自部署简单
多设备同步可靠
额度数据可信
提醒值得开启
Connector 可维护
导入数据不丢失
配置迁移不泄密
Provider 差异被明确表达
```

产品成功的第一组建议指标：

```text
7 日活跃用户中启用提醒比例
每用户绑定 Provider 数
自动 Connector 覆盖率
Quota 数据 freshness
通知点击率
通知关闭率
Conversation Import 使用率
Config Transform 成功率
```

---

## 27. 与架构设计书的变更规则

路线图不拥有架构最终解释权。

当开发中发现需要修改以下事项时，必须先更新架构设计书的 ADR：

```text
客户端框架
服务端语言/形态
事实数据库
同步模型
Connector 权限模型
Secret 存储方式
Canonical Schema 的根结构
```

普通功能范围、时间估算、Provider 顺序可以只修改路线图。

两份文档因此形成明确职责：

```text
Technical Architecture
= 稳定的系统边界与技术决策

Development Roadmap
= 可调整的实施顺序与交付计划
```

---

## 28. 建议的下一动作

完成本文后，不建议立即实现所有数据表。下一步应该建立 `M0` 的真实仓库骨架，并同时做一个 2~3 天的 `Connector Feasibility Spike`：挑选三个能力差异明显的 AI 平台，只验证“额度/套餐/重置时间能通过什么方式拿到、稳定性如何、需要哪些权限”。

该 Spike 的结果应直接回写技术架构设计书中的 Provider Capability Matrix，并决定 M2 第一批 Connector 的真实顺序。

架构细节、核心数据结构、同步协议和 ADR 见配套文档：[AI 聚合平台技术架构设计书](./AI_Aggregation_Platform_Technical_Architecture.md)。
