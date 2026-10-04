# 额度消耗分析图（每日 / 每周 / 每月）详细设计

状态：P1/P2 已实现并上线（migration 024 + 消耗 API + Web 消耗分析 tab）；移动端视图与 P3 候选项（热力图/导出/异常提醒）尚未实现。

## 一、目标与非目标

**目标**
1. 把"额度消耗"从单点百分比升级为**时间序列视图**：当前窗口（实时）、每日、每周、每月四个粒度。
2. 全部账户可用（含 Pro Lite 这类只上报周窗口的套餐；含纯手机/网页的零设备用户——只读自身数据，无桥接时显示无数据说明）。
3. 零第三方依赖：图表全部沿用现有 `Trend` 组件的手绘 SVG 方案。
4. 数据诚实：无样本的时段留空，绝不插值伪造；重置不当作消耗。

**非目标**
- 不做绝对量（token/消息条数）估算——上游只给百分比；
- 不做跨账户对比；
- 不做导出 CSV（三期候选）。

## 二、数据基础与缺口

| 数据源 | 现状 | 缺口 |
|---|---|---|
| `codex_history`（bridge_id, observed_at, snapshot JSONB） | 每桥 ~5 分钟一个样本，**保留 7 天**（每次上传时修剪） | 覆盖不了"每周/每月"视图的时间跨度 |
| advisor 报告里的 `history` | 只回传最近 **6 小时** | 当前窗口全景需要"自窗口起点起"的全部样本 |
| 套餐窗口能力 | prolite 只上报周窗口（10080min），plus/pro 预期 5h+周 | 五小时视图必须按套餐自适应 |

**结论**：必须新增**降采样聚合层**（小时粒度 rollup），并明确"月/周视图从上线之日起逐步积累，无法回填 7 天以前的历史"。

## 三、数据模型

### migration 024

```sql
CREATE TABLE codex_usage_rollup (
    bridge_id   TEXT        NOT NULL REFERENCES codex_bridges(id),
    hour_bucket TIMESTAMPTZ NOT NULL,              -- UTC 小时桶
    window_key  TEXT        NOT NULL,              -- bucket_id + ':' + duration_minutes，如 'codex:10080'
    consumed_pp NUMERIC     NOT NULL DEFAULT 0,    -- 本小时内消耗的百分点（只累计，不含重置跳变）
    samples     INTEGER     NOT NULL DEFAULT 0,
    resets      INTEGER     NOT NULL DEFAULT 0,    -- 本小时内发生的窗口重置次数
    level_last  NUMERIC,                           -- 本小时最后一个 used_percent
    PRIMARY KEY (bridge_id, hour_bucket, window_key)
);
```

- **保留 400 天**（rollup 修剪挂在既有修剪处）；
- 不存 user_id：查询时按 user → bridges 联表（与现状一致，1 账户 1 电脑）；
- `hour_bucket` 用 UTC 存储，展示层按账户时区（`codex_preferences.settings.utc_offset_minutes`，默认 +480）换算分桶。

### 聚合写入（快照接收后的异步小事务，与雷达晋升同一位置）

每次 `Upload` 成功且 `applied=true` 后（`connector/codex.go`，雷达晋升旁）：

```
prev = 该桥上一份快照（Upload 事务里已有的 old）
对 q.Buckets 里每个窗口 w：
  key = w.ID + ':' + w.DurationMinutes
  若 prev 中存在同 key 且 resets_at 相同（同一代窗口）：
      delta = clamp(w.UsedPercent - prev_w.UsedPercent, 0, 100)
  否则（新代窗口=发生了重置）：
      delta = 0；resets += 1
  hour = q.ObservedAt 截断到小时
  UPSERT codex_usage_rollup：consumed_pp += delta, samples += 1, resets += resets, level_last = w.UsedPercent
```

要点：
- **best-effort**：放在 commit 后独立事务，失败只记日志（与雷达晋升同模式），绝不影响额度上报；
- 同秒去重依赖既有 `applied` 判定（重试样本不会重复计数）；
- `used_percent` 的平台侧修正（非重置的下降）会被 clamp 丢掉——宁可少计不可多计。

### 回填

上线时对 `codex_history` 现存的 ≤7 天原始样本跑一次回填任务（复用同一计算逻辑，按 observed_at 顺序重放），使"每日"视图第一天起就有近 7 天数据。"每周/每月"视图从 0 开始积累，UI 明示"积累中（自 YYYY-MM-DD 起）"。

## 四、API 设计

### `GET /api/v1/codex/consumption`

鉴权：会话中间件（与 overview 相同）。查询参数：

| 参数 | 取值 | 默认 |
|---|---|---|
| `granularity` | `window`（当前窗口）/ `daily` / `weekly` / `monthly` | `daily` |
| `window` | `10080` / `300` / 缺省=全部已上报窗口 | 全部 |
| `tz_offset` | 分钟（-720~840） | 账户偏好里的 utc_offset_minutes |

响应（示例为 daily）：

```json
{
  "generated_at": "2026-10-04T12:00:00Z",
  "tz_offset_minutes": 480,
  "windows": [{"id": "codex", "duration_minutes": 10080, "present": true},
               {"id": "codex", "duration_minutes": 300, "present": false, "reason": "该套餐当前未上报五小时窗口"}],
  "series": [
    {"bucket_start": "2026-10-01T00:00:00+08:00",
     "consumed_pp": 12.4,          // 当日消耗百分点
     "level_end": 18.0,            // 日终已用水平
     "peak_pp_hour": 3.1,          // 单小时峰值消耗
     "resets": 1,                  // 当日窗口重置次数
     "coverage_hours": 21}         // 有样本覆盖的小时数（0-24）
  ],
  "resets": [{"at": "2026-10-01T05:00:00+08:00", "window": "codex:10080"}],
  "coverage": {"first_day": "2026-09-28", "days_with_data": 6, "accumulating_since": "2026-10-04"}
}
```

