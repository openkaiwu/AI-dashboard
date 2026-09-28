# Windows App 与额度强提醒

## 使用

**推荐：一键安装** — 在项目根目录运行：

```powershell
powershell -ExecutionPolicy Bypass -File scripts/Install-AIHub.ps1
```

或手动运行 `artifacts/desktop/installer/AIHub-Setup-0.3.3.exe`。安装后双击 **AI Hub** 即可：

1. 首次选择「登录服务器」后填写管理员提供的 HTTPS 根地址；选择本地开发模式需要 WSL、PostgreSQL 和已初始化的管理员。
2. 用管理员授权的账户登录。该安装绑定为桌面设备；再次启动恢复有效会话并自动连接 Bridge。
3. 关闭窗口保留托盘和采集。托盘可打开、重连、切换使用方式和明确退出；退出会停止本应用管理的进程。本轮不配置开机自启。
4. 连接失败时提示重试或修改地址，不会自动切到另一个本地账户。

配置与日志保存在 `%APPDATA%/AI Hub/runtime/`：

| 文件 | 作用 |
|------|------|
| `sessions.protected` | 由操作系统安全存储加密的会话 |
| `bridge-token.protected` | 由操作系统安全存储加密的上报凭据 |
| `bridge.json` | 不含令牌的 Codex/Cursor 采集器配置 |
| `app-config.json` | 覆盖默认云端/本地模式 |

桌面端不从开发目录复制账户凭据或数据库配置。Bridge 会在登录后自动配对，并受该桌面设备的解绑和禁用操作约束。

**便携版** — 解压 `artifacts/aihub-desktop-windows-x64.zip` 后仍可用，但需自行运行 `scripts/Start-AIHub.ps1` 启动本地服务。

### 托盘与提醒

- 关闭窗口隐藏到托盘，继续轮询。托盘菜单可打开、重新连接服务、退出（退出时停止本应用启动的本地服务与 bridge）。
- 强提醒默认开启。额度消耗过快、临近重置仍有余量、重置卡临近到期，弹出原生警告窗口并提示任务栏。优先级高的先处理，每次一个弹窗。
- 支持已读、1 小时后提醒、忽略本次；后两者同步到同账户其他设备。已读只表示此设备本次已看过，下一次服务端冷却到期后仍可提醒。
- 尊重账户提醒开关、免打扰时间。新闻不会触发强提醒。默认 23:00–08:00 免打扰；需要全天提示时把起止小时设为相同值。
- 提醒设置中的“测试强提醒”可随时测试，即使在免打扰期间；测试不修改真实提醒。App 内 Ctrl+Shift+T 也能测试原生弹窗，无需登录。
- 浏览器使用模态弹窗；所有登录后页面均生效。浏览器关闭、App 退出、电脑休眠、服务不可达时均无法保证送达。原生手机后台推送未实现。

### Codex 采集器

安装版在已绑定的桌面会话登录后自动创建连接。若本机已安装 Codex CLI，程序会自动查找 `codex.exe`；即使未找到 Codex，Cursor 采集仍可运行。浏览器扩展固定样本 PoC 的配对码在应用内 Cursor 连接页查看。

## 前置条件

- 连接已有 HTTPS 服务器无需本机 WSL 或 PostgreSQL。本地开发模式需要 **WSL** 与 **PostgreSQL 14**，并由管理员初始化账户。

## 关于停止任务

本版提供“打开 Codex 手动停止”，打开 Codex 后由用户选择对应任务并停止，不自动结束进程。

当前桥接只采集账户额度，不能把额度百分比等同于具体任务 token 消耗，也没有桌面已有任务的活动控制连接。官方 app-server 文档存在 thread/tokenUsage/updated 和 turn/interrupt，但正确停止需要同一运行实例内准确的 threadId / turnId，收到 turn/completed interrupted 后才能确认成功。

因此本版没有声称支持直接关闭某个高消耗任务。未来需要显式接入或由本应用托管的任务执行通道、每任务 token 增量与时间窗、准确活动轮次映射、用户指定停止和结果确认。不会以杀死全部 Codex 进程替代。

参考：https://learn.chatgpt.com/docs/app-server

## 构建

**安装版（NSIS）**

```powershell
.\scripts\Build-Desktop-Installer.ps1
```

产物：`artifacts/desktop/installer/AIHub-Setup-0.3.0.exe`

**便携版**

```powershell
.\scripts\Build-Desktop.ps1
```

产物：`artifacts/aihub-desktop-windows-x64.zip`

桌面渲染启用 sandbox、contextIsolation、关闭 Node 集成；原生 IPC 校验主窗口及本地来源，不暴露任意命令执行接口。源码、Go 服务运行包与桌面包分别发布。桌面包未进行代码签名。

## 本次验证

Web 构建及 9 项前端测试通过；桌面 IPC 来源校验、按钮映射和单弹窗串行测试通过。安装版内置 `runtime.cjs` 负责 WSL/PostgreSQL/服务/bridge 自启动。实际启动 Windows x64 App，Ctrl+Shift+T 弹出了原生警告对话框，点击已读正常关闭。
