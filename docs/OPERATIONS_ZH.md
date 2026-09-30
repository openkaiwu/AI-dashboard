# AI Hub 运维手册与发布清单（R3/INH-545）

本文是自部署运维手册（安装/升级/备份/恢复/回滚）与发布清单（Release Checklist）。配套脚本：`scripts/backup.sh`、`scripts/restore.sh`、`scripts/security-audit.sh`；打包模板在 `deploy/selfhost/`。

## 1. 安装（Clean install）

单二进制 + PostgreSQL 即可运行核心：

1. 准备 PostgreSQL 14+ 与空数据库 `aihub`。
2. 复制 `deploy/selfhost/aihub.env.example` 为 `aihub.env`，填写 `AIHUB_DATABASE_URL` 与 `AIHUB_ALLOWED_ORIGINS`。
3. 启动：`systemd`（`aihub.service` 模板）或可选 Compose（`deploy/selfhost/docker-compose.yml`）。
4. 首次启动自动执行迁移；创建唯一管理员：`echo '管理员密码' | aihub-admin --email you@example.com`（连接串通过 `AIHUB_DATABASE_URL` 传入）。
5. 验证：`GET /ready` 返回 ok；`GET /api/v1/meta` 返回 protocol=1。

## 2. 升级（Upgrade）与版本拒绝（INH-540）

1. 备份（见下节）。
2. 替换二进制，重启服务。启动顺序：**Preflight → 迁移 → 服务**：
   - Preflight 发现数据库由**更新的二进制**迁移过（存在本版本不认识的 migration 版本）→ 拒绝启动，exit 2——旧进程永不改动新 schema。
   - 迁移在**单事务**内执行：失败即整体回滚，旧 schema 完整保留（回滚路径 = 重启旧二进制即可）。
3. 仅想跑迁移不上服务：`AIHUB_MIGRATE_ONLY=1 aihub`。
4. 升级后验证：`GET /api/v1/admin/operations` 的 `upgrade_preflight.compatible` 应为 true。

## 3. 备份与恢复（INH-538）

```bash
AIHUB_DATABASE_URL=... AIHUB_BACKUP_DIR=/opt/aihub/backups scripts/backup.sh   # pg_dump --clean，gzip
scripts/restore.sh /opt/aihub/backups/aihub-backup-<ts>.sql.gz                  # 二次确认后 --single-transaction 重放
```

- 备份为 plain SQL（`--clean --if-exists`）：恢复即整体替换目标状态。
- 设置 `AIHUB_BACKUP_DIR` 后，`GET /api/v1/admin/operations` 自动显示最近一次备份时间与文件名。
- **恢复演练**已自动化：`go test ./internal/api -run TestR3BackupRestoreDrill`（真实 pg_dump → 破坏 schema → 恢复 → 校验数据回归）。

## 4. 健康与运维观测（INH-543）

- `GET /ready`：存活；`GET /api/v1/health`：数据库可达性（带 request_id）。
- `GET /api/v1/admin/operations`（管理员）：迁移版本、任务队列、升级预检、备份状态。
- `GET /api/v1/admin/telemetry`（管理员）：G1 观察报表（连接线新鲜度、额度新鲜度、失败分类、量级）。

## 5. 发布前安全审计（INH-544）

自动半：`scripts/security-audit.sh`（go vet、硬编码密钥/私钥扫描、注册关闭、桥接设备校验、令牌哈希存储、secret_ref 机制、扩展无 cookie、CORS 白名单、raw 快照不入 sync）。
人工半（逐项确认后记入发布说明）：

- [ ] 管理员账户为最小集合，初始密码已轮换
- [ ] `AIHUB_ALLOWED_ORIGINS` 仅含部署域名；反向代理启用 HTTPS
- [ ] 桥接令牌仅下发到受信桌面；解绑流程演练过
- [ ] 导入文件来源可信；raw 快照保留策略与合规要求一致
- [ ] 备份文件权限 600 且存放在数据库主机之外

## 6. Release Checklist（发版逐项）

- [ ] `scripts/gate.sh` 全绿（边界 + Go -race + 真实 PG + Flutter + Web）
- [ ] `scripts/security-audit.sh` PASS + 人工半逐项确认
- [ ] 跨平台产物：server（linux/amd64、windows）、bridge、admin、Web dist、Flutter APK
- [ ] 迁移只增不改；`AIHUB_MIGRATE_ONLY=1` 预跑成功
- [ ] 备份 → 升级 → `operations` 验证 → 冒烟（登录/额度/导入）→ 备份留存
- [ ] 发布说明含：变更、迁移清单、回滚步骤（旧二进制 + restore.sh）
- [ ] INH-546 清洁安装验收通过（全新机器按本文档从零到可用）
