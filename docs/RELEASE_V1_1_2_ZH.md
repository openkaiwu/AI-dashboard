# AI Hub Windows v1.1.2 采集器更新

Windows 桌面版及独立采集器默认每分钟采集 Codex/Cursor 额度；网页和 Android 原生页面每分钟同步，Codex 快照在 3 分钟无新数据后标记过期。原有 300 秒默认配置由新采集器自动按 60 秒运行。Codex 单次返回空额度时保留上次有效数据，连续失败才报告采集不可用。

下载统一使用 `http://<服务器地址>:8888/`：

- Windows 安装包：`/downloads/AIHub-正式版v1.1.2.exe`
- 独立采集器：`/downloads/AIHub-Codex-Collector-Windows-latest.zip`
- Android 客户端：`/downloads/AIHub-正式版v1.1.2.apk`（构建号 6，与 v1.1.1 相同的调试签名，支持覆盖升级；未使用商店发布签名）
- 校验清单：`/downloads/SHA256SUMS-latest.txt`

独立采集器包不包含连接令牌。新安装须从应用内生成 `bridge.json`，放到解压包的 `bridge` 目录；已经安装桌面版的用户可直接覆盖安装 v1.1.2。

验收：Go 全量测试、网页 18 项测试、Flutter 测试、Windows 桌面测试均通过；服务端 `/ready` 正常。本机采集器连续按约 1 分钟间隔成功上报，单次空结果后下一轮恢复。三种发布包均通过 `8888` 端口完整下载并与 SHA-256 校验清单一致。Windows 安装包无数字签名。当前没有连接 Android 真机，因此尚未做真机安装与通知验收。
