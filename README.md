# AI Hub

自部署的 AI 服务资产中枢。当前仓库实现的是 **Web 端额度追踪 MVP**：登录后手工登记各平台套餐与额度，看板按状态排列，规则引擎生成提醒，并可查看用量历史。

配套设计见 `docs/AI_Aggregation_Platform_Technical_Architecture.md` 与 `docs/AI_Aggregation_Platform_Development_Roadmap.md`。

## 运行

本机需要 Go 1.22+ 与 Node.js 20+。

```bash
# 终端 1：API + SQLite（含演示数据）
make run-demo

# 终端 2：Web
make web-install
make web-dev
```

打开 http://127.0.0.1:5173

演示账户：`demo@aihub.local` / `demo1234`

## 这个版本覆盖什么

- 注册 / 登录
- 5 个预置 Provider（OpenAI、Anthropic、Gemini、Cursor、GitHub Copilot）
- 手工登记账户、套餐、额度桶、重置/过期时间
- 状态计算：正常 / 额度不足 / 即将重置 / 即将过期 / 数据过期 / 未知
- 五类提醒规则 + 预览 + 去重
- 通知收件箱
- 7/30 天用量快照

本地数据库文件：`data/aihub.db`。单进程即可跑通，生产环境的 PostgreSQL 与 Flutter 客户端按架构后续接入。
