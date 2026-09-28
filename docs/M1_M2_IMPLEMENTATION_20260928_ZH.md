# M1/M2 与设备绑定实施记录（2026-09-28）

## 本分支已实现

- 管理员初始化命令、账户创建/禁用/重置密码/设备解绑；公开注册关闭。旧用户迁移为待确认，旧设备及未绑定 Bridge 失效。桌面和手机各限一台有效安装；同安装重复登录复用绑定。
- 服务端按账户检查额度、历史、规则、提醒、连接及同步；Web 管理会话只可访问管理 API。桌面会话由操作系统安全存储保护，Bridge 凭据不写入明文配置。桌面首次填写 HTTPS 服务器，登录后自动启动采集，窗口关闭保留托盘，退出停止进程。
- Codex 套餐手动选择及来源字段：Plus 展示五小时窗口，Pro 排除五小时窗口及关联建议/提醒，未知套餐不推断。窗口按 `duration_minutes=300` 识别，与 primary/secondary 顺序无关。
- 手工额度快照的操作 ID 幂等重试、本机按服务器/账户/设备隔离的待同步队列、账户名称/地区/手工套餐离线编辑、历史趋势、CSV/JSON 文件预览和字段校验。重复文件行不会生成重复快照；手工更新不会改写自动额度桶的元数据。
- 提醒中心的已读、忽略和稍后提醒操作在服务端同步；稍后提醒到期后重新进入未读状态。
- Codex/Cursor 独立选择入口与两端五页框架；Cursor 缺字段保持未知。通用连接器清单、桌面独立采集退避、绑定桌面的 Bridge，以及固定页面扩展 PoC。扩展样本进入独立 `connector_samples`，不会成为真实额度。
- BillingEvent 基础存储与按账户查询/手工创建 API。

## 明确未完成或未验证

- INH-332/336/339：合同已有基础表和 API，手工 Entitlement 名称可编辑；完整账单字段与账户/额度的跨端离线写入尚未完成。
- INH-342：额度快照及现有账户字段可离线重试；新建 ProviderAccount、Entitlement、QuotaBucket 的离线同步仍需扩展。
- INH-345/349/353：服务端规则、去重、预览和时间调度已存在；移动端系统本地通知、投递状态持久化与系统终止后的调度尚未实现。
- INH-366/369/372：manifest 与采集退避已有基础实现；统一 Connector Result/Error、完整运行器限流/任务管理及 golden fixture 回放尚未完成。
- INH-380/383：固定页面、逐站点显式授权、Bridge 配对与结构化数值上报源码已实现；真实 Chrome 扩展加载、权限撤销与页面结构变化实测尚未完成。
- INH-388/390：绑定、撤销与健康信息已有一部分；任意目录扫描、路径逃逸与 symlink 防护功能尚未实现，因此不视作完成。
- INH-396：本轮只记录 Codex/Cursor，未选择第三个平台。INH-399 官方 API 接入、INH-404 真实官方网页适配明确暂缓。INH-407 标准 CSV/JSON 及手工 fallback 已实现，但特定 Provider 的官方导出格式未冻结。INH-412 的完整回放框架待补。
- INH-361、INH-416 正式验收 Gate 与跨网络手机/桌面实机验收按本轮要求暂缓。源码构建/单元测试不代表已部署或实机可用。

## 开发检查结果

- Go 全量单元与 API 集成测试通过；Windows/Linux 的 server、Bridge、admin 六个目标已在独立目录交叉编译成功。
- Web TypeScript/Vite 构建、Flutter 静态分析与单元测试、Electron 主进程测试通过。Flutter 的两项真实服务器联测因缺少预置账户而跳过。
- Android APK 构建未运行到编译阶段：当前 WSL 环境没有 Android SDK。Chrome 扩展未在真实浏览器安装验证。
- 旧本地 server exe 正在运行，发布脚本不能覆写该文件；本轮没有替换运行进程，也没有公网部署。

## 新增接口

| 接口 | 权限 | 说明 |
| --- | --- | --- |
| `POST /api/v1/auth/login` | 公开 | 必须给出 `device_kind` 与 `installation_id`；只接受 active 账户。 |
| `GET/POST /api/v1/admin/users` 等 | 管理员 | 账户授权、禁用、密码重置、解绑；管理浏览器不占桌面/手机名额。 |
| `GET/PUT /api/v1/codex/plan` | 当前账户 | `plus/pro/unknown` 及 `manual` 来源。 |
| `POST /api/v1/quota-buckets/{id}/manual-snapshot` | 桶所属账户 | 可选 `operation_id`；重复相同请求返回 `replayed=true`，复用 ID 但内容不同返回 409。`source_type` 可为 `user_manual` 或 `file_import`。 |
| `PATCH /api/v1/provider-accounts/{id}` | 账户所属用户 | 编辑显示名称/地区及手工套餐；自动采集套餐拒绝覆盖。 |
| `POST /api/v1/notifications/{id}/action` | 提醒所属用户 | `read`、`dismiss`、`snooze`；稍后提醒默认一小时。 |
| `GET/POST /api/v1/provider-accounts/{id}/billing-events` | 账户所属用户 | 账单事件查询及手工录入。 |
| `GET /api/v1/connectors` | 登录用户 | 返回当前 Codex/Cursor 连接器清单。 |
| `POST /api/v1/connectors/sample` | 有效桌面 Bridge 凭据 | 只接收固定页面 Cursor 数值样本，隔离存储。 |

## 安全边界

安装 ID 是账户授权槽位标识，并非硬件证明；管理员解绑是换设备的可信操作。浏览器扩展仅用当前页面显式权限与本机配对码，不能持有通用账户令牌。所有示例域名/IP 均为占位符；构建与公开仓库不得包含 `.runtime`、Cookie、访问令牌、数据库、原始网页或个人用量。
