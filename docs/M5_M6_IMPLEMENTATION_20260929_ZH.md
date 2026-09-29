# M5/M6 纵切实现记录（2026-09-29）

按项目所有者指示继续在 G1 之前实现 M5/M6 的服务端核心纵切与 Web 界面。状态不等同于正式验收；两份契约（CONTRACTS.md 新增两节）需按 `Contract·Change-Required` 机制评审。

## 逐票状态

| 票 | 状态 | 说明 |
| --- | --- | --- |
| INH-501 | 已实现（待评审） | 工作区/角色 ACL/共享资源契约；共享域严格限定 canonical 资产，Provider 账号与配额不共享 |
| INH-503 | 已实现 | Workspace/Member/Invite 持久化与管理 API（邀请按邮箱、接受即生效） |
| INH-505 | 回归通过 | 共享读取走 workspace.ReadableScope；隔离/撤销/越权用例全部通过 |
| INH-509 | 部分 | 共享 + 评论/mention；Branch events 未含（记录为遗留） |
| INH-513 | 部分（语义完整） | SSE change hints：Last-Event-ID 重连重放、每用户 256 条缓冲、粗粒度指针；WebSocket 升级为交付细节留待收口 |
| INH-516 | 部分 | 协作工作区页面（成员/邀请/评论/接受邀请）；权限状态展示为简版 |
| INH-519 | 回归通过 | ACL、跨工作区隔离、成员移除即时生效、SSE 重连重放、审计纵切测试全部通过 |
| INH-484 | 已实现（待评审） | Promotion/Source/Confidence/Dedup 契约（内容指纹 + 时间精度 + 来源可信度） |
| INH-488 | 部分 | Source Registry + RSS/Atom 适配器（可注入 fetch）+ 用户提交通道；官方站点专用解析器需真实环境证据 |
| INH-491 | 已实现 | normalize/dedup（追踪参数归一）、Provider/plan/region 匹配、过期归档（提交即判 + ticker 扫描） |
| INH-495 | 已实现 | Watchlist CRUD + Feed + 提交表单（Web 页面）；来源展示含可信度 |
| INH-497 | 回归通过 | Watch → dedup → 通知纵切测试通过：一活动一通知，跨来源不重复 |

## 验证记录（2026-09-29，WSL + PostgreSQL 14）

- `go vet`、`go build ./...` 通过；`scripts/check-boundaries.py` 扩展后 PASS（workspace/promotion 包归属 + 新表所有权冻结）。
- 新增回归测试（真实 PostgreSQL）：`TestWorkspaceACLGate`、`TestPromotionWatchDedupNotificationAndExpiry`、`TestPromotionRSSIngestDedup` 全部通过。
- Web `tsc --noEmit && vite build` 通过；新增 `/workspaces`、`/promotions` 页面与导航入口。

## 明确未完成 / 遗留

- INH-509 的 Branch events、INH-516 的完整权限状态 UI。
- WebSocket 传输替换 SSE（INH-513 收口项）；跨实例 hub 同步（单二进制内为内存实现，多实例部署需要扩展）。
- 官方优惠源的站点专用解析器（INH-488 余项）与 G1 真实观察窗证据。
- M5/M6 移动端界面未做；通知与优惠的规则引擎深度集成（按规则条件过滤优惠）留待正式验收阶段。
