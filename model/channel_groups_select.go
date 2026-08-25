package model

import (
	"sort"

	"github.com/QuantumNous/new-api/common"
)

// GetRandomSatisfiedChannelByGroups 按组顺序批量选择渠道：
// 返回第一个存在可用渠道的组及其渠道。firstRetry 仅作用于第一个组
// （组内按优先级降级），后续组一律使用其最高优先级。
//
// 性能：内存缓存模式下全程只持有一次读锁；DB 模式下将逐组多套 SQL
// （MAX 子查询 + getPriority + 渠道回查）压缩为 1 条 abilities 批量查询
// + 1 条渠道回查，与组数无关。
//
// 与逐组调用 GetRandomSatisfiedChannel 的多组路径一致：单个组内部的
// 一致性错误不阻断后续组；所有组均无可用渠道时返回 (nil, "", nil)。
func GetRandomSatisfiedChannelByGroups(groups []string, model string, firstRetry int, formatGroup common.APIFormatGroup) (*Channel, string, error) {
	if len(groups) == 0 {
		return nil, "", nil
	}
	if !common.MemoryCacheEnabled {
		return getRandomSatisfiedChannelByGroupsDB(groups, model, firstRetry, formatGroup)
	}

	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	for i, group := range groups {
		retry := 0
		if i == 0 {
			retry = firstRetry
		}
		channel, err := getRandomSatisfiedChannelLocked(group, model, retry, formatGroup)
		if err != nil {
			continue
		}
		if channel != nil {
			return channel, group, nil
		}
	}
	return nil, "", nil
}

func getRandomSatisfiedChannelByGroupsDB(groups []string, model string, firstRetry int, formatGroup common.APIFormatGroup) (*Channel, string, error) {
	query := DB.Table("abilities").Select("abilities.*").
		Where("abilities."+commonGroupCol+" IN ? and abilities.model = ? and abilities.enabled = ?", groups, model, true)
	if formatGroup != common.FormatGroupOther {
		query = query.Joins("JOIN channels ON channels.id = abilities.channel_id")
		if channelTypes := common.FormatGroup2ChannelTypes(formatGroup); len(channelTypes) > 0 {
			query = query.Where("channels.type IN ?", channelTypes)
		}
	}

	var abilities []Ability
	if err := query.Order("weight DESC").Find(&abilities).Error; err != nil {
		return nil, "", err
	}

	byGroup := make(map[string][]Ability, len(groups))
	for _, ab := range abilities {
		byGroup[ab.Group] = append(byGroup[ab.Group], ab)
	}

	for i, group := range groups {
		rows := byGroup[group]
		if len(rows) == 0 {
			continue
		}
		retry := 0
		if i == 0 {
			retry = firstRetry
		}
		targetPriority := pickAbilityPriority(rows, retry)
		// rows 已按 weight DESC 排列，过滤后仍保持与 GetChannel 一致的选取顺序
		bucket := make([]Ability, 0, len(rows))
		for _, ab := range rows {
			if abilityPriority(ab) == targetPriority {
				bucket = append(bucket, ab)
			}
		}
		channelID := weightedPickAbility(bucket)
		if channelID <= 0 {
			continue
		}
		channel := Channel{}
		if err := DB.First(&channel, "id = ?", channelID).Error; err != nil {
			continue
		}
		return &channel, group, nil
	}
	return nil, "", nil
}

func abilityPriority(ab Ability) int64 {
	if ab.Priority == nil {
		return 0
	}
	return *ab.Priority
}

// pickAbilityPriority 返回组内第 retry 档优先级（降序），超出层数时收敛到最低优先级，
// 与 getPriority / selectChannelFromIDs 的语义一致。
func pickAbilityPriority(rows []Ability, retry int) int64 {
	seen := make(map[int64]struct{}, len(rows))
	priorities := make([]int64, 0, len(rows))
	for _, ab := range rows {
		p := abilityPriority(ab)
		if _, ok := seen[p]; !ok {
			seen[p] = struct{}{}
			priorities = append(priorities, p)
		}
	}
	sort.Slice(priorities, func(i, j int) bool { return priorities[i] > priorities[j] })
	if retry < 0 {
		retry = 0
	}
	if retry >= len(priorities) {
		retry = len(priorities) - 1
	}
	return priorities[retry]
}

// weightedPickAbility 复刻 GetChannel 的权重随机选取：weight+10 平滑。
func weightedPickAbility(bucket []Ability) int {
	weightSum := 0
	for _, ab := range bucket {
		weightSum += int(ab.Weight) + 10
	}
	if weightSum <= 0 {
		return 0
	}
	weight := common.GetRandomInt(weightSum)
	for _, ab := range bucket {
		weight -= int(ab.Weight) + 10
		if weight <= 0 {
			return ab.ChannelId
		}
	}
	return 0
}
