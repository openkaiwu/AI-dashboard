# Change Hints WebSocket 收口（INH-513，2026-10-02）

## 背景

M5 的 change hints v1 以 SSE 交付（`GET /api/v1/change-hints`：每用户 256 条内存缓冲、`Last-Event-ID` 重连重放、仅粗粒度指针），WebSocket 升级按契约预留为"同语义的传输替换"。本次按契约完成收口：同一路由同时支持 WebSocket 与 SSE，语义完全一致。

## 实现

### 服务端

| 文件 | 内容 |
|---|---|
| `server/internal/workspace/hub.go` | 抽出 `subscribe`/`unsubscribe` 公共逻辑；`Stream` 按请求头分流：带 `Upgrade: websocket` 走 WS，否则维持 SSE |
| `server/internal/workspace/websocket.go` | WS 处理器：升级握手后按 SSE 相同顺序发送回放事件与 hello；15 秒协议级 ping 心跳；读协程失败即断开；`CheckOrigin` 与 `httpx.CORS` 同源策略一致 |
| `server/internal/api/server.go` | `wsBearer` 中间件（仅此路由）：浏览器无法在 WS 握手设置请求头，允许以 `access_token` 查询参数传递访问令牌 |
| `server/internal/httpx/httpx.go` | 日志包装器 `statusWriter` 补充 `Hijack`/`Unwrap`，使 WS 升级能穿透中间件链 |

语义保持不变：hello 标记、seq 单调、256 条重放窗口、payload 只含粗粒度指针。恢复连接时原生客户端用 `Last-Event-ID` 头，浏览器客户端可用 `last_event_id` 查询参数。

### Web 客户端（桌面 exe 共用）

| 文件 | 内容 |
|---|---|
| `apps/web/src/changeHints.ts` | 订阅封装：优先 WebSocket；WS 不可用（代理拦截、旧服务器）自动降级 SSE，并在 5 分钟后重试 WS；断线 30 秒重连 |
| `apps/web/src/App.tsx` | 登录后订阅，事件派发为 `aihub-change-hint` 自定义事件 |
| `apps/web/src/pages/Workspaces.tsx` | 监听 `comment`/`member_removed` 提示并即时刷新列表，不再依赖页面重进 |

## 验证

- `server/internal/api/change_hints_ws_test.go`（真实 PostgreSQL）：未认证握手 401；建工作区+共享会话+评论后 WS 实时收到 `comment` 事件（seq 连续）；断开期间漏掉的事件经 `last_event_id=seq` 重连精确重放，随后收到 hello；SSE 通道保持可用（`text/event-stream` + retry + hello）。
- `go test ./...` 全量通过；`go vet ./...` 通过。
- `npm run build`（tsc 类型检查）与 vitest 9/9 通过，dist 已同步至 `server/webassets/dist`。

## 边界与后续

- 移动端 App 目前仍为轮询刷新，未接 WS（本机无 Flutter 编译环境）；服务端完全向后兼容，App 可后续独立接入。
- 访问令牌经查询参数传输仅限此路由：令牌 15 分钟过期、可撤销，且 httpx 日志只记录路径不记录查询串；如需进一步收敛可改走 Cookie 或首帧鉴权。
- 跨实例 hub 同步仍为单实例内存实现（多实例部署需引入共享广播，属 M7+ 范围）。
