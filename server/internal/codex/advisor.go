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
	CooldownHours int     `json:"cooldown_hours"`
	QuietStart    int     `json:"quiet_start"`
	QuietEnd      int     `json:"quiet_end"`
	UTCOffset     int     `json:"utc_offset_minutes"`
}

func Defaults() Preferences { return Preferences{true, 24, 35, 15, 20, 24, 6, 23, 8, 480} }
func (p Preferences) Valid() bool {
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
	Key      string `json:"key"`
	Kind     string `json:"kind"`
	Title    string `json:"title"`
	Body     string `json:"body"`
	Priority int    `json:"priority"`
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
	for i := len(h) - 1; i >= 0; i-- {
		v := &h[i]
		age := s.ObservedAt.Sub(v.ObservedAt)
		if age <= 0 {
			continue
		}
		if age > 6*time.Hour {
			break
		}
		old := matching(*v, id, slot)
		if v.Status != "ok" || old == nil || old.ResetsAt == nil || w.ResetsAt == nil || *old.ResetsAt != *w.ResetsAt || old.DurationMinutes != w.DurationMinutes || old.UsedPercent > lastUsed {
			break
		}
		oldest = v
		lastUsed = old.UsedPercent
	}
	if oldest == nil || s.ObservedAt.Sub(oldest.ObservedAt) < 15*time.Minute {
		return nil
	}
	old := matching(*oldest, id, slot)
	return ptr((w.UsedPercent - old.UsedPercent) / s.ObservedAt.Sub(oldest.ObservedAt).Hours())
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
	knownResets := 0
	for _, b := range s.Buckets {
		for slot, w := range []*Window{b.Primary, b.Secondary} {
			if w == nil {
				continue
			}
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
					if m.Remaining <= p.LowPercent || (hours > near && (w.UsedPercent-expected >= p.PaceLead || (m.Runway != nil && *m.Runway < hours*0.7))) {
						body := fmt.Sprintf("%s 剩余 %.0f%%，距重置 %.1f 小时。当前已用 %.0f%%，均匀使用参考为 %.0f%%。", b.ID, m.Remaining, hours, w.UsedPercent, expected)
						if m.Runway != nil {
							body += fmt.Sprintf("按近 6 小时内的观测速度，约 %.1f 小时后耗尽；这是估计，不是承诺。", *m.Runway)
						}
						slows = append(slows, Advice{key + ":slow", "slow", "放慢一点，给后续任务留额度", body, 2})
					} else if hours <= near && m.Remaining >= p.SparePercent {
						boosts = append(boosts, Advice{key + ":use", "use", "快重置了，可以多安排一些任务", fmt.Sprintf("%s 还有 %.0f%%，约 %.1f 小时后重置。可以优先处理已计划的代码审查、测试或整理任务。", b.ID, m.Remaining, hours), 1})
					}
				}
			}
			out.Metrics = append(out.Metrics, m)
		}
	}
	if len(slows) > 0 {
		out.State = "slow"
		out.Advice = append(out.Advice, slows...)
	} else if !blocked && len(boosts) > 0 {
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
				out.Advice = append(out.Advice, Advice{"credit:" + item.ID, "credit", "重置卡快到期了", fmt.Sprintf("一张可用重置卡约 %.1f 小时后到期。请在 Codex 检查兑换条件，安排任务并在到期前手动使用；不会自动消耗重置卡。", hours), 3})
			}
		}
	}
	if n := s.News; n != nil && n.Status == "checked" && now.Sub(n.CheckedAt) < NewsFreshFor {
		for _, item := range n.Items {
			if now.Sub(item.PublishedAt) <= 72*time.Hour {
				out.Advice = append(out.Advice, Advice{"news:" + item.URL, "news", "Tibo 发布了重置相关消息", item.Summary + " 这不代表你的账户已重置，请以本机额度为准。", 1})
			}
		}
	}
	if len(out.Metrics) == 0 || knownResets == 0 {
		out.State = "unknown"
	}
	return out
}
