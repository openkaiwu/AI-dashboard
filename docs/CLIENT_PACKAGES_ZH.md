# 全平台客户端

当前统一版本为 **v1.1.4**（移动端构建号 8）。下载页为 `http://<服务器地址>:8888/`，账户登录使用自己的 HTTPS 工作台地址。下面旧版示例文件名仅供历史参考。

| 平台 | 安装包 | 说明 |
| --- | --- | --- |
| Windows x64 | `AIHub-正式版v1.1.4.exe` | 未配置 Authenticode 签名 |
| Android | `AIHub-正式版v1.1.4.apk` | 沿用现有调试签名 |
| macOS Apple Silicon / Intel | `AIHub-1.1.4-arm64.dmg` / `AIHub-1.1.4-x64.dmg` | 无 Developer ID 签名与公证 |
| Linux x64 / ARM64 | `AIHub-1.1.4-linux-<架构>.AppImage` / `.deb` | 内置同架构服务与采集器 |
| iOS / iPadOS 15+ | `AIHub-1.1.4-unsigned.ipa` | 需要自行签名，不能直接安装 |
| Web / PWA | 服务器 HTTPS 工作台 | Safari 可添加到主屏幕 |

各平台共享浅色界面、提醒状态与便笺同步。iOS 当前没有 APNs 远程推送，也不能在退出应用后持续后台轮询。首次安装与原生通知仍需目标设备验收。

macOS/Linux 桌面包由 `.github/workflows/release.yml` 在对应系统构建；发布前必须取得所有目标包、校验 SHA-256，并在服务器上验证完整下载。

## Windows 桌面版

桌面版是 Electron 外壳，加载本机 `http://127.0.0.1:8080`，界面与浏览器网页版一致，并额外支持：

- 系统托盘与关闭后后台轮询
- Codex 额度原生强提醒弹窗

### 一键启动

```powershell
powershell -ExecutionPolicy Bypass -File scripts/Start-Desktop.ps1
```

会先启动本地 AI Hub 服务、Codex 采集器（若已配置），再打开桌面 App。

### 仅打开已构建 App

**安装版（推荐）**：运行 `scripts/Install-AIHub.ps1`（或 `AIHub-Setup-0.3.3.exe`）。启动时选择 **登录服务器** 或 **离线本地模式**。

**便携版**：解压 `artifacts/aihub-desktop-windows-x64.zip`，需先运行 `scripts/Start-AIHub.ps1` 再打开 `AIHub.exe`。

### 重新打包

```powershell
powershell -ExecutionPolicy Bypass -File scripts/Build-Desktop.ps1
```

产物：
- 安装版：`artifacts/desktop/installer/AIHub-Setup-0.3.3.exe`
- 便携版：`artifacts/aihub-desktop-windows-x64.zip`

## Android 原生 App（Flutter）

Android App 使用 **Flutter 原生 UI**。源码中的 `https://hub.example.com` 是脱敏示例地址，使用前须在应用中设置实际服务器。界面为浅色移动端适配版，功能对齐网页 Codex 工作台（额度总览、提醒中心、便笺、设备）。

### 安装

**公网下载**：最新下载页默认使用 `8888` 端口：

- 下载页：`http://<服务器地址>:8888/`（直接打开最新版下载页）
- Windows：`http://<服务器地址>:8888/downloads/AIHub-正式版v1.1.2.exe`
- Android：`http://<服务器地址>:8888/downloads/AIHub-正式版v1.1.2.apk`
- 电脑采集器：`http://<服务器地址>:8888/downloads/AIHub-Codex-Collector-Windows-latest.zip`
- SHA-256：`http://<服务器地址>:8888/downloads/SHA256SUMS-latest.txt`

若浏览器无法下载：
1. 腾讯云 → 安全组 → 入站放行 **TCP 80、443、8888**（0.0.0.0/0）
2. Clash 等代理将服务器地址设为 **DIRECT**，或暂时关闭 TUN
3. 电脑端用 SSH 下载（不依赖 HTTP 端口）：`powershell -File scripts/Download-Apk.ps1`
4. 手机建议用 **4G/5G** 访问上述 `8888` 链接

历史 APK 曾包含编译期自动登录配置，已废弃且不可分发。使用脱敏源码重新构建的 APK 不再嵌入账号密码；登录时由用户输入。

**一键验收**（桌面 + 云端 + 本地）：

```powershell
powershell -ExecutionPolicy Bypass -File scripts/Verify-Clients.ps1
```

本地构建包：`artifacts/android-native/aihub-mobile.apk`

### 重新打包

```powershell
powershell -ExecutionPolicy Bypass -File scripts/Build-Flutter-Android.ps1
```

产物：`artifacts/android-native/aihub-mobile.apk`

### 旧版 WebView 壳（已弃用）

`artifacts/android-web/aihub-web-shell.apk` 为早期 WebView 试验包，请改用 Flutter 原生包。

## 对比

| 客户端 | 连接目标 | 适用场景 |
| --- | --- | --- |
| Windows 桌面版 | 本机 127.0.0.1:8080 | 电脑本地精美运行 + 原生提醒 |
| Android 原生 App | 公网 HTTPS | Flutter 原生 UI，默认公网服务器 |
| 手机浏览器/PWA | 公网 HTTPS | 无需安装 APK，添加到主屏幕 |
