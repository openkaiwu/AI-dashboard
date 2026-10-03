# 邀请码注册与管理员审核（R4）

## 功能概述

桌面端（exe）、手机 App 和登录页开放**邀请码注册**：用户凭管理员发放的邀请码提交注册，账户进入 `pending`（待审核）状态，无法登录；管理员在 Web 管理台**批准**后账户转为 `active`（可正常登录并绑定设备），**拒绝**则账户保持停用并留存原因。

- 邀请码形如 `XXXX-XXXX-XXXX-XXXX`（不含易混淆字符 I/L/O/0/1），服务器只存哈希，**明文码仅在生成时显示一次**。
- 注册不占用设备绑定名额；设备绑定发生在审核通过后的首次登录，规则不变（每账户一台电脑 + 一台手机）。
- 注册接口按来源 IP 限速（每小时 20 次），邀请码校验失败有延迟响应，防爆破。

## 管理员操作

入口：登录页登录管理员账户 → 「⚙ 账户管理」（管理台）。

### 1. 生成邀请码

在「邀请码」区块填写：

| 字段 | 说明 |
|---|---|
| 备注 | 发给谁，例如"发给张三"（≤120 字） |
| 有效天数 | 0 = 永不过期；否则自生成时刻起 N 天内有效（≤365） |
| 可用次数 | 1–100；1 = 一次性码 |

点击「生成邀请码」后明文码**仅显示一次**，请立即复制发给用户。已生成的码可随时「吊销」（恢复后继续可用）；用完、过期、吊销的码在列表中均有状态标注。

### 2. 审核注册申请

「注册审核」区块展示所有申请（默认含待审/已批/已拒），显示邮箱、留言、邀请码备注、提交时间：

- **批准**：账户转为 active，用户即可登录；
- **拒绝**：需填写原因（≤200 字，留存审计），账户保持停用，用户登录会得到"账户尚未获授权或已停用"提示。

补充说明：被拒邮箱重新注册会返回"该邮箱的注册申请未通过审核"；如需给该邮箱第二次机会，可在「账户」区块对该用户执行「授权」（disabled → active）。原有的"创建账户"直建入口保留，管理员仍可绕过邀请码直接开户。

## 用户操作（exe / App / 登录页）

1. 在登录页点击「使用邀请码注册」；
2. 填写邀请码、邮箱、密码（至少 8 位）、留言（可选）；
3. 提交后页面显示"等待管理员审核"；
4. 管理员批准后，回到登录页用邮箱 + 密码 + 设备名称登录。

若在审核前尝试登录，客户端会提示"账户正在等待管理员审核"。

## 接口清单（服务端）

| 方法与路径 | 说明 |
|---|---|
| `POST /api/v1/auth/register` | 邀请码注册，成功返回 201 `{status:"pending"}` |
| `GET /api/v1/admin/invites` | 邀请码列表 |
| `POST /api/v1/admin/invites` | 生成邀请码（明文码仅本次返回） |
| `PATCH /api/v1/admin/invites/{id}/status` | 吊销 / 恢复（`status: active|revoked`） |
| `GET /api/v1/admin/registrations?status=` | 审核列表（`pending`/`approved`/`rejected`/`all`，默认 `pending`） |
| `POST /api/v1/admin/registrations/{id}/approve` | 批准 |
| `POST /api/v1/admin/registrations/{id}/reject` | 拒绝，body `{reason}` |

注册错误码：`invalid_input`、`email_pending_review`（409，该邮箱已在待审）、`email_taken`（409）、`email_rejected`（409，曾被拒）、`invite_invalid` / `invite_expired` / `invite_exhausted` / `invite_revoked`（400）、`too_many_attempts`（429）。登录新增错误码：`account_pending`（403，待审核）。

## 数据模型（migration 020）

- `invite_codes`：`code_hash`（SHA-256，与 installation_hash 同一套哈希）、`note`、`max_uses`/`use_count`、`status(active|revoked)`、`expires_at`、`created_by`；
- `registration_applications`：`user_id`、`email` 快照、`invite_code_id`、`note`、`status(pending|approved|rejected)`、`reviewed_by`/`reviewed_at`/`reject_reason`。

审计事件：`registration_submitted`、`registration_approved`、`registration_rejected`、`invite_created`、`invite_revoked`、`invite_restored`。

## 安全设计要点

1. **注册产生 pending 账户**：登录与鉴权中间件均只放行 `active` 账户，审核是登录的必经闸门；
2. **邀请码只存哈希**：数据库泄露不泄露可用码；码空间约 79 bit，配合 IP 限速无法在线爆破；
3. **次数扣减原子**：注册事务内 `UPDATE ... WHERE use_count<max_uses AND 未过期 AND status='active' RETURNING id`，并发不超发；
4. **审核操作行级锁 + 状态守卫**：`FOR UPDATE` 加 `WHERE account_status='pending'`，重复审批返回 409；
5. `scripts/security-audit.sh` 已将审计项更新为校验上述不变量。

## 验证

- Go 集成测试：`server/internal/api/invite_registration_test.go` 覆盖发码→注册→待审登录→批准→登录→设备绑定→拒绝→吊销/过期/超次全流程；`go test ./...` 全量通过（需 `AIHUB_TEST_DATABASE_URL`）。
- Web：`npm run build`（含 tsc 类型检查）通过；桌面端 exe 内嵌同一 Web UI，服务器更新后自动生效。
- 移动端：`apps/mobile` 无 Flutter SDK 环境时请在本机执行 `flutter test` 与 `flutter analyze` 复核。
