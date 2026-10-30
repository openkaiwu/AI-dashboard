# Windows 时区切换

复用本机 `D:/时区软件/windows` 的 Windows v1.0.0 切换逻辑，集成日期 2026-10-30。AI Hub 左侧入口切到应用内部 `/timezone` 页面，显示两地时钟与系统实际时区。

构建：仓库根目录执行 `scripts/Stage-TimeZone-Tool.ps1`。输出位于忽略提交的 `apps/desktop/resources/timezone`，作为安装包外部资源携带。安装后不依赖原项目目录。

检查：执行本目录的 `test.ps1`，覆盖中国/洛杉矶标识、冬夏偏移、夏令时边界、状态判定和 DPI 布局，不实际修改系统时区。

组件支持 Windows 10/11、.NET Framework 4.8。后台 `--status` 输出 JSON 系统状态；`--switch cn|la` 执行切换、按需要申请管理员权限并输出核验结果。提权子进程使用原 `--apply` 协议，保留自动校时及失败回滚。AI Hub 不启动独立工具窗口，仅在点击城市按钮后执行切换；Windows 必要的系统授权提示由用户确认。
