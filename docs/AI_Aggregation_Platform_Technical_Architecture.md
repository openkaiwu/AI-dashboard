# AI 聚合平台技术架构设计书

> 文档版本：v0.1  
> 基线日期：2026-08-22  
> 配套文档：[AI 聚合平台开发路线图](./AI_Aggregation_Platform_Development_Roadmap.md)  
> 本文定义系统“应该长成什么样”；路线图定义“按什么顺序把它做出来”。两份文档通过 `ARCH-*`、`ADR-*`、`M*`、`R*` 编号互相引用。

---

## 0. 执行摘要

本项目不是一个“统一调用多家模型 API 的聊天壳”，而是一个面向 AI 重度用户的 **AI 服务资产与工作流聚合中枢**。它需要统一管理多个 AI 平台的额度、套餐、充值与重置周期，持续跟踪优惠信息，沉淀并迁移对话、项目、Skill、MCP、插件等配置，并最终支持多人共享与协作。

技术上最重要的约束不是 UI，而是第三方平台能力高度不一致：有的平台提供官方 API，有的平台只提供网页端状态，有的平台对额度、历史对话、项目和插件配置完全没有公开接口。因此系统必须从第一天就采用 **Provider Adapter / Connector 能力模型**，允许同一平台通过官方 API、用户授权浏览器扩展、本地 Bridge、文件导入、手工录入等不同方式获取数据，同时记录数据的来源、可信度与更新时间。

本设计采用 **Local-first App + Go 模块化单体服务端 + PostgreSQL + 事件日志增量同步 + Connector Runtime**。App 首发使用 Flutter，客户端本地使用 SQLite/Drift 缓存；服务端以 PostgreSQL 为唯一事实源，初期不强依赖 Redis、Kafka、NATS 等基础设施，通过 PostgreSQL Outbox + Worker 完成异步任务。对话、项目、配置均先映射为平台无关的 Canonical Model，再保留 Provider Raw Snapshot，确保“可迁移”而不是“强行抹平平台差异”。

---

## 1. 产品边界与架构目标

### 1.1 核心目标

系统首要解决五类问题。

第一，用户可以自部署服务端，并且服务端不仅能在局域网使用，也可以安全暴露到公网；客户端优先支持移动 App，随后扩展桌面、Web 与浏览器扩展。

第二，统一跟踪不同 AI 平台的套餐、额度、充值记录、到期时间与重置时间，并能处理“额度即将耗尽”“额度即将重置但尚未用完”“本轮额度即将失效”“订阅即将续费”等场景。

第三，收集平台优惠、促销、赠送额度、周年活动、夜间折扣、学生/地区政策等信息，并允许用户只关注自己已使用或感兴趣的平台。

第四，将 AI 对话、项目与工作记录抽象成平台无关的数据结构，支持导入、导出、同步、共享与多人协作。

第五，将 Skill、MCP Server、插件、Agent 配置、模型偏好、Prompt Template 等视作“AI 工作环境配置资产”，支持跨平台迁移、版本化和多设备同步。

### 1.2 非目标

v1 不做模型 API 聚合计费平台，不做代理转售，不保存用户完整浏览器 Cookie，不通过服务端模拟用户登录第三方平台，不以违反第三方 ToS 的方式进行大规模抓取。

v1 也不追求把所有 Provider 的概念完全统一。Canonical Model 的作用是建立可交换的“最低公共语义层”，Provider 特有信息通过扩展字段和 Raw Snapshot 保留。

---

## 2. 架构原则

`ARCH-PRINCIPLE-01 Local-first`：客户端必须能够离线浏览最近数据、编辑提醒规则、记录手工额度和创建本地待同步操作。网络恢复后通过 Sync Engine 增量同步。

`ARCH-PRINCIPLE-02 Canonical + Raw`：任何外部数据同时保留“规范化数据”和“原始来源数据”。规范化数据用于跨平台功能，Raw Snapshot 用于回溯、重新解析和避免信息损失。

`ARCH-PRINCIPLE-03 Capability-based Connector`：不按“平台 = 固定实现”建模，而按能力建模。一个 Provider 可以支持 `quota.pull`，但不支持 `conversation.export`；另一个 Provider 可以通过浏览器扩展实现 `quota.pull`，通过官方导出包实现 `conversation.import`。

