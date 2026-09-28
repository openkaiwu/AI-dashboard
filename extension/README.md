# 固定页面扩展 PoC

1. 在 Chrome 的扩展管理页以开发者模式加载本目录。
2. 桌面应用登录并启动 Bridge，在 Cursor 连接页复制配对码。
3. 从 `extension/fixtures` 启动本机静态 HTTP 服务，打开 `cursor.html`。
4. 点击扩展，输入配对码，授权当前站点并同步。

扩展只提取 `data-aihub-sample="cursor"` 上的 `data-used-percent`。它不读取 Cookie、凭据、网页正文或对话内容。服务端把样本写入独立的 `connector_samples`，不计入真实额度。真实 Cursor 页面结构未验证，适配器保持关闭。
