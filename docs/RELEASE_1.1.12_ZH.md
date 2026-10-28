# AI Hub Windows 1.1.12 白屏修复

日期：2026-10-28。本次只更新 Windows EXE；Android 保持 1.1.11（构建号 15），其他平台保持 1.1.4。

## 问题与修复

在已安装的 1.1.11 中复现了窗口只有背景、没有界面的问题。生产构建的 `precache.mjs` 会把前端 JavaScript 内嵌到 HTML，而桌面静态资源处理器发送的 CSP 只有 `script-src 'self'`，因此 Chromium 拒绝执行整个应用脚本。HTML 导航成功和普通 Node 单元测试不足以发现这个问题。

修复后，桌面根据随包 HTML 中脚本的实际内容生成 SHA-256 CSP 白名单，并按 HTML 解析规则处理换行。没有开放任意内联脚本或 `eval`；sandbox、contextIsolation、禁用 Node 集成及原有 API 转发保持启用。独立启动保护脚本在根节点 10 秒后仍为空时显示“界面未能启动”和“重新加载”，避免没有反馈的白屏。

Windows 打包脚本新增两个实际 Electron 门槛：先检查暂存界面，打包后再从 `app.asar` 和随包 UI 检查。测试失败时停止后续产物元信息生成。

## 验证结果

| 项目 | 结果 |
| --- | --- |
| 修复前复现 | 实际安装窗口白屏；Electron 回归捕获 CSP 拒绝内联脚本，登录表单未渲染 |
| 桌面单元测试 | 38 项通过，包含精确脚本哈希、换行、HEAD 响应、更新后重新计算白名单 |
| Web | 44 项通过；TypeScript 和生产构建通过 |
| 实际 Electron | 暂存界面、打包后的资源、安装后的资源均通过 HTTP/HTTPS 登录页渲染、preload 桥接、未授权内联拦截、故障提示及点击重试恢复 |
| 覆盖安装 | 73 个升级前持久化文件的哈希在安装后、启动前保持一致；安装的 app.asar、UI、服务端和采集器二进制与构建一致 |
| 安装后窗口 | 原有登录正常，额度总览及本地数据可见；深色主题可切换；桌面控制台显示采集器运行和版本 1.1.12 |
| Go race | 本次无 Go 改动，未重新执行；上一版 WSL + 隔离 PostgreSQL 的 race 门槛结果不冒充本版重跑结果 |
| Android | 安装包及签名不变，本次未重新执行 Android 验收 |

实际 Electron 回归可在仓库根目录执行：

```powershell
node scripts/run-desktop-render-test.cjs
node scripts/run-desktop-render-test.cjs artifacts/desktop/installer/win-unpacked
```

第一个命令要求已运行 `scripts/Stage-Desktop-UI.ps1`；第二个要求已经打包。测试使用隔离的临时用户目录和保留的 `.invalid` 域名，不读取真实账户凭据。

## 产物

- 文件：`AIHub-正式版v1.1.12.exe`
- 大小：119776994 字节
- SHA-256：`d02e36d67f5d38f041bce8105ef21503201ddd8cdffe25eb6a256d4905faa6ce`
- Windows 发布者签名：未配置。
- 下载页路径：`/downloads/index.html`；稳定 EXE 链接：`/downloads/desktop/AIHub-Setup.exe`。

发布状态：服务器已上架 1.1.12，版本清单和稳定下载链接验证通过；替换前文件保存在 `backups-v1.1.12-20261028T051411Z`。公网完整 EXE 字节校验进行中，完成后补记结果。

本次不修改云端服务、数据库、Nginx 或 Android 包；下载页保留时区切换、NetScope 和其他平台入口。Linear 其余待办继续以原实施台账为准。