`ARCH-PRINCIPLE-04 Server-authoritative Event Log`：业务数据最终由服务端分配单调递增的事件序列号，客户端使用 cursor 增量拉取。对话本身优先采用 append-only graph，而不是在 v1 引入全量 CRDT。

`ARCH-PRINCIPLE-05 Secure by default`：第三方令牌、Cookie、API Key 与普通业务数据隔离；能不上传服务端的凭据就不上传。需要上传时必须加密，并记录 Credential Scope。

`ARCH-PRINCIPLE-06 Modular Monolith First`：单人或小团队阶段优先模块化单体，避免过早拆微服务。Connector Worker、Notification Worker 是优先可拆分的执行面。

`ARCH-PRINCIPLE-07 Observable uncertainty`：额度、重置时间、促销有效期等信息可能不完整。系统必须显式记录 `confidence`、`source_type`、`observed_at`，而不是把估算值伪装成精确值。

---

## 3. 总体逻辑架构

系统逻辑上分成五层。

### 3.1 Client Layer

`ARCH-C01 Mobile App`：Flutter App，首发 Android/iOS。承担账户视图、额度看板、提醒中心、对话/项目浏览、配置资产管理、离线缓存和设备通知。

`ARCH-C02 Future Desktop/Web`：桌面端优先复用 Flutter 业务模型；Web 管理台可单独使用轻量前端。桌面端未来还可承担 Local Bridge 的 UI。

`ARCH-C03 Browser Extension`：用于用户授权后读取第三方网页中“官方 API 不提供但 UI 可见”的信息，或者导出当前会话。扩展只采集白名单字段，默认发送结构化结果而不是完整页面 DOM。

`ARCH-C04 Local Bridge Agent`：运行在用户电脑上的轻量守护进程，负责读取本地 AI 工具配置、MCP 配置、Skill 目录、CLI 工具状态和本地导出文件。Bridge 与服务端之间使用设备级凭据认证。

### 3.2 Edge/API Layer

`ARCH-S01 API Gateway`：初期作为 Go 服务内部模块，不单独部署。负责 TLS 后的身份认证、设备认证、速率限制、幂等键和请求追踪。

`ARCH-S02 Auth & Device Service`：管理用户、Workspace、设备、设备配对、Access Token、Refresh Token、OIDC/Passkey 扩展点。

`ARCH-S03 Sync API`：提供 `/sync/push`、`/sync/pull` 与 WebSocket 事件通知。App 通过 cursor 增量获取变化。

### 3.3 Domain Service Layer

`ARCH-S10 Provider Registry`：Provider 元数据、套餐模板、能力矩阵、Connector 版本。

`ARCH-S11 Account & Entitlement Service`：第三方账户、订阅、套餐、购买记录、权益和币种信息。

`ARCH-S12 Quota Service`：额度桶、用量快照、重置策略、过期策略、预测与异常检测。

`ARCH-S13 Promotion Intelligence Service`：优惠源、优惠条目、Provider 关联、适用地区、适用套餐、可信度、用户关注。

`ARCH-S14 Conversation & Project Service`：统一存储对话、消息、分支、项目、标签、附件元数据以及 Provider 映射。

`ARCH-S15 Config Portability Service`：统一管理 Skill、MCP、Plugin、Prompt、Agent Profile 和模型偏好。

`ARCH-S16 Collaboration Service`：Workspace、成员、角色、资源授权、共享链接、评论、Presence 和协作事件。

`ARCH-S17 Notification Rule Engine`：统一的提醒规则、规则求值、抑制、去重和通知投递。

`ARCH-S18 Audit & Provenance Service`：数据来源、Connector Run、审计日志和敏感操作记录。

### 3.4 Execution Layer

`ARCH-W01 Connector Runtime`：执行 Provider Connector。支持 Server Connector、Browser Extension Connector、Local Bridge Connector、Import Connector。

`ARCH-W02 Scheduler`：按 Provider / Account 的刷新策略触发额度、订阅与优惠同步。

