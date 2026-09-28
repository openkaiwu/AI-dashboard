# M0 验证记录

日期：2026-09-22。范围：本地候选版本 0.2.0-m0；不代表公网发布或真机验收。

## 已执行且通过

| 项目 | 执行证据 |
| --- | --- |
| 模块边界 | scripts/check-boundaries.py：依赖与 auth/sync 表所有权 PASS |
| Go + PostgreSQL | 配置真实 AIHUB_TEST_DATABASE_URL，go test -race ./... 通过，go vet ./... 通过 |
| 迁移/事务/jobs | 重复迁移幂等、回滚、8 并发 worker 只有一个 claim、过期重领、旧 lease ack 拒绝 |
| 两设备同步 | 同一版本离线修改产生冲突、相同 operation_id 重放、变更 payload 重用 ID 拒绝、分页、tombstone、cursor reset |
| 并发写入 | 12 个同时基于 version 0 写同一实体：1 次 applied、11 次 conflict、只有 1 条事件 |
| 账户安全 | 错误密码、access 失效、refresh 轮换/重用撤销、设备间隔离、tenant 隔离、CORS、协议字段校验 |
| 进程恢复 | 测试真正启动子进程、写入后 kill、重新启动、同 operation_id 重放，数据库只有 1 条事件 |
| 数据库不可用 | 认证返回 503，不误判为凭据失效；/health 存活、/ready 降级 |
| Web | npm test：6 项通过；TypeScript + Vite 生产构建通过；npm 安装时审计 0 vulnerabilities |
| Flutter | Flutter 3.47.5 / Dart 3.13.4，flutter analyze 无问题；8 项测试通过 |
| Drift 持久化 | 真 SQLite 文件关闭/重开后 queue 与 profile 保留；lost-response 重试保留 operation_id；冲突草稿、分页中断和重建 |
| 真实客户端联测 | 两套独立 Drift 数据库通过 HTTP 连接 Go + PostgreSQL；冲突 → 显式解决 → 双端一致 → 删除 → revoke |
| 完整 Gate | scripts/gate.sh 最终输出 M0 PostgreSQL + Drift + real HTTP gate PASS；临时 schema 和服务自动清理 |
| Windows 运行包 | 实际启动原生 Windows exe，/ready=ok、/api/v1/meta protocol=1；未指定 Web 外部目录，内嵌 HTML/JS/CSS 正常返回 |
| Linux 产物 | linux/amd64 单二进制构建成功；同入口已在 WSL 实际运行，通过浏览器/API 与 Flutter 联测 |
| 浏览器 UI | 实际登录、便笺保存/同步、额度样例卡片；桌面和 390×844 宽度检查，无横向溢出 |
| 浏览器离线 | 关闭服务后重新载入页面，保留已同步便笺；离线新建后 pending=1；恢复服务后 pending=0、server version=1 |
| 打包 | 源码/运行 ZIP 完整性与路径审计；不包含 .runtime、数据库、凭据、node_modules 或上游 clone |

Go 顶层用例中 TestServerProcess 是子进程 helper，正常顶层运行会跳过；它由 TestActualProcessRestart 在子进程执行。
这不等同于缺失数据库导致的集成测试跳过。最终 Gate 中数据库集成用例实际执行。

## 修复过程中发现的问题

- D 盘一度写满，4 个前端文件写成空文件：已恢复，重装损坏依赖，重新通过构建与测试。
- Windows PowerShell 5 读 UTF-8 无 BOM 中文脚本异常：本地启动脚本已采用 UTF-8 BOM。
- 脚本被 Windows 写为 CRLF：gate.sh 已恢复 LF，并添加 .gitattributes。
- 旧多文件 PWA 外壳离线重开出现白屏：改为自包含 HTML，实际关闭服务后重开、保存、补传通过。
- 原生 Windows 服务使用 WSL PostgreSQL 时遇到空闲回收：本地启动器加入 WSL keepalive，停止器只停止记录的本项目进程。
- DB 错误被认证中间件误报 401：修正为 503，并加测试，避免无意义刷新/撤销。
- UI 自动同步与手动同步重叠：同一页面只保留一个运行中的同步任务，避免过期错误覆盖成功结果。

