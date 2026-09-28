# 桌面版与 Android 原生 App

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

**公网下载（推荐）**：

- **HTTP 80**：http://203.0.113.10/downloads/aihub-mobile.apk
- **HTTP 8888（备用）**：http://203.0.113.10:8888/downloads/aihub-mobile.apk
- **HTTPS**：https://hub.example.com/downloads/aihub-mobile.apk（部分 Windows 因 IP 证书失败）

若浏览器无法下载：
1. 腾讯云 → 安全组 → 入站放行 **TCP 80、443、8888**（0.0.0.0/0）
2. Clash 等代理将 `203.0.113.10` 设为 **DIRECT**，或暂时关闭 TUN
3. 电脑端用 SSH 下载（不依赖 80 端口）：`powershell -File scripts/Download-Apk.ps1`
4. 手机建议用 **4G/5G** 访问上述链接

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
