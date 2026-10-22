# A 批次第一批实施记录（2026-10-21）

> **2026-10-22 更新**：本次已获得 WSL 访问权限，使用现有 gcc 和隔离 PostgreSQL 实际跑通全量 `go test -race ./...` 与 `go vet ./...`。下文的环境限制和“待补”是 10 月 21 日快照，最新结论以 [v1.1.7 发布记录](RELEASE_V1_1_7_ZH.md) 为准。补齐了 Web 时间回退漏洞、桌面本地观察时间与 Android 来源/时间显示、小时视图窗口过滤、长离线重放和原回执一致性。本机 Windows 覆盖更新完成，38 个持久化文件及凭据保持一致。A1 的其他收尾和 A2–A5/B/C 未因此视为完成。

依据：[OPTIMIZATION_EXECUTION_PLAN_2026_10_21_ZH.md](OPTIMIZATION_EXECUTION_PLAN_2026_10_21_ZH.md)。本批次在 v1.1.6 工作区（基线 d711ed7）上完成 **A0 全部**与 **A1 核心代码**；A2–A5、B、C 未开始。状态：代码完成、自动化通过；Windows 覆盖安装、Android 恢复、服务器发布均**未验证**，按方案要求保持待验证。

## 1 已完成内容

### A0 基线与复现（完成）

- 基线快照：`backups/code-baseline-20261021/`（git status/diff patch/HEAD）。
- 复现记录与测试：[A0_BASELINE_2026_10_21_ZH.md](A0_BASELINE_2026_10_21_ZH.md)；4 项风险转为可重复用例，1 项保留审查。

### A1 数据身份、新鲜度与可靠同步（服务端 + 桌面核心完成）

**服务端**

| 改动 | 文件 | 说明 |
| --- | --- | --- |
| 迁移 029 | `server/migrations/029_local_data_identity.sql` | 新表 `local_data_receptions`（幂等接收台账，PK(user_id, operation_id)）；`local_data_backups` 增加 `data_as_of`（业务数据时间）、`schema_version`、`operation_id` 列与新鲜度索引；存量行按 payload 回填 |
| 原子接收 + 幂等 | `server/internal/connector/local_data.go` | 备份、近期样本、接收台账同一事务提交；同 operation_id 重放返回原接收结果（`duplicate:true`+原 synced_at），不触碰任何存储数量；备份行 upsert 只按 `data_as_of` 前进，旧设备晚上传不能回退；提醒分析改为提交后的异步任务（失败不报给客户端，下次检查自然重试） |
| 来源选择与合并 | `server/internal/connector/consumption.go`、新文件 `source_selection.go` | ① 每小时桶只取一个有效采集源（最近活跃 bridge 优先，源切换留痕于桥状态），多采集源不再相加；② 空窗参数默认周额度（规则 7：不同窗口不相加）；③ daily/hourly 视图按桶合并桌面离线聚合（覆盖多者胜、从不跨源相加）；④ window 视图仅当桌面观察到比服务器更新的样本时才返回备份序列；⑤ 全部响应新增 `data_as_of`、`source`（server_rollup/local_backup/merged/none）、`stale_reason`（no_source/stale_source） |

**桌面**

| 改动 | 文件 | 说明 |
| --- | --- | --- |
| 持久化上传队列 | `apps/desktop/localData.cjs` | `upload-queue.json`：epoch+sequence 生成 operation_id；入队/接受/失败状态机，重启后保留；重试复用同一 operation_id（丢响应重放=服务端去重）；备份 payload 携带 operation_id |
| 断网补传 | `apps/desktop/runtime.cjs` | 上传进入队列后按 30s→1m→2m→4m→8m→15m 退避（±20% 抖动）自动重试；401/403 标记 needs_auth 停止自动重试；手动同步始终立即尝试；队列状态并入 localStatus |
| 状态展示 | `apps/web/src/LocalDataSettings.tsx`、`UsageTab.tsx`、`consumption.ts` | 显示“有 1 份待上传数据 · 下次自动重试 …/自动上传已暂停（登录过期）”；消耗图注改为“数据截至〈观察时间〉· 来源：…”（不再把响应生成时间当作采集成功时间） |

