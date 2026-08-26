package notification

import (
	"encoding/json"
	"fmt"
	"time"

	"aihub.dev/server/internal/quota"
)

const (
	TypeLowQuota         = "quota.low"
	TypeResetSoonUnused  = "quota.reset_soon_unused"
	TypeExpireSoonUnused = "quota.expire_soon_unused"
	TypeRenewalSoon      = "entitlement.renewal_soon"
	TypeStale            = "connector.stale"
)

type Rule struct {
	ID                string
	Name              string
	Type              string
	Enabled           bool
	Params            Params
	ProviderAccountID *string
}

type Params struct {
	Ratio *float64 `json:"ratio,omitempty"`
	Hours *float64 `json:"hours,omitempty"`
}

func ParseParams(raw string) (Params, error) {
	var p Params
	if raw == "" {
		return p, nil
	}
	err := json.Unmarshal([]byte(raw), &p)
	return p, err
}

type Subject struct {
	AccountID      string
	AccountName    string
	ProviderName   string
	BucketID       string
	ScopeKey       string
	EntitlementID  string
	PlanName       string
	LimitValue     *float64
	RemainingValue *float64
	RemainingRatio *float64
	ResetAt        *time.Time
	ExpiresAt      *time.Time
	RenewsAt       *time.Time
	ObservedAt     *time.Time
}

type Result struct {
	WouldFire          bool       `json:"would_fire"`
	Reason             string     `json:"reason"`
	EstimatedTriggerAt *time.Time `json:"estimated_trigger_at,omitempty"`
	DedupeKey          string     `json:"dedupe_key"`
	Severity           string     `json:"severity"`
	Title              string     `json:"title"`
	Body               string     `json:"body"`
	AccountID          string     `json:"provider_account_id"`
	BucketID           string     `json:"quota_bucket_id,omitempty"`
}

