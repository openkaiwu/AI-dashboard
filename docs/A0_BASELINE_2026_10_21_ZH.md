# A0 基线与复现记录（2026-10-21）

> **2026-10-22 验证补记**：先前未运行的数据库用例已在 WSL 隔离测试库实际执行。首次运行暴露了设备夹具不满足单桌面授权规则及旧跨窗口求和预期，修正夹具后验证通过；小时视图遗漏窗口过滤属于实际实现缺陷，已修复。最终全量 race 与 vet 通过。以下“修复前实际行为”来自代码审查和测试设计，不表示曾对旧版本逐项运行并保存失败日志。当前证据证明修复版的回归结果；详见 [发布记录](RELEASE_V1_1_7_ZH.md)。

依据：[OPTIMIZATION_EXECUTION_PLAN_2026_10_21_ZH.md](OPTIMIZATION_EXECUTION_PLAN_2026_10_21_ZH.md) 第 3 节。本记录只盘点与复现，不含修复；修复见 [IMPLEMENTATION_STATUS_2026_10_21_ZH.md](IMPLEMENTATION_STATUS_2026_10_21_ZH.md)。

## 1 代码基线

| 项目 | 值 |
| --- | --- |
| HEAD 提交 | d711ed7（Add rollback deployment and full cross-platform download verification） |
| 分支 | codex/interface-redesign-20261011 |
| 未提交改动 | 约 37 个已修改文件 + 新增文件（localData.cjs、BackgroundSyncJob.kt 等），完整快照见 `backups/code-baseline-20261021/` |
| 快照内容 | git-status.txt、git-diff-stat.txt、uncommitted.patch（1.3 MB）、head.txt |
| 迁移编号 | 当前最大 028_mobile_reminder_once.sql；本次 A1 新增 029 |
| 测试基线 | `go vet ./...` 通过；`go test ./...` 全部通过（数据库集成用例无 `AIHUB_TEST_DATABASE_URL` 时自动跳过，仓库既有约定）；`node --test localData.test.cjs` 5/5 通过 |

未提交改动属于 v1.1.6 工作区，本批次工作叠加其上，未覆盖任何既有未提交内容；回退基线时恢复上述快照即可。

## 2 风险复现记录

复现均为可重复执行：纯函数级复现已固化为单元测试（`server/internal/connector/source_selection_test.go`），接口级复现固化为集成测试（`server/internal/api/local_data_identity_test.go`，需 `AIHUB_TEST_DATABASE_URL`，无库自动跳过）。每条用例的“实际”一列是修复前代码的真实行为。

### 2.1 旧备份遮蔽新统计

| 项 | 内容 |
| --- | --- |
| 输入 | 桌面上传过一份本地备份；随后采集器通过桥接上报了更新的实时样本（codex_history / codex_usage_rollup 已覆盖更新时间） |
| 复现入口 | `server/internal/connector/consumption.go` 旧代码 51–63 行：granularity=daily/hourly/window 时，只要 `local_data_backups` 存在任意一行就直接返回其 payload |
| 预期 | 展示按业务数据时间选择来源，新样本进入统计 |
| 实际（修复前） | 返回旧备份，实时新样本被完全遮蔽；响应无 `data_as_of`/`source`，调用方无从发现数据过期 |
| 受影响接口 | `GET /api/v1/codex/consumption`（daily/hourly/window） |

### 2.2 按上传时间而非数据时间选择备份

| 项 | 内容 |
| --- | --- |
| 输入 | 设备 A 上传了较新数据；设备 B（本地数据较旧）随后执行上传 |
| 复现入口 | `local_data.go` 旧代码 26 行 `ORDER BY updated_at DESC LIMIT 1`；且备份行 upsert 仅比较 `payload->>'generated_at'`（生成时间，非观察时间） |
| 预期 | GET 返回业务数据时间（最新样本观察时间）最新的备份 |
| 实际（修复前） | 谁最后上传谁被读取；`generated_at` 又是每次生成时的时间戳，晚到的旧数据会顶掉新数据 |
| 受影响接口 | `GET /api/v1/local-data`、`GET /api/v1/codex/consumption`、`GET /api/v1/background/local-data` |

### 2.3 同步部分成功（备份已存但报错）

| 项 | 内容 |
| --- | --- |
| 输入 | 无桥接的桌面设备携带 recent_snapshots 执行 `PUT /api/v1/local-data` |
| 复现入口 | `local_data.go` 旧代码 73–115 行：备份行先独立 Exec 提交，随后另起事务写近期样本；样本阶段任何失败（如 `bridge_required` 409）返回错误，但备份已落库 |
| 预期 | 报错时什么都没保存；成功时一次提交全部内容 |
| 实际（修复前） | 返回 409/503，`local_data_backups` 已新增一行，与响应矛盾；调用方无法区分“已接收”与“已生效” |
| 受影响接口 | `PUT /api/v1/local-data`、`PUT /api/v1/background/local-data` |

### 2.4 多设备重复计量（设计审查 + 回归防护）

| 项 | 内容 |
| --- | --- |
| 输入 | 同一账号存在多个采集源（当前公开 API 一次只保留一个活跃 bridge，但 rollup 行按 bridge 独立保留；直连数据库插入两个活跃 bridge 可复现） |
| 复现入口 | `consumption.go` 旧代码 119/553 行：`bridge_id IN (...)` 的 rollup 行直接相加 |
| 预期 | 同一小时桶只计一个有效采集源 |
| 实际（修复前） | 两源各报 10 pp 时日消耗显示 20 pp（翻倍）；快速消耗判定与展示合计均受影响 |
| 受影响接口 | `GET /api/v1/codex/consumption`（daily/weekly/monthly/hourly） |
| 说明 | 固化为回归测试 `TestConsumptionTwoActiveBridgesNotDoubleCounted`，防止未来放开多 bridge 时回归 |

### 2.5 不能复现、保留为设计审查项

- 快速消耗迟到弹窗：补传陈旧数据不触发新弹窗的机制在 `mobile_reminders.go`（事件时效 + 30 分钟账号级限频）中成立，本次未构造出反例，保留审查。
- 上游账号切换（同一 Hub 用户换 Codex 账号）身份混淆：依赖 §4.1 的身份协议，属于 A1 后续完整落地项，本次未复现具体事故。

## 3 隔离与安全

- 复现测试全部走 `testdb` 临时 schema，测试账号（m0@example.com）与日常数据隔离；不清理、不触碰真实提醒记录，不产生真实用户通知。
- 真实本地数据未做任何修改；未执行只读盘点以外的操作（本机 `.runtime` 中的本地存储样本未参与测试）。
