# Codex 电脑 → 移动端连接

电脑运行独立 `aihub-bridge`，调用本机已登录的 Codex `app-server`，通过官方 `account/rateLimits/read` 读取额度；用独立连接凭据主动向 AI Hub 上报。手机/PWA/Flutter 登录同一 AI Hub 账户，从服务端读取，并缓存最新摘要。部署后电脑和手机只需都能访问同一 HTTPS 地址，无需同一局域网。

本阶段同步额度窗口、已用百分比、重置时间、采集时间及状态；不含聊天记录、工作目录、任务内容、终端输出、账号邮箱或 Codex 登录凭据。不支持手机远程控制 Codex。持续采集由独立程序完成，不依赖当前聊天。

官方协议：[Codex App Server](https://learn.chatgpt.com/docs/app-server)。按实际返回展示：primary 不一定是 5 小时窗口，null 表示未知，不能当作剩余 100%。优先读取多额度桶。

## Windows 使用

1. 在电脑正常登录 Codex，并启动 AI Hub。
2. 在 AI Hub 网页 **Codex 连接** 页面输入电脑名称，点击 **生成连接配置**，下载 `bridge.json`。配置只在创建后提供，请勿分享。服务器只保存凭据哈希。
3. 解压运行包，把配置放到其中 `bridge/bridge.json`，在 PowerShell 运行：

   ```powershell
   powershell -ExecutionPolicy Bypass -File .\bridge\Start-Codex-Bridge.ps1
   ```

   自动查找桌面 Codex，限制配置文件为当前 Windows 用户可访问，隐藏启动采集程序。首次采集最长约 40 秒，随后每 5 分钟采集。特殊安装路径可传入 `-CodexPath '完整的 codex.exe 路径'`。
4. 网页 **Codex 连接** 查看状态。Flutter 底部新增 **Codex** 页面。移动端填写相同服务地址、登录同一个 AI Hub 账户。
5. 停止本地采集：运行 `bridge/Stop-Codex-Bridge.ps1`。终止凭据：网页 **撤销此连接**；采集程序收到 401 即退出。退出网页登录与撤销采集连接相互独立。

源码工作区可指定 `-ConfigPath .\.runtime\bridge.json`。本机配置放在 `.runtime`，不进入 ZIP。不设置系统开机自启；重启电脑后再次运行启动脚本。Codex 更新导致路径变化时，传入新路径重新启动。

## Linux / 自定义运行

配置中 `codex_path` 为本机 Codex 可执行文件绝对路径：

```sh
chmod 600 bridge.json
./aihub-bridge-linux-amd64 --config ./bridge.json
# 单次联调
./aihub-bridge-linux-amd64 --config ./bridge.json --once
```

`CODEX_HOME` 继承进程环境；使用自定义目录时，启动前设置为实际目录。不要将 auth.json 复制到服务器。

## 同步和权限

- 采集程序不监听网络端口，服务端不能下发 Codex 操作。
- 专用 token 只能上报自己的快照，不能读取 AI Hub 数据。手机以正常账号会话读取所属账户的连接。
- 拒绝未声明字段、越界百分比、未来超过两分钟的采集时间。请保持设备时钟准确。
- 按采集时间更新，重复或更早快照不覆盖新数据。这是最新状态同步，不保存每个历史采样。
- 网络失败下一周期重新采集上传，不积压过时额度。采集失败上报 unavailable，不伪装为已用 0%。
- 网页和 Flutter 每 5 分钟拉取，缓存按服务器及用户隔离。超过 11 分钟标为过期。撤销在下一次在线拉取可见；离线缓存无法获知远端撤销。
- 当前本地服务只监听 127.0.0.1。手机无法用自己的 127.0.0.1 访问电脑；公网 HTTPS 按用户决定后续配置。回环验收不等于跨公网验收。

| API | 认证 | 用途 |
|---|---|---|
| POST /api/v1/codex/bridges | AI Hub 会话 | 创建连接，一次返回 token |
| GET /api/v1/codex/bridges | AI Hub 会话 | 当前用户连接和最新快照 |
| DELETE /api/v1/codex/bridges/{id} | AI Hub 会话 | 撤销连接 |
| POST /api/v1/codex/snapshot | 连接 token | 严格白名单上报 |

使用专用快照接口，未复用便笺增量事件流；共享账户和服务端。无 cookie 导入，无第三方凭据同步。

## 智能提醒增量

首页现为额度节奏助手，配对入口移到 `/connections`。新增重置卡数量/到期与公开消息摘要白名单、7 天采样历史及账户规则。详见 [Codex 智能提醒](CODEX_ADVISOR_ZH.md)，其中的新行为优先于本文初版“仅最新状态”的说明。
