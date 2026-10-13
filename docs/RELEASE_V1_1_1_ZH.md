# AI Hub v1.1.1 提醒逻辑更新

版本：Windows `1.1.1`，Android `1.1.1+5`。日期：2026-10-08。

## 更新内容

- 同一额度周期、窗口及风险类型的事件只自动提醒一次，未读也不定时重发。
- 选择“一小时后提醒”或“恢复”后允许再次检查；新周期及风险升级使用独立事件。
- 已读、稍后及忽略状态跨端同步；服务端预留与送达记录减少电脑、手机重复弹出。
- 弹窗已显示后保留送达记录，操作保存失败或关闭网页不会重新弹出。
- 移除重复间隔设置；轻量建议留在页面，低额度、持续消耗偏快及重置卡到期按风险排序。
- 改善低余量和临近重置的显示，补充通知权限及失败处理。

## 发布验证

- Go：全量 `go test -race ./...`、`go vet ./...` 与模块边界检查通过；含两天未读不重发、最新额度快照、多端重复请求、稍后到期及并发领取回归。
- Web：14 项测试、TypeScript 检查及生产构建通过。线上 HTML 和 Service Worker 与本轮构建的 SHA-256 一致。
- Windows：19 项桌面测试通过；安装包及本机安装后的源码、版本、内置服务端与采集器核对通过。已覆盖升级并重新启动，8 个持久化凭据/连接配置文件哈希保持一致。
- Android：静态检查无问题；16 项测试通过，2 项需要专用集成账号的测试跳过。APK 版本为 `1.1.1`、构建号为 `5`；v2 签名有效，签名证书与 v1.1 一致。
- 服务端已备份旧二进制和数据库、应用迁移 025/026 并重新启动；本地及公网健康检查通过。已有桌面登录仍可访问线上提醒接口，连续读取不改变同一事件的触发时间，新版送达接口已上线。
- 最新下载页、SHA-256 清单和 Windows/Android 安装包已发布；完整 HTTPS 下载校验记录见本地发布目录。

## 8888 下载入口复验（2026-10-09）

- 默认下载页使用 HTTP `:8888`；公网根路径返回 302 并跳转到 `/downloads/index.html`。下载页返回 200，3188 字节，与本地发布页 SHA-256 一致。
- Windows EXE 和 Android APK 的完整公网下载均完成，字节数和 SHA-256 与本地发布包一致；SHA-256 清单也与本地文件一致。
- EXE 的 1 MiB Range 请求返回 206，分段 SHA-256 与本地文件相同；Nginx 配置检查通过。
- 下载入口更新后，Web 测试为 17/17 通过。复验明细记录在 `artifacts/releases/v1.1.1/download-port-8888-verification.json`。

## 产物和证据

本地目录：`artifacts/releases/v1.1.1/`。

| 文件 | SHA-256 |
| --- | --- |
| AIHub-正式版v1.1.1.exe | `231ab8452cf90a16180c542a47559a40acdb9fe8838e4d38d3c5ff001ece86e8` |
| AIHub-正式版v1.1.1.apk | `fc49d3ddad0416715503c5e9ada1adb654fb5b4bc9987e30521231c8e2c8fefa` |

证据包括 `desktop-installation.json`、`android-signature.txt`、`live-api-verification.json`、`download-exe.json`、`download-apk.json` 和 `verification.json`。服务器下载页使用相对链接，不包含部署凭据。

## 验证边界

Windows 仍未配置发布者 Authenticode 签名；Android 沿用现有 Android Debug 签名证书。当前没有连接安卓设备，尚未进行本轮手机覆盖安装或真机通知验收。完整 M0 Gate 和 Windows 整机重启未执行。
