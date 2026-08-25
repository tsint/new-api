package service

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
)

type RetryParam struct {
	Ctx          *gin.Context
	TokenGroup   string
	ModelName    string
	Retry        *int
	resetNextTry bool
	FormatGroup  common.APIFormatGroup // 新增：API 格式分组
}

func (p *RetryParam) GetRetry() int {
	if p.Retry == nil {
		return 0
	}
	return *p.Retry
}

func (p *RetryParam) SetRetry(retry int) {
	p.Retry = &retry
}

func (p *RetryParam) IncreaseRetry() {
	if p.resetNextTry {
		p.resetNextTry = false
		return
	}
	if p.Retry == nil {
		p.Retry = new(int)
	}
	*p.Retry++
}

func (p *RetryParam) ResetRetryNextTry() {
	p.resetNextTry = true
}

func UserGroupListFromCtx(c *gin.Context) []string {
	if v, ok := common.GetContextKey(c, constant.ContextKeyUserGroupList); ok {
		if list, ok2 := v.([]string); ok2 && len(list) > 0 {
			return list
		}
	}
	return []string{common.GetContextKeyString(c, constant.ContextKeyUserGroup)}
}

func tryGroupsInOrder(param *RetryParam, candidateGroups []string) (*model.Channel, string) {
	startGroupIndex := 0
	crossGroupRetry := common.GetContextKeyBool(param.Ctx, constant.ContextKeyTokenCrossGroupRetry)
	if lastGroupIndex, exists := common.GetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex); exists {
		if idx, ok := lastGroupIndex.(int); ok {
			startGroupIndex = idx
		}
	}
	if startGroupIndex >= len(candidateGroups) {
		return nil, ""
	}

	// 批量选择：一次调用按顺序扫描所有候选组，避免逐组独立的锁/DB 往返
	firstRetry := param.GetRetry()
	remaining := candidateGroups[startGroupIndex:]
	channel, hitGroup, _ := model.GetRandomSatisfiedChannelByGroups(remaining, param.ModelName, firstRetry, param.FormatGroup)
	if channel == nil {
		common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, len(candidateGroups))
		common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupRetryIndex, 0)
		param.SetRetry(0)
		return nil, ""
	}

	hitIndex := startGroupIndex
	for i, g := range remaining {
		if g == hitGroup {
			hitIndex = startGroupIndex + i
			break
		}
	}
	priorityRetry := 0
	if hitIndex == startGroupIndex {
		priorityRetry = firstRetry
	}
	logger.LogDebug(param.Ctx, "Selected group in order: %s (index %d), priorityRetry: %d", hitGroup, hitIndex, priorityRetry)
	common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroup, hitGroup)
	if crossGroupRetry && priorityRetry >= common.RetryTimes {
		// 当前分组重试已用完，下次重试切到下一个分组
		common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, hitIndex+1)
		param.SetRetry(0)
		param.ResetRetryNextTry()
	} else {
		common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, hitIndex)
	}
	return channel, hitGroup
}

// MatchGroupForChannel 返回 groupList 中第一个对 (modelName, channelID) 可用的组
func MatchGroupForChannel(groupList []string, modelName string, channelID int) (string, bool) {
	for _, g := range groupList {
		if model.IsChannelEnabledForGroupModel(g, modelName, channelID) {
			return g, true
		}
	}
	return "", false
}

// getAutoGroupsCached 每个请求上下文只计算一次 auto 候选组列表。
// usable groups 的计算涉及全量 map copy 与 special settings 合并，
// 重试场景下会被反复触发，缓存到 context 中避免重复计算。
func getAutoGroupsCached(c *gin.Context) []string {
	if v, ok := common.GetContextKey(c, constant.ContextKeyAutoGroupList); ok {
		if list, ok2 := v.([]string); ok2 {
			return list
		}
	}
	list := GetUserAutoGroupByGroups(UserGroupListFromCtx(c))
	common.SetContextKey(c, constant.ContextKeyAutoGroupList, list)
	return list
}

func CacheGetRandomSatisfiedChannel(param *RetryParam) (*model.Channel, string, error) {
	var channel *model.Channel
	var err error
	selectGroup := param.TokenGroup

	if param.TokenGroup == "auto" {
		if len(setting.GetAutoGroups()) == 0 {
			return nil, selectGroup, errors.New("auto groups is not enabled")
		}
		autoGroups := getAutoGroupsCached(param.Ctx)
		channel, hitGroup := tryGroupsInOrder(param, autoGroups)
		if channel == nil {
			return nil, selectGroup, nil
		}
		return channel, hitGroup, nil
	}

	if param.TokenGroup == "" {
		groupList := UserGroupListFromCtx(param.Ctx)
		if len(groupList) <= 1 {
			group := groupList[0]
			channel, err = model.GetRandomSatisfiedChannel(group, param.ModelName, param.GetRetry(), param.FormatGroup)
			if err != nil {
				return nil, group, err
			}
			if channel == nil {
				return nil, "", nil
			}
			return channel, group, nil
		}
		channel, hitGroup := tryGroupsInOrder(param, groupList)
		return channel, hitGroup, nil
	}

	channel, err = model.GetRandomSatisfiedChannel(param.TokenGroup, param.ModelName, param.GetRetry(), param.FormatGroup)
	if err != nil {
		return nil, param.TokenGroup, err
	}
	return channel, selectGroup, nil
}
