# AI Hub 跨平台真机 QA 清单（Ubuntu / macOS）

覆盖桌面端 Linux（Ubuntu 22.04/24.04）与 macOS（Apple Silicon 优先）版本的安装、云模式、桥接采集、强提醒回归。Windows 版本不在本文范围。

## 0. 获取测试产物

- **CI 产物（推荐）**：在 GitHub Actions 手动触发 `Release Packages` 工作流（或推 `v*` tag），从 run 的 artifacts 下载 `aihub-linux-x64`（AppImage/deb + 服务端二进制）与 `aihub-darwin-arm64`（dmg/zip）。
- **本地构建**：任一平台执行 `bash scripts/build.sh`，然后 `bash scripts/stage-desktop-binaries.sh linux amd64`（或 `darwin arm64`）+ `npx electron-builder --linux|--mac`。

## 1. 自动化冒烟（先跑这个）

### Ubuntu

```bash
sudo ./scripts/qa-smoke-linux.sh --deb ./AIHub-1.0.0-amd64.deb          # deb 全量（含本地模式）
./scripts/qa-smoke-linux.sh --appimage ./AIHub-1.0.0-x86_64.AppImage   # AppImage（无 sudo 项自动降级）
```

脚本自动完成：安装/解包 → 云模式对本地桩服务器（`127.0.0.1:18080`）启动 → 校验窗口出现 → `aihub-bridge --probe codex` 离线采集探针 → 本地模式全链路（预置 PostgreSQL 角色后由应用拉起内置服务端，`/ready` 返回 ok）。

**通过标准**：`PASS=… FAIL=0`。无显示器环境需预装 `sudo apt install xvfb xdotool`。

### macOS

```bash
./scripts/qa-smoke-macos.sh --dmg ./AIHub-1.0.0-arm64.dmg
```

脚本自动完成：dmg 挂载 → 包结构（Contents/MacOS、Resources/bin 两个内置二进制 +x）→ Gatekeeper 状态（未签名会打 WARN，属预期）→ 启动存活 → bridge 探针 → 本地模式（Homebrew PostgreSQL + `/ready`）。

**通过标准**：`PASS=… FAIL=0`（WARN 不算失败，但未签名分发要在手动清单中确认绕行流程）。

## 2. Ubuntu 手动清单

| # | 项 | 步骤 | 预期 |
|---|---|---|---|
| U1 | 图标与元信息 | 安装 deb 后查看应用菜单 | 出现 AI Hub 图标（Utility 分类） |
| U2 | 云模式登录 | 输入管理员 HTTPS 地址 | 加载 Web UI，可登录 |
| U3 | 桥接自动发现 | 安装 Codex CLI 后登录 | `bridge.json` 的 `codex_path` 自动填入 PATH 中的 codex |
| U4 | 额度上报 | 等待采集周期（默认 5 分钟） | 管理端可见额度数据 |
| U5 | 强提醒 | 制造额度告警（或 Ctrl+Shift+T 演示） | 弹窗正常显示；Linux 上 `flashFrame` 无效属预期 |
| U6 | 托盘 | 关闭窗口 | Ubuntu 22.04+ 默认带 AppIndicator 支持，托盘可见；关闭=隐藏，再次启动唤回 |
| U7 | GNOME 无托盘兜底 | 在 GNOME 无扩展的环境验证 | 托盘创建失败时关闭窗口=真正退出，应用不"失踪" |
| U8 | 本地模式引导流 | 全新环境选"本地开发" | 弹窗给出 `sudo -u postgres psql -f <sql>` 命令；执行后从托盘「重新连接」能起服务 |
| U9 | 自启 | Linux 上不注册自启（v1 预期行为） | 登录后不自动启动，无报错 |
| U10 | 凭据保护 | 无钥匙环环境（i3 等 WM） | 报"系统凭据保护不可用"并含指引，不崩溃 |
| U11 | Cursor 采集 | 安装 Cursor 登录后 probe | `aihub-bridge --probe cursor` 能读到 token（新增 unix 支持） |

## 3. macOS 手动清单

| # | 项 | 步骤 | 预期 |
|---|---|---|---|
| M1 | 首次启动 | dmg 拖入 Applications，右键 → 打开 | 绕过 Gatekeeper 正常启动（未签名） |
| M2 | 钥匙串授权 | 首次登录/配对 | Keychain 弹窗允许后，`safeStorage` 可用 |
| M3 | 自启 | 系统设置 → 通用 → 登录项 | 出现 AI Hub（`setLoginItemSettings`） |
| M4 | 托盘 | 观察菜单栏 | 图标可见（当前为 256px 彩色图，后续换 template image 更协调） |
| M5 | 强提醒 | Ctrl+Shift+T 或真实告警 | 弹窗 + Dock 图标弹跳（`flashFrame`） |
| M6 | 关闭行为 | 点红色关闭钮 | 窗口隐藏，应用驻留菜单栏；Dock 图标保留 |
| M7 | 本地模式 | 安装 `brew install postgresql@16` 后选本地开发 | 全自动建角色建库并起服务（当前用户即超级用户，无需 sudo） |
| M8 | Codex 桥接 | 登录后检查 | 自动发现 `/opt/homebrew/bin`、`~/.codex/bin` 等路径下的 codex |
| M9 | Cursor 采集 | 安装 Cursor 登录后 probe | 能读到 `~/Library/Application Support/Cursor/.../state.vscdb` 的 token |
| M10 | 签名复验（拿到 Developer ID 后） | 公证构建跑本脚本 | Gatekeeper 直接放行，M1 的绕行步骤不再需要 |

## 4. 已知限制（验收时不要当 bug 报）

- macOS 包未签名/未公证：首次打开需右键 → 打开，或 `xattr -dr com.apple.quarantine /Applications/AI\ Hub.app`。
- 图标源文件仅 256px：Dock 图标在放大时略糊；用 ≥1024px 源图重跑 `node scripts/make-icns.cjs` 即可提升。
- Ubuntu 本地模式首次使用需手动执行一条 sudo 命令（设计如此，见 U8）。
- Linux 上 `flashFrame` 是 no-op；强提醒可见性依赖弹窗本身。
- 冒烟脚本 kill 应用后可能残留 `aihub-server` 进程，脚本已用 `pkill` 兜底。

## 5. 回归范围备忘

- 桌面端单测：`apps/desktop` 下 `node <name>.test.cjs`（main / platform / platform.linux / platform.darwin，Linux/macOS 分支在任意主机可跑）。
- 服务端：`go test ./...`（DB 相关用例无 `AIHUB_TEST_DATABASE_URL` 时自动跳过，完整 gate 走 CI）。
