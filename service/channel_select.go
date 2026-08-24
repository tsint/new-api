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
	var channel *model.Channel
	selectGroup := ""
	startGroupIndex := 0
	crossGroupRetry := common.GetContextKeyBool(param.Ctx, constant.ContextKeyTokenCrossGroupRetry)
	if lastGroupIndex, exists := common.GetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex); exists {
		if idx, ok := lastGroupIndex.(int); ok {
			startGroupIndex = idx
		}
	}
	for i := startGroupIndex; i < len(candidateGroups); i++ {
		group := candidateGroups[i]
		priorityRetry := param.GetRetry()
		if i > startGroupIndex {
			priorityRetry = 0
		}
		logger.LogDebug(param.Ctx, "Selecting group in order: %s, priorityRetry: %d", group, priorityRetry)
		channel, _ = model.GetRandomSatisfiedChannel(group, param.ModelName, priorityRetry, param.FormatGroup)
		if channel == nil {
			common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i+1)
			common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupRetryIndex, 0)
			param.SetRetry(0)
			continue
		}
		common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroup, group)
		selectGroup = group
		if crossGroupRetry && priorityRetry >= common.RetryTimes {
			common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i+1)
			param.SetRetry(0)
			param.ResetRetryNextTry()
		} else {
			common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i)
		}
		break
	}
	return channel, selectGroup
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

func CacheGetRandomSatisfiedChannel(param *RetryParam) (*model.Channel, string, error) {
	var channel *model.Channel
	var err error
	selectGroup := param.TokenGroup

	if param.TokenGroup == "auto" {
		if len(setting.GetAutoGroups()) == 0 {
			return nil, selectGroup, errors.New("auto groups is not enabled")
		}
		autoGroups := GetUserAutoGroupByGroups(UserGroupListFromCtx(param.Ctx))
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
