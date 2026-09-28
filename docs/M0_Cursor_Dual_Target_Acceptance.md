# AIDASH M0 验收清单（Cursor 双目标）

> 状态：设计已锁定 · 2026-09-24  
> 范围：本文件只定义验收标准；实现前需 git + 远程。不重做公网基建。

依据已锁定基线。实现时按下列条目打勾。**Codex 现网行为不得回退。**

## 0. 前置（实现前，不算功能验收）

- [ ] 仓库具备 git + 远程后，才允许 CloudAgent 改代码
- [ ] 不重做公网部署 / 防火墙 / 已通联调基建（`https://hub.example.com`）

## 1. 架构与数据

- [ ] 存在共用 `provider` 抽象；`codex` / `cursor` 为两套 connector 实现
- [ ] 单 bridge 进程可同时挂多 connector（不是双 bridge 守护进程）
- [ ] Cursor 快照写入 **generic** provider/quota 路径（复用 dashboard/rules/inbox），不新造一套 Codex 分叉表作为唯一真相
- [ ] 迁移在现有 `003/004_codex` 之后扩展 provider 维度；既有 Codex 行可回填 `provider=codex`
- [ ] Codex 与 Cursor 账号/历史 **并列保留**；切换模式不删除、不合并另一边

## 2. Cursor 采集（C：混合）

- [ ] 本机只读 `%APPDATA%\Cursor\User\globalStorage\state.vscdb` 认证相关键；**令牌不下云、不进上传 payload**
- [ ] 优先 `api2.cursor.sh`：`GetCurrentPeriodUsage`；必要时回退 `auth/usage`
- [ ] 成功上报消毒 snapshot，`source=cursor_api2`（无邮箱/token/原始凭证）
- [ ] 失败时：保留上次成功并标 `stale`，或回落 `user_manual`；再不行 `unknown` / `unavailable`
- [ ] 任何路径 **不写假额度数字**
- [ ] 采集周期与 Codex bridge 同级可运维（至少支持 periodic + `--once` / `--probe` 类探活）

## 3. 模式切换（只影响展示）

- [ ] Web / 桌面 / Android 均有 `Codex 模式` | `Cursor 模式`，可随时切换
- [ ] 切换后列表/详情/设置入口只呈现当前 provider
- [ ] **采集与提醒两边都继续**；模式不暂停另一边
- [ ] 偏好本机记忆（`mode-preference` / `activeProvider`）；默认：有历史用上次，否则偏已有账号（现网偏 Codex）
- [ ] M0 不做「全部」总览第三视角；不做跨设备云端统一偏好

## 4. 提醒与通知

- [ ] 规则可绑 `(provider, account)`；正文可见 provider 名
- [ ] Codex、Cursor 提醒均能触发（不因当前 UI 模式而静音另一边）
- [ ] 点击非当前模式的通知 → **先切到对应模式再打开详情**
- [ ] Cursor 在 `stale` / `unknown` 时：可提示数据不可用，但不得用臆造用量触发「额度耗尽」类误报

## 5. 客户端展示（M0 最小）

- [ ] 复用 generic Dashboard / Rules / Inbox，Cursor 自动或手动数据能并列出现
- [ ] Cursor 模式空态文案明确（未连接 / 采集失败 / 仅手动）
- [ ] Codex 专用五页工作台 / advisor **不要求**在 M0 被 Cursor 完整镜像
- [ ] 桌面/手机：模式切换可用；强提醒链路对 Cursor 至少能展示带 provider 的告警

## 6. 回归（Codex 不得坏）

- [ ] 现有 Codex 采集、bridge、`/api/v1/codex/*`、顾问提醒、桌面自动 bridge 行为保持可用
- [ ] 打开 Cursor 模式再切回 Codex，数据与提醒正常

## 7. 明确不做（M0 失败也不算欠债）

- 重建基建；统一 OAuth 合并登录
- 跨 provider 汇总图表 / 统一成本归因
- Cookie 爬 `cursor.com` 控制台
- 假额度；把 Cursor accessToken 上传服务器
- 为 Cursor 完整复制 Codex advisor 五页体验（证明采数后再议）

## 8. 建议验收顺序

1. 消毒 snapshot 契约 + 令牌不出域的单元/集成探活
2. Bridge 上报进 generic 表 → Dashboard 可见
3. 失败回落 `stale` / `manual` / `unknown`
4. 模式切换三端
5. 双端提醒 + 通知自动切模式
6. Codex 回归

## 附录：已锁定产品决策摘要

| 项 | 决策 |
|----|------|
| 双目标 | Codex 与 Cursor 并存，非二选一上线 |
| UI 模式 | 可切换 Codex / Cursor 模式 |
| 切换范围 | 只影响展示；采集与提醒双端继续 |
| 通知 | 带 provider；点击时自动切到对应模式 |
| 采集 | C 混合：api2 优先；失败 stale / manual / unknown；不造假数 |
| 安全 | 令牌只留本机；只上传消毒 snapshot |
| 架构 | 共用 provider 抽象 + 单 bridge 多 connector |