**固定规则核对**（方案 §1）：桌面/网页静默未改动（强提醒仍由主手机投递）；手机一次语义事件一次提醒逻辑未触碰；30 分钟限频与免打扰未改；快速消耗阈值未改（仅修复其输入的重复计量）；默认本地、同步需用户开启未改（auto_sync 默认 false 不变）；备份不含凭据/聊天未改；多设备不相加已实施；通知三状态语义未触碰。

## 2 验证命令与证据

本机环境：Windows，无 WSL（被安全策略阻止）、无本机 PostgreSQL、无 C 编译器。Go 1.24.9 安装于 `C:\Users\admin\.workbuddy\binaries\go\`。

| 命令 | 结果 |
| --- | --- |
| `go vet ./...`（server） | 通过 |
| `go test ./...`（server，无测试库时集成用例跳过） | 全部 ok |
| `go test ./internal/connector/`（新增 7 个单元测试） | ok：合并不翻倍/补空档/平局归实时、过期阈值、备份新鲜度、legacy 回填、窗口过滤、来源标签 |
| `go test -race` | 本机无法执行（需 cgo+gcc），属 WSL/CI 门槛，待补 |
| `node --test localData.test.cjs`（桌面，新增队列生命周期测试） | 6/6 通过 |
| `node --test runtime.test.cjs` | 通过 |
| `npx vitest run`（web） | 36/36 通过 |
| `npx tsc --noEmit`（web） | 通过 |

已编写、需数据库环境执行的集成验收测试（`server/internal/api/local_data_identity_test.go`，符合仓库“无库跳过、完整 gate 走 CI”惯例）：

1. `TestLocalDataIdempotentReception`——同一上传重放 10 次，codex_history 与接收台账数量不变，每次返回 duplicate。
2. `TestLocalDataLateOlderUploadDoesNotRegress`——旧数据晚上传，`data_as_of` 与展示值不回退。
3. `TestLocalDataAtomicWhenBridgeMissing`——拒绝上传后备份行为 0（修复前会残留 1 行，即 §2.3 的部分成功）。
4. `TestConsumptionTwoActiveBridgesNotDoubleCounted`——双采集源日消耗 10 pp 而非 20 pp。
5. `local_data_test.go` 断言更新为新契约（完整日历 + 合并桶 + source/data_as_of 元数据）。

**在配置了 `AIHUB_TEST_DATABASE_URL` 的环境运行一次上述用例，是 A1 验收的剩余必做项。**

## 3 未完成项与已知边界

**未开始**：A2 备份恢复中心、A3 提醒链路体检、A4 双端雷达统一、A5 发布回归、B（FCM，Firebase 项目未创建）、C。Flutter 端（`local_cache.dart`/`background_settings.dart` 的数据时间与队列状态展示）本次未改动，属于 A1 移动侧收尾项。

**已知边界（有意为之，非缺陷）**：

1. 备份与服务器时区不一致时，daily 按桶标签合并可能错位一天；按小时重算需 §4.1 完整身份协议，列入 A1 收尾。
2. 近期样本插入沿用既有保守条件（不插入比桥最新快照更新的样本）；断网期的完整数据由备份聚合覆盖，raw 缺口由 window 视图的备份回退覆盖。
3. 幂等台账按 (user, operation_id) 记录，不设过期清理；量级为每次同步一行，后续可随诊断日志一并清理。
4. 多设备 bridge 当前被 API 约束为单活跃，去重查询是回归防护；若未来放开多 bridge 需补充主手机式的“有效采集源”绑定。
5. `go test -race`、Windows 覆盖安装、Android 模拟器恢复、服务器发布、双端旧客户端混用（方案 §8）均未验证——本批次不得据此宣布 A 版本发布就绪。

## 4 下一步建议顺序

1. 配置测试数据库跑通 5 个集成验收用例（A1 验收闭环）。
2. A1 移动侧收尾（Flutter 数据时间/同步状态展示 + 旧协议兼容双读验证）。
3. 进入 A2 备份恢复中心（依赖 A1 的 schema_version/operation_id，已就绪）。
