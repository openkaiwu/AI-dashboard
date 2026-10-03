# AI Hub iOS 版构建与侧载指南

对齐 Android 版功能集（账号绑定、同步引擎、额度/Cursor 页、强提醒），产物为**可侧载的未签名 ipa**。最低支持 iOS 15.0，Bundle ID `local.aihub.aihubMobile`。

## 1. 产物获取

- **CI（推荐）**：GitHub Actions `Release Packages` 工作流（手动触发或推 `v*` tag）的 `aihub-ios-unsigned` artifact，内含 `AIHub-<版本>-unsigned.ipa` + SHA256。
- **Mac 本地构建**：
  ```bash
  cd apps/mobile
  flutter pub get
  flutter build ios --release --no-codesign   # 未签名，用于重签侧载
  # 或带签名：在 Xcode 中打开 ios/Runner.xcworkspace，选择 Team 后 Build
  ```

> 构建要求 macOS + Xcode 16+（插件走 Swift Package Manager 集成，本项目已无 Podfile）。Windows/Linux 上无法产出 iOS 包。

## 2. 四种侧载/安装路径

| 路径 | 前提 | 有效期 | 适用 |
|---|---|---|---|
| A. Xcode 直装 | Mac + 免费 Apple ID，`ios/Runner.xcworkspace` 选 Personal Team 后 Run 到设备 | **7 天**，到期重装 | 开发自用、真机调试 |
| B. Sideloadly / AltStore 重签 | Windows 或 Mac + 免费 Apple ID（Sideloadly 需 iTunes+iCloud 驱动） | 7 天 | 无 Mac 时分发测试（用本指南 CI 产的未签名 ipa） |
| C. TrollStore | 设备 iOS 14.0–16.6.1 / 17.0 特定版本 | 永久 | 越狱向用户 |
| D. 付费开发者账号（$99/年） | Ad Hoc：注册设备 UDID，1 年有效；TestFlight：内测最多 1 万人，90 天/版本 | — | 正式内测分发 |

未签名 ipa 重签命令（有 Mac + 证书时）：

```bash
unzip AIHub-1.0.0-unsigned.ipa
codesign -f -s "Apple Development: <你的证书>" --entitlements ent.plist Payload/AIHub.app
zip -r resigned.ipa Payload
```

免费 Apple ID 没有 Apple Development 证书时直接用 Sideloadly/AltStore 的自动重签即可，无需手动 codesign。

## 3. 与 Android 版的功能对齐

| 能力 | Android | iOS |
|---|---|---|
| 账号绑定/会话 | ✓ | ✓（同一套 Dart 代码，凭据存 iOS Keychain） |
| 本地库/同步引擎 | Drift/SQLite | ✓ 同一套代码 |
| 强提醒弹窗 | 通知渠道 + full-screen intent | Darwin 类别 `aihub_reminder` + time-sensitive 打断级别 + 横幅 |
| 提醒动作按钮 | 已读 / 1 小时后提醒 / 忽略本次 | 同三个动作，action id 完全一致（`read`/`snooze`/`dismiss`） |
| Cursor 页 | 服务器数据 | ✓（iOS 无桌面 Cursor，本地采集只在桌面端桥接完成） |
| 首次通知授权 | `requestNotificationsPermission` | `requestPermissions(alert,sound,badge)`，App 首启触发系统弹窗 |

## 4. QA 清单（装到真机后）

| # | 项 | 预期 |
|---|---|---|
| I1 | 首启授权 | 弹出通知权限弹窗；允许后 `flutter_local_notifications` 初始化成功 |
| I2 | 登录 | 输入服务器地址（默认公网 AI Hub），账号登录成功，Keychain 持久化（杀 App 重启免登） |
| I3 | 同步 | 额度/收件箱数据与 Web 端一致 |
| I4 | 强提醒 | 后台收到告警 → 横幅+声音；长按出现三个动作按钮 |
| I5 | 动作回调 | 点「已读/稍后/忽略」回传 action id（与 Android 相同的 `read/snooze/dismiss`） |
| I6 | 时间敏感 | 专注模式下提醒可突破（需在系统设置允许"时间敏感通知"） |
| I7 | 前台弹窗 | App 在前台时走应用内强提醒 UI（strong_reminder_ui），不依赖系统通知 |
| I8 | 侧载有效期 | 免费签名 7 天后确认过期提示与重装流程 |

## 5. 已知限制

- 未签名 ipa 不能直接双击安装；必须经上述 A–D 路径之一。
- 免费签名 7 天限制是 Apple 政策，非本工程问题。
- iOS 无全屏 intent 等价物；强提醒在锁屏时以时间敏感横幅呈现。
- 图标仍为 Flutter 模板占位图；替换 `ios/Runner/Assets.xcassets/AppIcon.appiconset` 即可。
- 后台长驻同步依赖 App 前台（与 Android 版一致，未接 WorkManager/BGTaskScheduler）。

## 6. CI 侧载包的构建细节

`release.yml` 的 `ios` job：`flutter analyze` + 单测（`real_server_test.dart` 需要 gate 服务器+PostgreSQL，由 ubuntu gate 覆盖，此处不重复）→ `flutter build ios --release --no-codesign` → `Payload/AIHub.app` 打包为 `AIHub-<版本>-unsigned.ipa` + SHA256SUMS-ios.txt。