`ARCH-W03 Notification Worker`：根据规则求值结果生成站内通知，并调用 APNs/FCM/WebPush/邮件等通道。

`ARCH-W04 Import/Export Worker`：解析大型聊天导出包、项目压缩包和配置目录，避免阻塞 API。

### 3.5 Data Layer

`ARCH-D01 PostgreSQL`：服务端唯一事实源。承担事务数据、事件日志、Outbox、调度状态和绝大部分查询。

`ARCH-D02 Object Storage`：保存导入包、附件、Provider Raw Snapshot、大型导出文件。自部署可使用 S3 兼容存储或文件系统后端。

`ARCH-D03 SQLite/Drift`：App 本地数据库，保存可离线查看的数据副本、同步 cursor、pending operations 和本地通知状态。

`ARCH-D04 Optional Redis`：不是 MVP 前置依赖。只有在 Presence、限流或高频短期缓存成为瓶颈后再引入。

---

## 4. 推荐技术栈

### 4.1 移动端

Flutter + Dart 作为首发 App 技术栈。理由是移动端优先，但项目中长期明确需要多平台；Flutter 可以复用数据模型、网络层、同步引擎与绝大部分 UI。状态管理建议 Riverpod，路由使用 GoRouter，本地数据库使用 Drift/SQLite，网络层使用 Dio 或等价成熟库。

不建议首期使用 React Native + 多套桌面技术，因为本项目复杂度集中在数据同步与后台状态而不是高度原生 UI；减少客户端语言数量更有价值。

### 4.2 服务端

Go 模块化单体。推荐 HTTP 框架保持轻量，核心业务尽量不耦合具体 Web Framework；数据访问可使用 `pgx + sqlc` 或等价方式，优先显式 SQL 而不是重 ORM。

Go 的优势是自部署友好、单二进制、内存占用稳定、交叉编译简单、长连接和 Worker 场景成熟。对于“用户自己部署在 NAS、Linux、小主机或 VPS”的产品，比 JVM/Node 全家桶更易维护。

### 4.3 数据库与任务

PostgreSQL 作为核心数据库。异步任务在 MVP 阶段使用 `jobs` 表 + `FOR UPDATE SKIP LOCKED` 或专用轻量任务库，领域事件使用 Transactional Outbox。只有在实际吞吐要求出现后再升级到 NATS/Kafka。

这种方案让第一版部署形态保持为：

```text
App
  │ HTTPS / WebSocket
  ▼
Reverse Proxy / TLS
  ▼
ai-hub-server
  ├── API
  ├── Sync
  ├── Scheduler
  ├── Connector Worker
  └── Notification Worker
  │
  ├── PostgreSQL
  └── Object Storage (optional for MVP)
```

### 4.4 自部署

官方发行应至少提供三种方式：单二进制 + systemd、Docker/Compose 作为可选方案、以及面向 NAS/家用服务器的预构建包。产品不应强制 Docker。

公网部署建议默认放置在 Caddy/Nginx/Traefik 等反向代理之后，强制 HTTPS。对于只希望私人访问的用户，可文档化 Tailscale/WireGuard 等私网入口，但这不是系统唯一网络模式。

---

## 5. 核心数据模型

### 5.1 Provider 与账户

```text
providers
- id
- slug
- display_name
- category
- homepage
- metadata_json

provider_capabilities
- provider_id
- capability
- support_level
- acquisition_mode
- connector_version

provider_accounts
- id
- user_id
- provider_id
- display_name
- external_account_hint
- region
- status

entitlements
- id
- provider_account_id
- plan_code
- plan_name
- starts_at
- renews_at
- expires_at
- currency
- price
- source_id
```

`support_level` 建议枚举为 `native / partial / import_only / bridge_only / unsupported`。`acquisition_mode` 可以是 `official_api / browser_extension / local_bridge / file_import / manual`。

### 5.2 额度模型

额度不能只用一个 `remaining` 字段。统一抽象为 `QuotaBucket`。

```text
quota_buckets
- id
- provider_account_id
- entitlement_id
- scope_key
- quota_type
- unit
- limit_value
- reset_policy
- reset_at
- expires_at
- rolling_window_seconds
- source_id
- confidence

usage_snapshots
- id
- quota_bucket_id
- observed_at
- used_value
- remaining_value
- raw_value_json
- source_id
```

