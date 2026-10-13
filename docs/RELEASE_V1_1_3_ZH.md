# AI Hub v1.1.3 发布说明

- Windows 桌面端：`AIHub-正式版v1.1.3.exe`，x64，版本 1.1.3。
- Android：`AIHub-正式版v1.1.3.apk`，版本 1.1.3，构建号 7。
- 下载页：`http://YOUR_SERVER_IP:8888/`；版本清单：`/downloads/SHA256SUMS-v1.1.3.txt`，最新清单：`/downloads/SHA256SUMS-latest.txt`。

本版包含桌面与移动端的浅色响应式界面更新、消耗分析的五小时实时/24 小时/7 天/30 天视图及无样本、部分数据、过期数据和错误状态。Plus 用户显示五小时额度，Pro 用户隐藏五小时额度。服务端已更新并保留现有数据库和环境配置。

桌面安装器已在本机覆盖安装，文件版本为 1.1.3 并已启动。Android APK 使用当前调试签名、未使用商店签名；本机没有连接 Android 设备，未做真机覆盖安装。

SHA-256：

- Windows：`$exeHash`
- Android：`$apkHash`

部署后 `/ready` 返回 200；发布脚本已在服务器验证版本化安装包和 SHA-256 清单。HTTP 下载路由已返回完整长度并通过 Range 请求读取包内容。
