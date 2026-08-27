package model

import (
	"github.com/QuantumNous/new-api/common"
)

// GetChannelsQuotaStatusCandidates 返回启用渠道中与额度状态页相关的最小字段集。
// 仅选择所需列，避免带出渠道密钥等敏感字段。
func GetChannelsQuotaStatusCandidates() ([]*Channel, error) {
	var channels []*Channel
	err := DB.Select("id", "name", commonGroupCol, "models", "model_quota_settings").
		Where("status = ?", common.ChannelStatusEnabled).
		Find(&channels).Error
	return channels, err
}
