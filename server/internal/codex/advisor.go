package codex

import (
	"fmt"
	"math"
	"time"
)

type Preferences struct {
	Enabled       bool    `json:"enabled"`
	NearHours     float64 `json:"near_hours"`
	SparePercent  float64 `json:"spare_percent"`
	LowPercent    float64 `json:"low_percent"`
	PaceLead      float64 `json:"pace_lead"`
	CreditHours   float64 `json:"credit_hours"`
	CooldownHours int     `json:"cooldown_hours"` // Legacy API compatibility; events no longer repeat on a timer.
	QuietStart    int     `json:"quiet_start"`
	QuietEnd      int     `json:"quiet_end"`
	UTCOffset     int     `json:"utc_offset_minutes"`
}

func Defaults() Preferences { return Preferences{true, 24, 35, 15, 20, 24, 6, 23, 8, 480} }
func (p Preferences) Valid() bool {
	if p.SparePercent <= p.LowPercent {
		return false
	}
	return p.NearHours >= 1 && p.NearHours <= 168 && p.SparePercent >= 10 && p.SparePercent <= 95 && p.LowPercent >= 1 && p.LowPercent <= 30 && p.PaceLead >= 5 && p.PaceLead <= 70 && p.CreditHours >= 1 && p.CreditHours <= 168 && p.CooldownHours >= 1 && p.CooldownHours <= 48 && p.QuietStart >= 0 && p.QuietStart <= 23 && p.QuietEnd >= 0 && p.QuietEnd <= 23 && p.UTCOffset >= -720 && p.UTCOffset <= 840
}
func (p Preferences) Quiet(now time.Time) bool {
	h := now.UTC().Add(time.Duration(p.UTCOffset) * time.Minute).Hour()
	if p.QuietStart == p.QuietEnd {
		return false
	}
	if p.QuietStart < p.QuietEnd {
		return h >= p.QuietStart && h < p.QuietEnd
	}
	return h >= p.QuietStart || h < p.QuietEnd
}

type Advice struct {
	Key        string `json:"key"`
	Kind       string `json:"kind"`
	Title      string `json:"title"`
	Body       string `json:"body"`
	Priority   int    `json:"priority"`
	Reason     string `json:"reason"`
	Interrupt  bool   `json:"interrupt"`
	SourceName string `json:"source_name,omitempty"`
}
type Metric struct {
	Bucket       string   `json:"bucket"`
	Slot         string   `json:"slot"`
	Remaining    float64  `json:"remaining"`
	Hours        *float64 `json:"hours_left"`
	ExpectedUsed *float64 `json:"expected_used"`
	Rate         *float64 `json:"rate_per_hour"`
	Runway       *float64 `json:"runway_hours"`
	DailyBudget  *float64 `json:"daily_budget"`
	Reset        *int64   `json:"resets_at"`
	Duration     int64    `json:"duration_minutes"`
}
type Analysis struct {
	State   string   `json:"state"`
	Metrics []Metric `json:"metrics"`
	Advice  []Advice `json:"advice"`
}

func ptr(v float64) *float64 { return &v }
func matching(s Snapshot, id string, slot int) *Window {
	for _, b := range s.Buckets {
		if b.ID == id {
			if slot == 0 {
				return b.Primary
			}
			return b.Secondary
		}
	}
	return nil
}

// History is chronological. Never fit across a reset, a counter decrease, or unknown fields.
func recentRate(s Snapshot, h []Snapshot, id string, slot int, w *Window) *float64 {
	var oldest *Snapshot
	lastUsed := w.UsedPercent
	lastTime := s.ObservedAt
	samples := 1
	for i := len(h) - 1; i >= 0; i-- {
		v := &h[i]
		age := s.ObservedAt.Sub(v.ObservedAt)
		if age <= 0 {
			continue
		}
		if age > time.Hour {
			break
		}
		if lastTime.Sub(v.ObservedAt) <= 0 {
			continue
		}
		if lastTime.Sub(v.ObservedAt) > 11*time.Minute {
			break
		}
		old := matching(*v, id, slot)
		if v.Status != "ok" || old == nil || old.ResetsAt == nil || w.ResetsAt == nil || *old.ResetsAt != *w.ResetsAt || old.DurationMinutes != w.DurationMinutes || old.UsedPercent > lastUsed {
			break
		}
		oldest = v
		lastUsed = old.UsedPercent
		lastTime = v.ObservedAt
		samples++
	}
	if oldest == nil || samples < 3 || s.ObservedAt.Sub(oldest.ObservedAt) < 15*time.Minute {
		return nil
	}
	old := matching(*oldest, id, slot)
	return ptr((w.UsedPercent - old.UsedPercent) / s.ObservedAt.Sub(oldest.ObservedAt).Hours())
}