`quota_type` 示例：`token / request / credit / currency / compute_minute / message / weighted_unit / unknown`。

`reset_policy` 示例：`fixed_time / rolling_window / subscription_cycle / manual / unknown`。

对于“系统只显示还剩 23% 但不知道总额度”的情况，可以 `limit_value = NULL`，只记录 `remaining_ratio` 到扩展字段中，不强行制造绝对值。

### 5.3 充值、购买与账期

```text
billing_events
- id
- provider_account_id
- event_type
- amount
- currency
- occurred_at
- effective_until
- external_ref
- source_id
```

`event_type` 包括 `subscription_purchase / renewal / topup / bonus_credit / refund / manual_adjustment`。

### 5.4 优惠模型

```text
promotions
- id
- provider_id
- title
- summary
- promotion_type
- starts_at
- ends_at
- regions_json
- plans_json
- source_url
- source_type
- confidence
- observed_at
- status
```

同一优惠允许多个 `promotion_sources` 佐证。后续可通过去重指纹将官网公告、社区讨论、邮件通知合并成一条 Promotion。

### 5.5 对话与项目

```text
projects
- id
- workspace_id
- title
- description
- created_by
- provider_origin
- origin_external_id

conversations
- id
- project_id
- title
- provider_origin
- origin_external_id
- current_branch_id

conversation_branches
- id
- conversation_id
- parent_branch_id
- forked_from_message_id
- title

messages
- id
- conversation_id
- branch_id
- parent_message_id
- role
- content_type
- content_json
- model_ref
- created_at
- source_id
```

消息默认不可原地覆盖。编辑使用 `message_revisions` 或追加 `message.edit` 事件。这样可以自然支持平台导入、分支、协作和历史追踪。

### 5.6 AI 配置资产

统一定义 `ConfigAsset`。

```text
config_assets
- id
- workspace_id
- type
- name
- canonical_json
- version
- source_provider
- origin_external_id
- secret_policy
- created_at
- updated_at

config_bindings
- id
- config_asset_id
- provider_id
- target_type
- target_external_id
- transform_version
```

`type` 首期包括：`skill / mcp_server / plugin / prompt_template / agent_profile / model_profile / tool_profile`。

配置中的密钥不直接放在 `canonical_json`，只写 `secret_ref`。

---

## 6. Portable Config 规范

建议项目内部定义一个版本化格式，例如 `AIPortableConfig v1`。

```yaml
schema: ai-hub.dev/portable-config/v1
kind: mcp_server
metadata:
  name: godot-docs
  tags: [godot, docs]
spec:
  transport: stdio
  command: npx
  args:
    - some-package
  env:
    API_KEY:
      secret_ref: secret://godot-docs/api-key
provider_extensions:
  cursor: {}
  codex: {}
  claude_code: {}
```

迁移流程不是“复制 JSON”，而是：

```text
Provider Config
    ↓ parse
Canonical Config
    ↓ validate
AIPortableConfig
    ↓ target transform
Target Provider Config
```

每个转换器必须返回 `loss_report`，明确哪些字段无法迁移。

例如：

```json
{
  "status": "partial",
  "warnings": [
    "Target provider does not support per-tool model override",
    "Secret API_KEY was not exported"
  ]
}
```

这会成为配置迁移可信度的核心机制。

---

## 7. Connector 架构

### 7.1 能力接口

每个 Connector 声明自己的 Capability。

```text
account.discover
entitlement.pull
quota.pull
billing.pull
promotion.pull
conversation.import
conversation.export
project.import
project.export
config.import
config.export
```

Provider Registry 只负责“平台是什么”；Connector 负责“通过什么方式能做什么”。

### 7.2 四类 Connector

`Server Connector`：服务端直接使用官方 API。最可靠，优先级最高。

`Browser Extension Connector`：用户已经登录第三方平台时，由扩展从页面或前端请求结果中提取结构化信息。扩展必须显式授权站点权限，并尽量只上传目标字段。