## 仍未验证 / 不在本次实现内

- 没有公网服务器、真实域名或 Wi-Fi/蜂窝网络跨网联测。HTTPS/Caddy 部署包已准备，按用户要求后续配置服务器。
- 没有 Android/iOS 真机或 Windows Flutter 原生 UI 验收；无 Android SDK，因此未生成 APK；Flutter 核心测试不等于安装包验证。
- Docker 镜像和 GitHub Actions 配置尚未在相应环境运行。已验证的是无 Docker 的真实本地路径。
- Codex 额度读取已由下述增量完成；其他 Provider 自动采集、手机后台推送、多人 ACL、长时间观察和负载测试未完成。
- M0 便笺使用增量事件流；Codex 使用独立快照同步及本机缓存；其他额度页面仍为在线手工功能。
- 未做服务器升级/恢复演练、独立安全评审或源代码公开发布。

## 本地证据位置

- 完整 Gate 日志：.runtime/gate-results.log（本机保留，避免将运行日志混入发布包）。
- 桌面截图：docs/images/desktop.png。
- 手机宽度截图：docs/images/mobile.png。
- 运行文件与 ZIP 校验：artifacts/SHA256SUMS.json。

## Codex 连接增量（2026-09-22）

- 实际运行桌面 Codex app-server，读取 account/rateLimits/read 成功；当前返回 7 天窗口、已用 30%。这是验收时的观测值，不是固定配置。
- Windows 采集器持续运行，通过专用桥接凭据上传到 PostgreSQL；网页确认显示真实剩余额度、重置时间、采集/接收时间。
- Go race 测试和 vet 通过，覆盖跨用户隔离、只写采集凭据、未知敏感字段拒绝、旧样本/重试不覆盖、撤销阻止上传、未知窗口不当成 0%。
- Flutter analyze 无问题，9 个测试通过：新增真实 Go/PostgreSQL HTTP 接收 Codex 数据、Drift 关闭重开缓存保留、服务器作用域隔离与撤销可见性。
- 网页 6 个既有核心测试及 TypeScript/Vite 构建通过。
- 验证日志 .runtime/codex-gate.log；真实采集日志 .runtime/bridge.log（只含时间、状态、桶数量）。配置/凭据不打包。
- 尚无真实手机或公网跨网络验收；Flutter 接口/缓存测试不等于 APK 安装验收。

- 浏览器 390 × 844 手机宽度检查无横向溢出；实际停止服务后刷新 /codex，离线重开成功且保留真实额度缓存。

- 实际中断服务期间采集日志记录上传失败；服务恢复后下个周期自动上传成功。截图：docs/images/codex-mobile.png、docs/images/codex-desktop.png。

## Codex 专属分析与提醒（2026-09-22）

- 改动前保存源码、运行包、校验文件、界面截图及数据库 dump：backups/codex-connected-20260922-223049。
- 完整 Gate：.runtime/advisor-gate-final.log；Go race/vet、Flutter analyze、9 个 Flutter 测试通过。新增分析的真实 HTTP 获取和 Drift 缓存重开断言。
- Go 测试覆盖快重置且余量多、超前消耗、低额度、多窗口冲突、未知/过期时间、样本不足、计数下降截断、重置卡到期、标识脱敏、时区、持久去重、稍后唤醒、跨端忽略、暂停规则及参数校验。
- 实际桌面主机时区回归发现并修正重复时区偏移；修复后单独重跑 Codex 单元测试通过。
- 浏览器真实保存规则成功；桌面与手机布局验证及截图见 docs/images/advisor-desktop.png 和 advisor-mobile.png。
- 桌面 Codex 实际读取重置卡数量为 0，未为测试兑换任何卡。已读取 Tibo 公开原帖并将带来源摘要展示在应用中。
- 每日 09:00 的 Codex heartbeat 已创建；首次读取是本轮手动验证，尚未等到下一次定时触发。
- 系统通知按钮和去重逻辑已实现；未替用户授予浏览器通知权限，也未声称已验证实际系统弹窗或手机后台推送。
