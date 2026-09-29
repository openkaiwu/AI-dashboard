# M3/M4 纵切实现记录（2026-09-29）

按项目所有者指示，在 G1 观察窗之前提前实现 M3/M4 的服务端核心纵切与 Web 界面。以下状态不等同于正式验收；M3/M4 的 Contract·Freeze 复核与两张 Gate 票仍需按流程收口，且契约（CONTRACTS.md 新增两节）需按 `Contract·Change-Required` 机制评审。

## 逐票状态

| 票 | 状态 | 说明 |
| --- | --- | --- |
| INH-433 | 已实现（待评审） | canonical 迁移 013、模型、aihub.conversation-archive v1 信封 |
| INH-438 | 已实现（待评审） | 尺寸/数量限制、单事务批次、原始快照保留、去重身份 |
| INH-442 | 部分 | 异步导入管道（jobs + worker）；object storage 未含（本地单二进制内完成） |
| INH-446 | 已实现 | chatgpt_export 树形导入器 + golden fixtures（基于公开导出格式整理，真实大文件待验证） |
| INH-450 | 部分 | codex_cli_jsonl 为第二来源；其余 Provider 待真实样本 |
| INH-454 | 部分 | Web 会话浏览器（列表/详情/分支/导入/导出/项目归属）；本地搜索未含 |
| INH-455 | 回归通过 | Archive/Markdown 导出 + round-trip 去重测试（真实 PostgreSQL） |
| INH-459 | 已实现（待评审） | AIPortableConfig v1：canonical 内容、SecretRef、loss_report |
| INH-463 | 部分 | ConfigAsset/version/binding 持久化与 API；sync wire 扩展未做（需 sync 契约 v2，见下） |
| INH-467 | 已实现 | Bridge 授权目录扫描（scan_directories）+ Secret 键名分类 + 服务端存储 |
| INH-471 | 已实现 | claude_desktop → canonical → codex_cli 参考转换（golden 断言） |
| INH-473 | 未开始 | 反向 TOML 解析与第三平台 |
| INH-477 | 部分 | diff/rollback API + Web 版本对比与转换预览；LossReport UI 为简版 |
| INH-479 | 回归通过 | round-trip / secret leakage / rollback / 扫描键名回归测试全部通过 |

## 验证记录（2026-09-29，WSL + PostgreSQL 14）

- `go vet ./...`、`go build ./...` 通过；`scripts/check-boundaries.py` 扩展白名单后 PASS（conversation/config 包归属 + 新表所有权冻结）。
- 新增回归测试（连接真实 PostgreSQL）：`TestConversationImportDedupBranchAndArchiveRoundTrip`、`TestConversationImportRejectsInvalidInput`、`TestConfigPortabilitySecretHygieneTransformAndRollback` 全部通过。
- Web `tsc --noEmit && vite build` 通过；新增 `/conversations`、`/config` 页面与导航入口。

## 明确未完成 / 遗留

- sync wire v1 只支持 note 实体；会话与配置的跨端离线同步需要 sync 契约 v2（payload 泛化 + 客户端过滤未知实体），本轮明确不做，相关票保持 open。
- 移动端未加入 M3/M4 界面；G1 两周真实观察窗仍未启动。
- chatgpt/codex 导入器的 fixtures 基于公开格式整理，真实账号大文件导出验证属于 AI·H-External 证据，尚未进行。
- INH-433/438/459 标注 Contract·Freeze：三张契约票在合入后应由 A 级评审复核，如需修改契约走 `Contract·Change-Required`。
