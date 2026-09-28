# AI Hub：M0 开发思路与实现边界

依据：[Linear 项目](https://linear.app/inhandy/project/ai-聚合平台-ai-hub-7318f9404f33/overview)、
[Architecture Baseline v0.1](https://linear.app/inhandy/document/architecture-baseline-v01-e36985743e97)。
本次范围：M0 Foundation 优先；电脑、手机跨网络同步；不包含小米手环。用户确认先交付本地应用与部署包，公网服务器以后配置。

## 1. 两个上游如何组合

| 基底 | 使用方式 | 本次没有继承的部分 |
| --- | --- | --- |
| openkaiwu/AI-dashboard，e4c7ce94e51f68466468fe56aeb09b6162477006 | 保留 Go 额度计算、提醒规则、React 看板；将持久化改为 PostgreSQL，替换认证，增加 Web 离线工作台 | 原 SQLite 服务端、原明文会话令牌表 |
| Vincent-hechuan/codex-quota-band，ef4b958e0b6904035ba92b8a0874bb2e06734b7a | 借鉴设备授权、摘要同步、缓存/实时区分、凭据留在本机的边界 | 小米 SDK、手环 UI、局域网发现/配对、旧 Electron 路径 |

不把两个项目的主程序硬拼起来：前者提供业务基础，后者提供桌面到手机的交互与隐私参考。
quota-band 的 Rust/Kotlin 局域网传输不是公网服务；本次采用 Linear 的 Go/Flutter/事件日志基线。

## 2. 跨局域网架构

电脑 Web/PWA 或 Flutter Desktop → HTTPS 自部署 AI Hub ← Flutter 手机或手机 Web/PWA。

- 两端均主动连接同一服务，无需互相发现、固定家庭 IP、端口映射或同一 Wi-Fi。
- 服务端 PostgreSQL 为事实源；客户端 Drift / IndexedDB 是缓存和待上传队列。
- HTTPS 由 Caddy 终止；数据库端口不对公网发布。
- 服务端持久化可读的便笺与业务数据；这是传输加密，不是端到端加密。
- M0 同步实体是低风险便笺。额度看板仍走已有在线 API，不能把它描述为已完成额度离线同步。

## 3. M0 任务映射

| Linear | 本地实现/证据 |
| --- | --- |
| INH-303 模块边界 | docs/CONTRACTS.md、scripts/check-boundaries.py |
| INH-306 Go skeleton | cmd/aihub、配置、结构化日志、/health、/ready、CI |
| INH-308 PostgreSQL | migrations、事务、jobs 租约、回滚与并发领取测试 |
| INH-311 Auth contract | CONTRACTS 中 token/device/URL 威胁模型 |
| INH-314 Auth 实现 | auth/register/login/refresh/logout、独立撤销、审计 |
| INH-317 Flutter/Drift | profiles CRUD、SQLite 持久化、Native Secure Storage |
| INH-318 Sync contract | 协议 v1、base_version、冲突矩阵、JSON Schema |
| INH-321 Server Sync | 序列化事件提交、幂等结果、tombstone、cursor pull |
| INH-324 Flutter Sync | pending_ops、原子 page apply、断网恢复、冲突保留 |
| INH-327 Gate | scripts/gate.sh、Go/Flutter/Web 回归，真实 PostgreSQL + Drift HTTP 联测 |

这些是本地实现和验证记录；没有修改 Linear 状态，也不代表独立架构 review 或手机真机验收完成。

## 4. 本次功能

- 注册/登录与每设备独立授权，15 分钟 access / 30 天 refresh，刷新轮换和重用撤销。
- 服务器配置增删改查，HTTPS 校验，仅 loopback HTTP 开发例外。
- 离线便笺、重新打开后保留、前台每 30 秒同步、手动同步。
- 冲突显示本地与服务端两个版本，由用户选择；不悄悄覆盖。
- 删除事件传播、重建缓存、重试不重复执行。
- 继承的在线额度看板、手工登记、提醒与历史记录。
- 可安装 Web/PWA 外壳；Flutter Android/Windows/Linux 源码共用核心。
- Windows/Linux 服务端单二进制内嵌 Web；PostgreSQL 独立运行。

## 5. 后续顺序

1. M0 真机补验：Windows、Android 键盘/生命周期、真 HTTPS、Wi-Fi ↔ 蜂窝网络、系统杀进程。
2. M1：把 ProviderAccount/Quota/Rule 纳入正式同步领域，增加 Flutter 额度界面、通知去重和时间规则测试。
3. M2：冻结 Connector acquisition/provenance contract，加入 local bridge；只上传白名单摘要，Cookie 和 Provider token 不进入普通同步。
4. Alpha Gate：真实 Provider 和至少两周观察窗；不能用本次模拟 fixture 或本地 HTTP 测试替代。
5. 再扩展对话迁移、配置资产和协作；不提前加入全局 CRDT、消息队列或微服务。

## 6. 保留的限制

M0 单用户私有 workspace 的 ID 等于 user ID，多人 ACL 留待 M5。
便笺有未确认操作或冲突时禁止继续编辑该条记录，避免重写可能已执行的 operation_id。
事件和幂等结果暂不裁剪；数据库恢复到旧备份后需要运维轮换 stream epoch，客户端保留待上传队列重建。
Flutter 后台没有常驻服务，不能承诺系统杀进程后的即时同步或推送。
浏览器存储可能被用户清理/系统回收；队列不是额外备份。

## 2026-09-22 优先增量：Codex 电脑连接

根据用户新的优先级，提前实现 Codex 只读额度采集器、独立连接凭据、专用快照同步与双端缓存。具体接入方式和边界见 [Codex 连接说明](CODEX_CONNECTION_ZH.md)。其他 Provider 和远程任务控制不在该增量内。
