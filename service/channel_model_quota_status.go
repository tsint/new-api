package service

import (
	"strings"
	"time"
)

// QuotaStatusChannel 渠道状态视图的最小字段集（由 model 层查询填充）
type QuotaStatusChannel struct {
	Id                 int
	Name               string
	Group              string
	Models             string
	ModelQuotaSettings string
}

// ChannelModelQuotaStatusRow 用户侧「模型额度」页单行
type ChannelModelQuotaStatusRow struct {
	ChannelId        int    `json:"channel_id"`
	ChannelName      string `json:"channel_name"`
	Group            string `json:"group"`
	Model            string `json:"model"`
	Limit4h          int64  `json:"limit_4h"`
	UsedCurrentBlock int64  `json:"used_current_block"`
	Remaining        int64  `json:"remaining"`
	ResetAt          int64  `json:"reset_at"`
	Status           string `json:"status"` // normal | exhausted
}

// ServesFunc 判断渠道能力表是否允许 (group, model) 经此渠道
type ServesFunc func(ch QuotaStatusChannel, group, model string) bool

func alwaysServesDefault(QuotaStatusChannel, string, string) bool { return true }

// BuildChannelQuotaStatusRows 聚合用户可见的限额行：
// 仅包含已配置正值限额、组属于用户、且渠道实际服务该 (组,模型) 的组合。
func BuildChannelQuotaStatusRows(channels []QuotaStatusChannel, userGroups []string, now time.Time, store ChannelQuotaStore, serves ServesFunc) []ChannelModelQuotaStatusRow {
	if serves == nil {
		serves = alwaysServesDefault
	}
	groupSet := make(map[string]bool, len(userGroups))
	for _, g := range userGroups {
		groupSet[g] = true
	}
	idx := QuotaBlockIndex(now)
	resetAt := NextQuotaBlockStart(now)

	var rows []ChannelModelQuotaStatusRow
	for _, ch := range channels {
		cfg, err := ParseChannelModelQuotaSettings(ch.ModelQuotaSettings)
		if err != nil || len(cfg) == 0 {
			continue
		}
		for group, models := range cfg {
			if !groupSet[group] {
				continue
			}
			for model, limit := range models {
				if limit <= 0 {
					continue
				}
				if !serves(ch, group, model) && !strings.EqualFold(model, "*") {
					// 通配行无法逐一验证能力，跳过校验保留展示；
					// 精确行必须通过能力校验
					continue
				}
				used := store.GetUsed(ch.Id, group, model, idx)
				status := "normal"
				remaining := limit - used
				if remaining < 0 {
					remaining = 0
				}
				if used >= limit {
					status = "exhausted"
				}
				rows = append(rows, ChannelModelQuotaStatusRow{
					ChannelId:        ch.Id,
					ChannelName:      ch.Name,
					Group:            group,
					Model:            model,
					Limit4h:          limit,
					UsedCurrentBlock: used,
					Remaining:        remaining,
					ResetAt:          resetAt,
					Status:           status,
				})
			}
		}
	}
	return rows
}