`Local Bridge Connector`：读取本地 CLI/IDE 配置、AI Agent 工具目录、导出文件。适合 Codex CLI、Claude Code、Cursor 配置、MCP 配置等本地资产。

`Import Connector`：解析用户上传的 ZIP/JSON/HTML/Markdown/SQLite 等导出物。

### 7.3 Connector Manifest

```yaml
id: openai-web-quota
provider: openai
version: 1.0.0
runtime: browser_extension
capabilities:
  - quota.pull
permissions:
  hosts:
    - chatgpt.com
data_policy:
  sends:
    - plan_name
    - quota_remaining
    - reset_at
  never_sends:
    - session_cookie
```

### 7.4 数据来源分级

所有采集结果必须带 Provenance。

```text
source_type:
1. official_api
2. official_export
3. official_web_ui
4. official_email
5. user_manual
6. trusted_community
7. heuristic
```

系统 UI 应显示类似“官方 API · 2 分钟前”“网页读取 · 1 小时前”“用户手工 · 3 天前”，避免用户误判。

---

## 8. 同步架构

### 8.1 为什么不直接使用通用 CRDT

本项目的大多数数据是账户状态、额度快照、不可变聊天消息、配置版本和提醒规则。它们更适合服务器分配版本号的增量事件模型。首期对全部数据引入 CRDT 会增加客户端实现、服务端调试和数据修复成本。

因此采用：

```text
Local SQLite
  ├── materialized tables
  ├── pending_ops
  └── sync_state(cursor)
         │
         │ push local ops
         ▼
Server Command Handler
         │
         ├── domain tables
         └── sync_events(seq)
         │
         ▼
client pull after cursor
```

只有未来出现真正“多人同时编辑同一长文本/白板”的需求时，再在该资源类型局部引入 Automerge/Yjs 类 CRDT。

### 8.2 Sync Event

```json
{
  "seq": 183921,
  "workspace_id": "ws_x",
  "entity_type": "notification_rule",
  "entity_id": "nr_x",
  "op": "upsert",
  "version": 7,
  "changed_at": "2026-08-22T12:00:00Z",
  "payload": {}
}
```

### 8.3 Push Operation

```json
{
  "operation_id": "device_uuid:local_seq",
  "entity_type": "notification_rule",
  "entity_id": "nr_x",
  "base_version": 6,
  "operation": "update",
  "payload": {}
}
```

`operation_id` 用于幂等。服务端重复收到同一操作时不得重复执行。

### 8.4 冲突规则

账户、额度、外部平台快照属于服务端/Connector 事实，不接受客户端覆盖。

用户自定义字段使用 `base_version` 乐观并发控制。

聊天消息采用 append-only，不存在内容覆盖冲突。

配置资产修改产生新版本，冲突时保留双方版本并提示合并。

提醒规则可使用 Last-write-wins，但必须保留审计历史。

---

## 9. 额度与提醒引擎

提醒不是简单的定时器，而是规则求值系统。

### 9.1 核心规则

首期至少支持：

```text
quota.remaining_ratio < X
quota.remaining_value < X
time_until_reset < X AND remaining_ratio > Y
time_until_expire < X AND remaining_ratio > Y
time_until_renewal < X
promotion.match(provider/watchlist)
connector.stale_for > X
```

“即将重置但额度没用完”可以表示为：

```text
WHEN time_until_reset <= 12h
AND remaining_ratio >= 0.30
THEN notify
```

“额度即将过期”：

```text
WHEN time_until_expire <= 24h
AND remaining_value > 0
THEN notify
```

### 9.2 求值策略

每次 Connector 写入 `usage_snapshot` 后立即触发相关规则求值。

Scheduler 每 15~60 分钟执行一次时间型规则扫描。

Rule Engine 生成 `notification_candidates`，再经过去重、静默期和频率限制后写入 `notifications`。

同一规则必须有 `dedupe_key`，避免额度在阈值附近波动导致疯狂通知。

---

## 10. 优惠情报子系统

优惠信息来源可以分为三类：官方源、用户源、社区源。

官方源包括官网公告、定价页变更、官方博客、官方邮件。社区源可以作为“发现信号”，但默认不直接作为高可信事实。

数据流程：

