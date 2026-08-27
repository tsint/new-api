package service

import (
	"time"

	"github.com/QuantumNous/new-api/common"
)

// ChannelModelQuotaBlockSeconds 4小时固定分块（24h 恰好切6块，UTC 对齐）
const ChannelModelQuotaBlockSeconds int64 = 14400

// ChannelModelQuotaConfig 形如 {"group": {"model": limit, "*": fallback}}
// limit > 0 生效；<=0 表示该键显式不限额
type ChannelModelQuotaConfig map[string]map[string]int64

func ParseChannelModelQuotaSettings(jsonStr string) (ChannelModelQuotaConfig, error) {
	cfg := ChannelModelQuotaConfig{}
	if jsonStr == "" {
		return cfg, nil
	}
	if err := common.Unmarshal([]byte(jsonStr), &cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// MatchChannelModelQuotaLimit 返回匹配的限额值：精确模型优先于 "*" 通配；
// 值 <=0 视为该键不限制；无任何匹配返回 0。
func MatchChannelModelQuotaLimit(cfg ChannelModelQuotaConfig, group, model string) int64 {
	models, ok := cfg[group]
	if !ok {
		return 0
	}
	if v, ok := models[model]; ok {
		if v > 0 {
			return v
		}
		return 0
	}
	if v, ok := models["*"]; ok && v > 0 {
		return v
	}
	return 0
}

// MatchChannelModelQuotaLimitFromRaw 直接对渠道存的原始 JSON 做匹配；
// 解析失败一律视为不限制（宁可漏限不可错杀）。
func MatchChannelModelQuotaLimitFromRaw(raw, group, model string) int64 {
	cfg, err := ParseChannelModelQuotaSettings(raw)
	if err != nil {
		return 0
	}
	return MatchChannelModelQuotaLimit(cfg, group, model)
}

// QuotaBlockIndex 当前时刻所属的固定分块序号（UTC 对齐）
func QuotaBlockIndex(now time.Time) int64 {
	return now.Unix() / ChannelModelQuotaBlockSeconds
}

// NextQuotaBlockStart 下一个分块开始的 unix 秒（即恢复时间）
func NextQuotaBlockStart(now time.Time) int64 {
	u := now.Unix()
	if u%ChannelModelQuotaBlockSeconds == 0 {
		return u + ChannelModelQuotaBlockSeconds
	}
	return (QuotaBlockIndex(now) + 1) * ChannelModelQuotaBlockSeconds
}