// A forecast must also have been risky at a previous collection, rather than
// turning a single burst or a resumed connection into an urgent message.
func sustainedRisk(s Snapshot, history []Snapshot, id string, slot int, w *Window, p Preferences) bool {
	for i := len(history) - 1; i >= 0; i-- {
		prev := history[i]
		gap := s.ObservedAt.Sub(prev.ObservedAt)
		if gap < 5*time.Minute {
			continue
		}
		if gap > 11*time.Minute || prev.Status != "ok" {
			return false
		}
		old := matching(prev, id, slot)
		if old == nil || old.ResetsAt == nil || w.ResetsAt == nil || *old.ResetsAt != *w.ResetsAt || old.DurationMinutes != w.DurationMinutes || old.UsedPercent > w.UsedPercent {
			return false
		}
		rate := recentRate(prev, history[:i], id, slot, old)
		hours := time.Unix(*old.ResetsAt, 0).Sub(prev.ObservedAt).Hours()
		if rate == nil || *rate <= 0 || hours <= 0 {
			return false
		}
		expected := math.Max(0, 100*(1-hours/(float64(old.DurationMinutes)/60)))
		return old.UsedPercent-expected >= p.PaceLead || (100-old.UsedPercent) / *rate < hours*0.7
	}
	return false
}

func remainingText(v float64) string {
	if v > 0 && v < 0.1 {
		return "不足 0.1%"
	}
	if v > 0 && v < 1 {
		return fmt.Sprintf("%.1f%%", v)
	}
	return fmt.Sprintf("%.0f%%", v)
}