```text
Source
  ↓
Fetcher / Importer
  ↓
Raw Item
  ↓
Normalize
  ↓
Deduplicate
  ↓
Provider Match
  ↓
Confidence Score
  ↓
Promotion
  ↓
User Watch Rules
```

v1 不建议先做全互联网 AI 爬虫。先支持少量高价值 Provider 的白名单源，并允许用户提交链接。等数据结构稳定后再扩展自动发现。

---

## 11. 对话/项目迁移架构

### 11.1 Canonical Conversation Package

建议定义 `AI Conversation Archive`：

```text
archive/
├── manifest.json
├── projects.jsonl
├── conversations.jsonl
├── messages.jsonl
├── configs/
├── attachments/
└── provider_raw/
```

`manifest.json` 包含 schema version、source provider、export time、hash 和扩展列表。

### 11.2 导入过程

```text
Upload / Local Import
   ↓
Format Detection
   ↓
Provider Parser
   ↓
Canonical Validation
   ↓
Deduplication
   ↓
Project/Conversation Mapping
   ↓
Persist
   ↓
Index / Search
```

Deduplication 建议综合 `origin_external_id`、message hash、时间和结构指纹，而不是仅按标题。

### 11.3 导出过程

任何 Canonical Conversation 都可以导出为平台中立包。若目标 Provider 有可写 API，再由 Provider Exporter 转换；没有 API 时，至少生成用户可读 Markdown/JSON/HTML 或目标工具可导入格式。

---

## 12. 多人协作设计

协作单位为 `Workspace`。

角色建议首期只做：Owner、Admin、Member、Viewer。

资源授权对象包括 Project、Conversation、Config Asset、Promotion Watchlist 和 Notification Rule Template。

多人聊天协作不应设计为“所有人共享一个第三方账号”。正确模式是共享平台内的 Canonical Conversation / Project，外部 Provider 调用仍由具体执行账户完成，并记录 `executed_by` 与 `provider_account_id`。

实时事件通过 WebSocket：

```text
presence.join
presence.leave
conversation.message_added
conversation.branch_created
project.updated
config.version_created
comment.added
```

Presence 可以在后期使用 Redis；MVP 允许仅在单实例内存中维护。

---

## 13. 身份认证与安全

### 13.1 系统账户

自部署默认支持本地账户。公网实例建议支持 OIDC。Passkey 可作为后续增强。

设备使用独立 Device Credential，不长期复用用户密码。

### 13.2 第三方凭据

凭据分成三类。

`Local-only Secret`：仅保存在用户设备，由 Bridge 使用。服务端只保存 Secret Reference。

`Server Secret`：必须由服务端调用第三方 API 时保存。数据库中仅保存加密密文，主密钥来自环境变量、OS Keyring 或外部 KMS。

`Ephemeral Browser Session`：浏览器扩展直接利用浏览器现有登录态完成读取，禁止上传原始 Cookie。

### 13.3 安全边界

Connector 输入属于不可信数据。Importer 必须限制压缩包大小、递归深度、文件数量和路径穿越。

Server Connector 必须防止 SSRF，Provider endpoint 应来自注册表白名单而非用户任意 URL。

所有 Connector Run 记录开始时间、结束时间、版本、结果、错误、写入实体数和 source provenance。

### 13.4 E2EE

v1 不建议强制全量端到端加密，因为会显著增加多人协作、搜索、服务端迁移和 Web 管理能力的复杂度。

可以先实现“服务端静态加密 + Secrets 独立加密”。若后续用户群明确需要零知识部署，再为 Conversation/Config Asset 引入 Workspace Key 的可选 E2EE 模式。

---

## 14. API 设计

外部 API 建议 `/api/v1` 版本化。

示例：

```text
POST   /api/v1/auth/login
POST   /api/v1/devices/pair

GET    /api/v1/providers
GET    /api/v1/provider-accounts
POST   /api/v1/provider-accounts

GET    /api/v1/quota-buckets
GET    /api/v1/quota-buckets/{id}/snapshots
POST   /api/v1/quota-buckets/{id}/manual-snapshot

GET    /api/v1/promotions
POST   /api/v1/promotion-watch-rules

GET    /api/v1/projects
GET    /api/v1/conversations/{id}
POST   /api/v1/import-jobs
POST   /api/v1/export-jobs

GET    /api/v1/config-assets
POST   /api/v1/config-assets
POST   /api/v1/config-assets/{id}/transform

POST   /api/v1/sync/push
GET    /api/v1/sync/pull?cursor=...
GET    /api/v1/events/ws
```