func Evaluate(rule Rule, sub Subject, now time.Time) Result {
	res := Result{
		AccountID: sub.AccountID,
		BucketID:  sub.BucketID,
		Severity:  "info",
	}
	ratio := quota.RemainingRatio(sub.LimitValue, sub.RemainingValue, sub.RemainingRatio)

	switch rule.Type {
	case TypeLowQuota:
		threshold := 0.2
		if rule.Params.Ratio != nil {
			threshold = *rule.Params.Ratio
		}
		res.DedupeKey = fmt.Sprintf("%s:%s", TypeLowQuota, sub.BucketID)
		res.Title = fmt.Sprintf("%s 额度偏低", sub.ProviderName)
		res.Severity = "warning"
		if ratio != nil && *ratio < threshold {
			res.WouldFire = true
			res.Reason = fmt.Sprintf("剩余比例 %.0f%% 低于阈值 %.0f%%", *ratio*100, threshold*100)
			res.Body = fmt.Sprintf("%s / %s 当前剩余 %.0f%%，已低于 %.0f%%。", sub.ProviderName, sub.AccountName, *ratio*100, threshold*100)
		} else if ratio != nil {
			res.Reason = fmt.Sprintf("剩余比例 %.0f%%，未低于阈值 %.0f%%", *ratio*100, threshold*100)
		} else {
			res.Reason = "没有可计算的剩余比例，无法判断"
		}

	case TypeResetSoonUnused:
		hours := 12.0
		if rule.Params.Hours != nil {
			hours = *rule.Params.Hours
		}
		unused := 0.3
		if rule.Params.Ratio != nil {
			unused = *rule.Params.Ratio
		}
		window := time.Duration(hours * float64(time.Hour))
		res.Severity = "warning"
		if sub.ResetAt != nil {
			est := sub.ResetAt.Add(-window)
			res.EstimatedTriggerAt = &est
			res.DedupeKey = fmt.Sprintf("%s:%s:%s", TypeResetSoonUnused, sub.BucketID, sub.ResetAt.UTC().Format(time.RFC3339))
		} else {
			res.DedupeKey = fmt.Sprintf("%s:%s", TypeResetSoonUnused, sub.BucketID)
		}
		res.Title = fmt.Sprintf("%s 即将重置且额度未用完", sub.ProviderName)
		if sub.ResetAt == nil {
			res.Reason = "没有重置时间"
			break
		}
		until := sub.ResetAt.Sub(now)
		ratioOK := ratio != nil && *ratio >= unused
		if until >= 0 && until <= window && ratioOK {
			res.WouldFire = true
			res.Reason = fmt.Sprintf("距离重置 %s，剩余 %.0f%% ≥ %.0f%%", fmtDuration(until), *ratio*100, unused*100)
			res.Body = fmt.Sprintf("%s / %s 将在 %s 重置，当前仍剩 %.0f%%。", sub.ProviderName, sub.AccountName, sub.ResetAt.Local().Format("01-02 15:04"), *ratio*100)
		} else if !ratioOK && ratio != nil {
			res.Reason = fmt.Sprintf("剩余 %.0f%%，未达到“未用完”阈值 %.0f%%", *ratio*100, unused*100)
		} else if until < 0 {
			res.Reason = "重置时间已过"
		} else {
			res.Reason = fmt.Sprintf("距离重置仍有 %s", fmtDuration(until))
		}

	case TypeExpireSoonUnused:
		hours := 24.0
		if rule.Params.Hours != nil {
			hours = *rule.Params.Hours
		}
		window := time.Duration(hours * float64(time.Hour))
		res.Severity = "warning"
		if sub.ExpiresAt != nil {
			est := sub.ExpiresAt.Add(-window)
			res.EstimatedTriggerAt = &est
			res.DedupeKey = fmt.Sprintf("%s:%s:%s", TypeExpireSoonUnused, sub.BucketID, sub.ExpiresAt.UTC().Format(time.RFC3339))
		} else {
			res.DedupeKey = fmt.Sprintf("%s:%s", TypeExpireSoonUnused, sub.BucketID)
		}
		res.Title = fmt.Sprintf("%s 即将过期但仍有剩余", sub.ProviderName)
		if sub.ExpiresAt == nil {
			res.Reason = "没有过期时间"
			break
		}
		until := sub.ExpiresAt.Sub(now)
		hasRemain := (sub.RemainingValue != nil && *sub.RemainingValue > 0) || (ratio != nil && *ratio > 0)
		if until >= 0 && until <= window && hasRemain {
			res.WouldFire = true
			remainText := "仍有剩余"
			if ratio != nil {
				remainText = fmt.Sprintf("剩余 %.0f%%", *ratio*100)
			}
			res.Reason = fmt.Sprintf("距离过期 %s，%s", fmtDuration(until), remainText)
			res.Body = fmt.Sprintf("%s / %s 将在 %s 过期，%s。", sub.ProviderName, sub.AccountName, sub.ExpiresAt.Local().Format("01-02 15:04"), remainText)
		} else if !hasRemain {
			res.Reason = "当前没有剩余额度"
		} else if until < 0 {
			res.Reason = "已经过期"
		} else {
			res.Reason = fmt.Sprintf("距离过期仍有 %s", fmtDuration(until))
		}

	case TypeRenewalSoon:
		hours := 72.0
		if rule.Params.Hours != nil {
			hours = *rule.Params.Hours
		}
		window := time.Duration(hours * float64(time.Hour))
		res.Severity = "info"
		if sub.RenewsAt != nil {
			est := sub.RenewsAt.Add(-window)
			res.EstimatedTriggerAt = &est
			res.DedupeKey = fmt.Sprintf("%s:%s:%s", TypeRenewalSoon, sub.EntitlementID, sub.RenewsAt.UTC().Format(time.RFC3339))
		} else {
			res.DedupeKey = fmt.Sprintf("%s:%s", TypeRenewalSoon, sub.EntitlementID)
		}
		res.Title = fmt.Sprintf("%s 即将续费", sub.ProviderName)
		if sub.RenewsAt == nil {
			res.Reason = "没有续费时间"
			break
		}
		until := sub.RenewsAt.Sub(now)
		if until >= 0 && until <= window {
			res.WouldFire = true
			res.Reason = fmt.Sprintf("距离续费 %s", fmtDuration(until))
			res.Body = fmt.Sprintf("%s / %s（%s）将在 %s 续费。", sub.ProviderName, sub.AccountName, sub.PlanName, sub.RenewsAt.Local().Format("01-02 15:04"))
		} else if until < 0 {
			res.Reason = "续费时间已过"
		} else {
			res.Reason = fmt.Sprintf("距离续费仍有 %s", fmtDuration(until))
		}

	case TypeStale:
		hours := 168.0
		if rule.Params.Hours != nil {
			hours = *rule.Params.Hours
		}
		window := time.Duration(hours * float64(time.Hour))
		res.Severity = "info"
		res.DedupeKey = fmt.Sprintf("%s:%s", TypeStale, sub.BucketID)
		res.Title = fmt.Sprintf("%s 数据过久未刷新", sub.ProviderName)
		if sub.ObservedAt == nil {
			res.WouldFire = true
			res.Reason = "还没有任何用量快照"
			res.Body = fmt.Sprintf("%s / %s 尚无用量记录，请手工刷新。", sub.ProviderName, sub.AccountName)
			break
		}
		age := now.Sub(*sub.ObservedAt)
		est := sub.ObservedAt.Add(window)
		res.EstimatedTriggerAt = &est
		if age >= window {
			res.WouldFire = true
			res.Reason = fmt.Sprintf("上次更新已过去 %s", fmtDuration(age))
			res.Body = fmt.Sprintf("%s / %s 的额度数据已 %s 未更新。", sub.ProviderName, sub.AccountName, fmtDuration(age))
		} else {
			res.Reason = fmt.Sprintf("上次更新于 %s 前", fmtDuration(age))
		}

	default:
		res.Reason = "未知规则类型"
	}
	return res
}

func fmtDuration(d time.Duration) string {
	if d < 0 {
		d = -d
	}
	if d < time.Hour {
		return fmt.Sprintf("%d 分钟", int(d.Minutes()))
	}
	if d < 48*time.Hour {
		return fmt.Sprintf("%.1f 小时", d.Hours())
	}
	return fmt.Sprintf("%.1f 天", d.Hours()/24)
}

func DefaultRules() []struct {
	Name   string
	Type   string
	Params Params
} {
	r := func(v float64) *float64 { return &v }
	return []struct {
		Name   string
		Type   string
		Params Params
	}{
		{Name: "低额度", Type: TypeLowQuota, Params: Params{Ratio: r(0.2)}},
		{Name: "即将重置但剩余过多", Type: TypeResetSoonUnused, Params: Params{Hours: r(12), Ratio: r(0.3)}},
		{Name: "即将过期但仍有剩余", Type: TypeExpireSoonUnused, Params: Params{Hours: r(24)}},
		{Name: "即将续费", Type: TypeRenewalSoon, Params: Params{Hours: r(72)}},
		{Name: "数据过久未刷新", Type: TypeStale, Params: Params{Hours: r(168)}},
	}
}
