package middleware

import (
	"fmt"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

// CheckChannelModelQuotaForChannel 渠道选路完成后的入方向时段限额检查；
// 命中则写出 429 错误并返回 true（调用方应立即 return，不得继续请求上游）。
func CheckChannelModelQuotaForChannel(c *gin.Context, channelId int, rawSettings, model string) bool {
	group := common.GetContextKeyString(c, constant.ContextKeyAutoGroup)
	if group == "" {
		group = common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
	}
	if group == "" || group == "auto" {
		if g, ok := service.MatchGroupForChannel(service.UserGroupListFromCtx(c), model, channelId); ok {
			group = g
		}
	}

	info := service.CheckChannelModelQuota(channelId, rawSettings, group, model, time.Now())
	if info == nil {
		return false
	}

	message := fmt.Sprintf(
		"分组 [%s] 下模型 [%s] 当前时段的服务额度已用尽（每4小时限额 %d tokens），将于 %s (UTC) 自动重置，请稍后重试或更换模型/渠道",
		group, model, info.Limit,
		time.Unix(info.ResetAt, 0).UTC().Format("15:04"),
	)
	c.JSON(http.StatusTooManyRequests, gin.H{
		"error": gin.H{
			"message":       common.MessageWithRequestId(message, c.GetString(common.RequestIdKey)),
			"type":          "channel_model_quota_exceeded",
			"code":          "rate_limit_exceeded",
			"quota_limit":   info.Limit,
			"quota_used":    info.Used,
			"quota_reset_at": info.ResetAt,
			"group":         group,
			"model":         model,
		},
	})
	c.Abort()
	return true
}