所有会产生写入的重试型请求应支持 `Idempotency-Key`。

分页统一使用 cursor，不依赖 offset。

---

## 15. 服务端目录建议

```text
repo/
├── apps/
│   └── mobile/
├── server/
│   ├── cmd/
│   │   └── aihub/
│   ├── internal/
│   │   ├── auth/
│   │   ├── provider/
│   │   ├── quota/
│   │   ├── billing/
│   │   ├── promotion/
│   │   ├── conversation/
│   │   ├── project/
│   │   ├── configasset/
│   │   ├── collaboration/
│   │   ├── notification/
│   │   ├── connector/
│   │   ├── sync/
│   │   └── audit/
│   ├── migrations/
│   └── api/
├── connectors/
│   ├── sdk/
│   ├── builtin/
│   └── manifests/
├── bridge/
├── extension/
├── schemas/
│   ├── portable-config/
│   └── conversation-archive/
├── docs/
└── deploy/
```

各领域模块只能通过明确接口访问其他模块，禁止直接跨模块改表。这一点是未来拆服务的前提。

---

## 16. 部署拓扑

### 16.1 个人自部署

```text
Internet
   │
 HTTPS
   │
Caddy / Nginx
   │
aihub-server
   │
PostgreSQL
   │
local filesystem / S3
```

单实例即可运行。

### 16.2 家庭局域网 + 外网访问

可以使用公网域名 + HTTPS，也可以使用 VPN Overlay。App 端 Server Profile 支持保存多个服务端，例如家庭服务器和测试服务器。

### 16.3 团队部署

当 Connector 任务与导入任务明显增加后：

```text
API Instance
Worker Instance
PostgreSQL
Object Storage
optional Redis
```

仍然不需要立即拆分领域微服务。

---

## 17. 可观测性与运维

必须从第一版建立以下指标：

```text
http_request_duration
connector_run_total
connector_run_failure_total
connector_data_staleness
quota_snapshot_total
notification_candidate_total
notification_sent_total
sync_event_lag
sync_push_conflict_total
import_job_duration
```

日志全部结构化，必须包含 `request_id`、`user_id` 或匿名主体、`connector_run_id` 等上下文，但严禁打印 Secret。

数据库备份策略至少包含每日自动备份和恢复演练。Object Storage 中的导入包可配置自动清理周期。

---

## 18. 测试策略

### 18.1 Contract Test

Connector SDK 必须提供统一测试套件。任何 Connector 发布前至少验证：

```text
manifest valid
capability declarations valid
normalization valid
error mapping valid
secret leakage test
idempotency
sample fixture replay
```

### 18.2 Sync Test

需要重点测试离线编辑、重复 push、断网重连、cursor 丢失、版本冲突、两设备同时修改。

### 18.3 Migration Golden Files

每个 Provider Importer 都维护 Golden Fixture。解析结果变更必须通过显式 schema migration 或 fixture 更新审核。

### 18.4 Reminder Time Test

额度重置涉及时区、夏令时、滚动窗口，Rule Engine 必须使用 UTC 存储，并以用户时区显示和计算用户自定义日历规则。

---

## 19. 关键 ADR

### ADR-001：Flutter 作为首发客户端

状态：Accepted。

原因：移动端优先且后续明确要求多平台；统一客户端语言能够降低 Sync Engine 与本地数据层维护成本。

### ADR-002：Go 模块化单体作为服务端

状态：Accepted。

原因：自部署、单二进制、运行资源、并发 Worker 与维护复杂度的综合平衡优于首期微服务。

### ADR-003：PostgreSQL 是唯一服务端事实源

状态：Accepted。

原因：业务一致性优先；Job、Outbox、事件序列均可在早期依赖 PostgreSQL，减少部署组件。

