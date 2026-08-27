package controller

import (
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

// GetUserChannelModelQuotaStatus 用户侧「模型额度」数据源：
// 返回所在组范围内、配置了时段限额的 渠道×组×模型 行及当前块用量。
func GetUserChannelModelQuotaStatus(c *gin.Context) {
	groups := service.UserGroupListFromCtx(c)

	channels, err := model.GetChannelsQuotaStatusCandidates()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	views := make([]service.QuotaStatusChannel, 0, len(channels))
	for _, ch := range channels {
		settings := ""
		if ch.ModelQuotaSettings != nil {
			settings = *ch.ModelQuotaSettings
		}
		views = append(views, service.QuotaStatusChannel{
			Id:                 ch.Id,
			Name:               ch.Name,
			Group:              ch.Group,
			Models:             ch.Models,
			ModelQuotaSettings: settings,
		})
	}

	rows := service.BuildChannelQuotaStatusRows(views, groups, time.Now(), service.GetChannelQuotaStore(),
		func(ch service.QuotaStatusChannel, group, modelName string) bool {
			return model.IsChannelEnabledForGroupModel(group, modelName, ch.Id)
		})

	if rows == nil {
		rows = []service.ChannelModelQuotaStatusRow{}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    rows,
	})
}
