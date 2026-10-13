# AI Hub Windows 采集器

此独立更新包包含 Codex/Cursor 本机只读采集器及启动、停止脚本。默认每 1 分钟采集；旧版配置中的 300 秒默认值会自动按 60 秒运行。

1. 在 AI Hub **电脑采集器**页面生成连接配置，保存为本包 `bridge\bridge.json`。该配置包含专用连接令牌，请勿分享。
2. 确保本机 Codex 已登录；如使用 Cursor，也需在本机登录 Cursor。
3. 在解压目录运行：

   ```powershell
   powershell -ExecutionPolicy Bypass -File .\bridge\Start-Codex-Bridge.ps1
   ```

4. 页面每分钟同步一次；单次采集失败时暂时保留上次有效数据，连续失败才报告采集不可用。超过 3 分钟未收到新数据会标记为过期。停止时运行 `bridge\Stop-Codex-Bridge.ps1`。

采集器在后台运行，只上传额度摘要。日志保存在 `bridge` 文件夹；不会上传提示词、对话、登录凭据或终端输出。