- `granularity=window`：返回当前窗口代（按 `resets_at` 分组）内全部原始样本 `{t, used_percent}` + 均匀使用参考线（由现有 `daily_budget` 推导）+ 重置点列表；数据源为 `codex_history`（窗口起点 ≤7 天，恰好被原始保留期覆盖）。
- `granularity=daily`：近 **30 天**，rollup 按日聚合（consumed 求和、level 取日末、peak 取小时最大、coverage = 有样本小时数）。
- `granularity=weekly`：近 **12 个日历周**，按账户时区的周一为周起点。
- `granularity=monthly`：近 **12 个日历月**。
- 数据不足的桶：`consumed_pp=null, coverage_hours=0`（前端渲染为空档，不补零 pretending 有数据）。

## 五、计算规则（唯一权威口径）

1. **消耗（pp）**：同一代窗口内，相邻样本 `used_percent` 的正增量之和。跨代（重置）不累计。
2. **重置判定**：`resets_at` 变化，或 `used_percent` 相对上一样本下降超过 2pp（容错 Codex 侧小幅修正，避免把修正当重置）。
3. **时区**：日/周/月分桶全部按账户时区换算后再截断。
4. **多设备**：当前 1 账户 1 电脑；若未来放开，按桥分别 rollup、查询层求和（样本时间重叠时不求均值——偏保守高估，符合"宁少勿多"）。
5. **prolite 等周窗口套餐**：只出现 `codex:10080` 的序列；五小时相关视图显示套餐能力说明（见下）。

## 六、UI 设计（Web：新 tab「消耗分析」；移动端三期跟上）

入口：CodexOverview 的 tabs 增加 `['usage','消耗分析']`。页面结构：

```
┌────────────────────────────────────────────────────────────┐
│ 窗口选择 chips: [7 天窗口] [5 小时窗口(套餐未上报→禁用)]      │
│ 粒度 segments:  [当前窗口] [每日] [每周] [每月]               │
├────────────────────────────────────────────────────────────┤
│ 统计行（随粒度变化）                                         │
│  总消耗 12.4pp · 日均 3.1pp · 峰值日 10-01(5.2pp) · 重置 1 次 │
│  · 有数据覆盖 21/30 天                                       │
├────────────────────────────────────────────────────────────┤
│ 主图（SVG，手绘，复用 Trend 风格）                            │
│  当前窗口：全代折线 + 均匀使用参考线 + 预计耗尽标记            │
│  每日：    30 根柱（consumed_pp）+ 重置标记 ▲ + 空档留白      │
│  每周：    12 根柱 + 环比箭头（±%）                           │
│  每月：    12 根柱 + 累计曲线叠加                             │
├────────────────────────────────────────────────────────────┤
│ 明细表（最近 14 行，可展开全部）：日期/消耗/日终水平/峰值/重置  │
└────────────────────────────────────────────────────────────┘
```

**状态设计**
- `accumulating`：每周/每月数据不足一个完整周期 → 顶部横幅"自 2026-10-04 起积累，当前 N 天"；
- 套餐未上报某窗口 → 窗口 chip 禁用 + 说明文案（"Pro Lite 当前未上报五小时窗口"）；
- 桥接离线空档 → 柱缺失（留白），tooltip 显示"无样本"；
- 零设备用户 → 整页显示"连接电脑后开始积累消耗数据"。

**移动端（三期）**：同一 API，渲染为简化柱列表。

## 七、实现切分

| 阶段 | 内容 | 估时 |
|---|---|---|
| P1 | migration 024 + 接收侧 rollup 写入 + 回填任务 + `consumption` API（window/daily）+ Web 消耗分析 tab（当前窗口/每日） | 1.5 天 |
| P2 | weekly/monthly 聚合查询 + Web 每周/每月视图 + 移动端 | 1 天 |
| P3（可选） | 小时×日热力图、CSV 导出、消耗异常检测接入提醒中心 | 0.5–1 天 |

## 八、风险与边界

| 风险 | 处理 |
|---|---|
| 7 天外历史无法回填 | 周/月视图显式"积累中"，不做假数据 |
| Codex 侧 used_percent 修正（下降） | clamp 丢弃，宁可少计 |
| 桥接器时钟/秒级重复样本 | 既有 `applied` 判定 + 同代窗口内才计 delta |
| 时区变更（用户改偏好） | 历史桶按 UTC 存，查询时换算，改动不破坏数据 |
| 报表体积 | rollup 单账户单窗口 ≤ 400×24 行/年，查询限 12 个月 |

## 九、验收要点

1. 重置日：柱状图出现 ▲ 标记、当日 consumed 不含跨代跳变；
2. 断电 1 天：次日柱缺失、coverage_hours=0、无插值；
3. prolite：五小时 chip 禁用并说明；plus/pro：五小时视图有数据时自动出现；
4. 回填后"每日"视图首日即有 ≤7 天数据；
5. `consumption` API 无会话返回 401；他人数据不可见（按 user_id 隔离）。