func resetText(hours float64) string {
	if hours < 1 {
		return fmt.Sprintf("%d 分钟", int(math.Ceil(hours*60)))
	}
	if hours >= 24 {
		return fmt.Sprintf("%.1f 天", hours/24)
	}
	return fmt.Sprintf("%.1f 小时", hours)
}
func Analyze(s Snapshot, history []Snapshot, p Preferences, now time.Time) Analysis {
	out := Analysis{State: "balanced", Metrics: []Metric{}, Advice: []Advice{}}
	if s.Status != "ok" || now.Sub(s.ObservedAt) > 11*time.Minute || s.ObservedAt.After(now.Add(2*time.Minute)) {
		out.State = "stale"
		return out
	}
	boosts := []Advice{}
	slows := []Advice{}
	blocked := false
	windows := 0
	knownResets := 0
	for _, b := range s.Buckets {
		for slot, w := range []*Window{b.Primary, b.Secondary} {
			if w == nil {
				continue
			}
			windows++
			m := Metric{Bucket: b.ID, Slot: []string{"primary", "secondary"}[slot], Remaining: 100 - w.UsedPercent, Reset: w.ResetsAt, Duration: w.DurationMinutes}
			if m.Remaining <= p.LowPercent {
				blocked = true
			}
			if w.ResetsAt != nil {
				hours := time.Unix(*w.ResetsAt, 0).Sub(now).Hours()
				m.Hours = ptr(hours)
				if hours > 0 && hours <= float64(w.DurationMinutes)/60+0.05 {
					knownResets++
					expected := math.Max(0, 100*(1-hours/(float64(w.DurationMinutes)/60)))
					m.ExpectedUsed = &expected
					m.DailyBudget = ptr(math.Min(100, m.Remaining/hours*24))
					m.Rate = recentRate(s, history, b.ID, slot, w)
					if m.Rate != nil && *m.Rate > 0 {
						m.Runway = ptr(m.Remaining / *m.Rate)
					}
					near := math.Min(p.NearHours, float64(w.DurationMinutes)/60*0.2)
					key := fmt.Sprintf("%s:%d:%d", b.ID, slot, *w.ResetsAt)
					window := fmt.Sprintf("%s · %.0f 小时窗口", b.ID, float64(w.DurationMinutes)/60)
					if w.DurationMinutes%1440 == 0 {
						window = fmt.Sprintf("%s · %d 天窗口", b.ID, w.DurationMinutes/1440)
					}
					if m.Remaining <= p.LowPercent || (hours > near && (w.UsedPercent-expected >= p.PaceLead || (m.Runway != nil && *m.Runway < hours*0.7))) {
						title, reason, priority := "本周期额度使用偏多", "已用比例超出均匀时间进度，可预留后续任务额度；这不代表当前仍在快速消耗。", 2
						code, interrupt := "pace_ahead", false
						if m.Remaining <= 0 {
							title, reason, priority = "当前窗口额度已用完", "请等待重置，并在 Codex 中确认可用额度。", 4
							key += ":exhausted"
							code, interrupt = "quota_exhausted", true
						} else if m.Remaining <= p.LowPercent {
							title, reason, priority = "剩余额度偏低，优先处理重要任务", "预留后续任务所需额度。", 3
							key += ":low"
							code, interrupt = "quota_low", hours > 0.25
							if !interrupt {
								title, reason = "额度偏低，即将重置", "重置已临近，按需完成当前工作并等待新周期确认。"
							}
						} else if m.Runway != nil && *m.Runway < hours*0.7 {
							reason = fmt.Sprintf("按近期观测速度，预计约 %.1f 小时后用完，可能早于重置。建议降低消耗；预测仅作参考。", *m.Runway)
							code = "runway_short"
						}
						if code == "pace_ahead" || code == "runway_short" {
							interrupt = m.Rate != nil && *m.Rate > 0 && sustainedRisk(s, history, b.ID, slot, w, p)
							if interrupt {
								title = "持续消耗偏快，建议放慢"
								key += ":sustained"
							} else {
								reason += " 暂作为页面建议，等待连续观测确认。"
							}
						}
						body := fmt.Sprintf("%s 剩余 %s，约 %s 后重置。%s", window, remainingText(m.Remaining), resetText(hours), reason)
						slows = append(slows, Advice{Key: key + ":slow", Kind: "slow", Title: title, Body: body, Priority: priority, Reason: code, Interrupt: interrupt})
					} else if hours <= near && m.Remaining >= p.SparePercent {
						boosts = append(boosts, Advice{Key: key + ":use", Kind: "use", Title: "临近重置，可安排计划中的任务", Body: fmt.Sprintf("%s 剩余 %s，约 %s 后重置。其他已知窗口未见额度紧张，可按需提前安排任务。", window, remainingText(m.Remaining), resetText(hours)), Priority: 1, Reason: "reset_surplus"})
					}
				}
			}
			out.Metrics = append(out.Metrics, m)
		}
	}
	if len(slows) > 0 {
		out.State = "slow"
		out.Advice = append(out.Advice, slows...)
	} else if !blocked && knownResets == windows && len(boosts) > 0 {
		out.State = "use"
		out.Advice = append(out.Advice, boosts...)
	}
	if c := s.ResetCredits; c != nil && c.AvailableCount > 0 {
		for _, item := range c.Items {
			if item.ExpiresAt == nil {
				continue
			}
			hours := time.Unix(*item.ExpiresAt, 0).Sub(now).Hours()
			if hours > 0 && hours <= p.CreditHours {
				priority, key := 3, "credit:"+item.ID
				if hours <= 1 {
					priority, key = 4, key+":urgent"
				}
				if hours <= 1.0/6 {
					priority, key = 5, "credit:"+item.ID+":last-chance"
				}
				out.Advice = append(out.Advice, Advice{Key: key, Kind: "credit", Title: "重置卡快到期了", Body: fmt.Sprintf("一张可用重置卡约 %s 后到期。请在 Codex 检查兑换条件，并在到期前按需手动使用。", resetText(hours)), Priority: priority, Reason: "credit_expiry", Interrupt: true})
			}
		}
	}
	if n := s.News; n != nil && n.Status == "checked" && now.Sub(n.CheckedAt) < NewsFreshFor {
		for _, item := range n.Items {
			if now.Sub(item.PublishedAt) <= 72*time.Hour {
				out.Advice = append(out.Advice, Advice{Key: "news:" + item.URL, Kind: "news", Title: "Tibo 发布了重置相关消息", Body: item.Summary + " 这不代表你的账户已重置，请以本机额度为准。", Priority: 1, Reason: "external_news"})
			}
		}
	}
	if len(slows) == 0 && (windows == 0 || knownResets < windows) {
		out.State = "unknown"
	}
	return out
}