### ADR-004：客户端采用 SQLite/Drift Local-first Cache

状态：Accepted。

原因：额度看板和历史记录必须在网络不稳定时可访问，并为移动后台同步提供稳定状态机。

### ADR-005：Provider 使用 Capability-based Connector

状态：Accepted。

原因：第三方平台接口能力不对称，这是整个系统最重要的可扩展边界。

### ADR-006：聊天与核心业务使用 Event Log + Incremental Sync，而非全局 CRDT

状态：Accepted。

原因：绝大多数实体不需要无中心并发编辑。Append-only Conversation Graph 可以天然支持分支和历史。

### ADR-007：Secrets 与 Config Asset 分离

状态：Accepted。

原因：跨平台配置同步必须做到“配置可复制，密钥默认不可泄漏”。

### ADR-008：对无公开 API 的 Provider 使用浏览器扩展 / Local Bridge / Import

状态：Accepted。

原因：不能把产品可用性寄托在不存在的官方接口，也不能把高风险登录自动化放在服务端。

### ADR-009：微服务、Redis、消息队列不是 MVP 前置条件

状态：Accepted。

原因：先证明产品价值和 Connector 可行性，再为真实瓶颈扩展基础设施。

### ADR-010：所有外部数据必须带 Provenance 和 Confidence

状态：Accepted。

原因：额度、促销、重置周期的来源与可靠性直接影响提醒可信度。

---

## 20. MVP 物理边界

MVP 必须完整贯通以下闭环：

```text
用户部署 Server
   ↓
App 连接并登录
   ↓
添加 Provider Account
   ↓
手工 / Connector 获取额度
   ↓
写入 Quota Snapshot
   ↓
Rule Engine 判断
   ↓
App 收到提醒
   ↓
历史趋势可查看
```

只要这个闭环稳定，后续 Promotion、Conversation、Config、Collaboration 都可以复用既有 Provider、Sync、Notification 和 Provenance 基础设施。

---

## 21. 与开发路线图的耦合关系

本文中的组件由路线图逐阶段落地：

```text
M0 → ARCH-D01 / ARCH-S01 / ARCH-S02 / ARCH-S03
M1 → ARCH-S10 / ARCH-S11 / ARCH-S12 / ARCH-S17
M2 → ARCH-W01 / ARCH-W02 / ARCH-C03 / ARCH-C04
M3 → ARCH-S14 / Conversation Archive
M4 → ARCH-S15 / Portable Config
M5 → ARCH-S16 / WebSocket Collaboration
M6 → ARCH-S13 / Promotion Intelligence
M7 → Desktop/Web/Extension 产品化与生态
```

所有路线图任务在进入开发前，应确认是否违反本文 `ADR-*`。如需修改关键决策，先更新 ADR，再修改路线图。

---

## 22. 第一版最值得验证的技术风险

最高风险不是 Flutter 或 Go，而是外部 Provider 可观测性。必须尽早验证三个问题：目标平台的额度数据究竟能否稳定取得；Provider 页面/导出格式变化后 Connector 的维护成本是否可控；用户是否接受“某些 Provider 只能手动/扩展读取”的差异化体验。

第二风险是跨平台配置迁移的语义损失。Portable Config 必须从少数真实平台对开始验证，不能先设计一个过于抽象的“大一统 Schema”。

第三风险是提醒可信度。只要出现几次错误重置时间或重复通知，用户会迅速关闭通知。因此 Provenance、Staleness、去重和规则模拟器应被视为核心能力，而不是 UI 附件。

---

## 23. 架构完成定义

当以下条件满足时，可以认为 v1 架构成立：同一账号额度可通过至少两种 acquisition mode 写入统一 Quota Model；两台设备可离线修改提醒规则并稳定同步；Connector 故障不会影响主 API；对话导入后能够无损保留 Provider Raw；配置导出不会泄漏 Secret；任意外部数据都能追溯来源；单机部署只依赖 Server + PostgreSQL 即可提供核心功能。

后续实际实施顺序、时间预算、里程碑和验收门槛见配套文件：[AI 聚合平台开发路线图](./AI_Aggregation_Platform_Development_Roadmap.md)。
