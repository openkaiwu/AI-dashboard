# 全平台客户端 · v1.1.4

下载页：`http://<服务器地址>:8888/`。登录工作台时使用管理员提供的 **HTTPS** 根地址。真实部署地址与凭据不写入源码。

| 平台 | 安装包 | 签名状态 |
| --- | --- | --- |
| Windows x64 | `AIHub-正式版v1.1.4.exe` | 未配置 Authenticode 签名 |
| Android | `AIHub-正式版v1.1.4.apk` | 沿用现有 Android Debug 签名，构建号 8 |
| macOS Apple Silicon | `AIHub-1.1.4-arm64.dmg` / `.zip` | 无 Developer ID 签名与公证 |
| macOS Intel | `AIHub-1.1.4-x64.dmg` / `.zip` | 无 Developer ID 签名与公证 |
| Linux x64 | `AIHub-1.1.4-linux-x64.AppImage` / `.deb` | 未签名 |
| Linux ARM64 | `AIHub-1.1.4-linux-arm64.AppImage` / `.deb` | 未签名 |
| iOS / iPadOS 15+ | `AIHub-1.1.4-unsigned.ipa` | 待自行签名，不能直接安装 |
| Web / PWA | 服务器 HTTPS 工作台 | 无需安装包 |

## 安装与使用

Windows 安装包支持覆盖更新，保留用户运行时配置。桌面端首次启动选择“登录服务器”并输入 HTTPS 地址；本地模式需要 PostgreSQL（Windows 还需要 WSL）。macOS/Linux 安装包均内置对应架构的服务端和 Codex/Cursor 采集器。

macOS 先在“关于本机”确认芯片类型。DMG 中的应用可拖入 Applications。未完成 Developer ID 签名与公证，系统可能拦截首次打开；请先核验文件来源与 SHA-256。无需关闭系统安全保护。

Linux 的 deb 可通过系统软件安装器安装。AppImage 需先添加执行权限。首次使用本地模式时，按照应用给出的 PostgreSQL 初始化命令操作。凭据保存需要可用的系统密钥环。

Android 可安装 APK；覆盖更新要求签名与已安装版本相同。其他来源或自行签名的旧包可能无法直接覆盖，请保留原数据并核实签名。

iOS IPA 需要自己的 Apple 签名及设备配置，不能当作 App Store/TestFlight 安装包。可直接在 Safari 打开 HTTPS 工作台，通过分享菜单“添加到主屏幕”使用 PWA。iPhone/iPad 安全区与主屏幕图标已适配。

## 提醒与同步边界

各平台共享额度总览、消耗分析、提醒处理状态和便笺。桌面采集器默认每 60 秒采集一次，手机端从服务器读取数据。原生通知需要系统权限。

iOS 当前没有 APNs 远程推送，也不能在关闭应用后持续后台轮询。签名、真机安装、后台生命周期和目标系统通知体验需要在相应设备验收；构建成功不等于完成这些验收。

## 独立 Windows 采集器

下载 `AIHub-Codex-Collector-Windows-latest.zip`。解压后在 `bridge/bridge.json` 保存应用生成的专用连接配置，运行 `bridge/Start-Codex-Bridge.ps1`。停止时运行 `bridge/Stop-Codex-Bridge.ps1`。此配置含连接令牌，不得分享或提交到代码仓库。

## 校验与构建

发布目录提供 `SHA256SUMS-latest.txt` 及 `release.json`，后者记录版本、构建来源、签名边界和各文件哈希。

```powershell
Get-FileHash .\安装包文件名 -Algorithm SHA256
```

```bash
# macOS
shasum -a 256 安装包文件名
# Linux
sha256sum 安装包文件名
```

Windows 使用 `scripts/Build-Desktop-Installer.ps1`；Android 使用 `scripts/Build-Flutter-Android.ps1`，以保留本机既有签名。`.github/workflows/release.yml` 在各目标系统构建 macOS/Linux/iOS，可按 `all`、`desktop`、`macos` 或 `ios` 范围触发。每个桌面任务只打包自身架构。

`deploy/publish-release.sh` 在更新下载页前检查所有文件哈希和链接，并备份旧发布文件。`deploy/update-server.sh` 仅更新服务二进制，保留数据库和配置，健康检查失败时恢复旧二进制。`scripts/verify-release-downloads.cjs` 从外部完整下载文件并核验 SHA-256。
