package model

import (
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

var group2model2channels map[string]map[string][]int // enabled channel
var group2model2format2channels map[string]map[string]map[common.APIFormatGroup][]int
var channelsIDM map[int]*Channel // all channels include disabled
var channelSyncLock sync.RWMutex

func InitChannelCache() {
	if !common.MemoryCacheEnabled {
		return
	}
	newChannelId2channel := make(map[int]*Channel)
	var channels []*Channel
	DB.Find(&channels)
	for _, channel := range channels {
		newChannelId2channel[channel.Id] = channel
	}
	var abilities []*Ability
	DB.Find(&abilities)
	groups := make(map[string]bool)
	for _, ability := range abilities {
		groups[ability.Group] = true
	}
	newGroup2model2channels := make(map[string]map[string][]int)
	newGroup2model2format2channels := make(map[string]map[string]map[common.APIFormatGroup][]int)
	for group := range groups {
		newGroup2model2channels[group] = make(map[string][]int)
		newGroup2model2format2channels[group] = make(map[string]map[common.APIFormatGroup][]int)
	}
	for _, channel := range channels {
		if channel.Status != common.ChannelStatusEnabled {
			continue // skip disabled channels
		}
		groups := strings.Split(channel.Group, ",")
		for _, group := range groups {
			models := strings.Split(channel.Models, ",")
			for _, model := range models {
				if _, ok := newGroup2model2channels[group][model]; !ok {
					newGroup2model2channels[group][model] = make([]int, 0)
				}
				newGroup2model2channels[group][model] = append(newGroup2model2channels[group][model], channel.Id)
				// Add channel to all supported format groups
				formatGroups := common.ChannelType2FormatGroups(channel.Type)
				for _, fg := range formatGroups {
					if _, ok := newGroup2model2format2channels[group][model]; !ok {
						newGroup2model2format2channels[group][model] = make(map[common.APIFormatGroup][]int)
					}
					if _, ok := newGroup2model2format2channels[group][model][fg]; !ok {
						newGroup2model2format2channels[group][model][fg] = make([]int, 0)
					}
					newGroup2model2format2channels[group][model][fg] = append(
						newGroup2model2format2channels[group][model][fg],
						channel.Id,
					)
				}
			}
		}
	}

	// sort by priority
	for group, model2channels := range newGroup2model2channels {
		for model, channels := range model2channels {
			sort.Slice(channels, func(i, j int) bool {
				return newChannelId2channel[channels[i]].GetPriority() > newChannelId2channel[channels[j]].GetPriority()
			})
			newGroup2model2channels[group][model] = channels
		}
	}

	// sort format-grouped channels by priority
	for group, model2format := range newGroup2model2format2channels {
		for model, format2channels := range model2format {
			for formatGroup, channels := range format2channels {
				sort.Slice(channels, func(i, j int) bool {
					return newChannelId2channel[channels[i]].GetPriority() > newChannelId2channel[channels[j]].GetPriority()
				})
				newGroup2model2format2channels[group][model][formatGroup] = channels
			}
		}
	}

	channelSyncLock.Lock()
	group2model2channels = newGroup2model2channels
	group2model2format2channels = newGroup2model2format2channels
	//channelsIDM = newChannelId2channel
	for i, channel := range newChannelId2channel {
		if channel.ChannelInfo.IsMultiKey {
			channel.Keys = channel.GetKeys()
			if channel.ChannelInfo.MultiKeyMode == constant.MultiKeyModePolling {
				if oldChannel, ok := channelsIDM[i]; ok {
					// 存在旧的渠道，如果是多key且轮询，保留轮询索引信息
					if oldChannel.ChannelInfo.IsMultiKey && oldChannel.ChannelInfo.MultiKeyMode == constant.MultiKeyModePolling {
						channel.ChannelInfo.MultiKeyPollingIndex = oldChannel.ChannelInfo.MultiKeyPollingIndex
					}
				}
			}
		}
	}
	channelsIDM = newChannelId2channel
	channelSyncLock.Unlock()
	common.SysLog("channels synced from database")
}

func SyncChannelCache(frequency int) {
	for {
		time.Sleep(time.Duration(frequency) * time.Second)
		common.SysLog("syncing channels from database")
		InitChannelCache()
	}
}

func selectChannelFromIDs(channelIDs []int, retry int) (*Channel, error) {
	if len(channelIDs) == 0 {
		return nil, nil
	}
	if len(channelIDs) == 1 {
		if channel, ok := channelsIDM[channelIDs[0]]; ok {
			return channel, nil
		}
		return nil, fmt.Errorf("数据库一致性错误，渠道# %d 不存在，请联系管理员修复", channelIDs[0])
	}

	uniquePriorities := make(map[int]bool)
	for _, channelId := range channelIDs {
		if channel, ok := channelsIDM[channelId]; ok {
			uniquePriorities[int(channel.GetPriority())] = true
		} else {
			return nil, fmt.Errorf("数据库一致性错误，渠道# %d 不存在，请联系管理员修复", channelId)
		}
	}
	var sortedUniquePriorities []int
	for priority := range uniquePriorities {
		sortedUniquePriorities = append(sortedUniquePriorities, priority)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(sortedUniquePriorities)))

	if retry >= len(uniquePriorities) {
		retry = len(uniquePriorities) - 1
	}
	targetPriority := int64(sortedUniquePriorities[retry])

	var sumWeight = 0
	var targetChannels []*Channel
	for _, channelId := range channelIDs {
		if channel, ok := channelsIDM[channelId]; ok {
			if channel.GetPriority() == targetPriority {
				sumWeight += channel.GetWeight()
				targetChannels = append(targetChannels, channel)
			}
		} else {
			return nil, fmt.Errorf("数据库一致性错误，渠道# %d 不存在，请联系管理员修复", channelId)
		}
	}

	if len(targetChannels) == 0 {
		return nil, errors.New(fmt.Sprintf("no channel found, priority: %d", targetPriority))
	}

	smoothingFactor := 1
	smoothingAdjustment := 0

	if sumWeight == 0 {
		sumWeight = len(targetChannels) * 100
		smoothingAdjustment = 100
	} else if sumWeight/len(targetChannels) < 10 {
		smoothingFactor = 100
	}

	totalWeight := sumWeight * smoothingFactor
	randomWeight := rand.Intn(totalWeight)

	for _, channel := range targetChannels {
		randomWeight -= channel.GetWeight()*smoothingFactor + smoothingAdjustment
		if randomWeight < 0 {
			return channel, nil
		}
	}
	return nil, errors.New("channel not found")
}

func GetRandomSatisfiedChannel(group string, model string, retry int, formatGroup common.APIFormatGroup) (*Channel, error) {
	// if memory cache is disabled, get channel directly from database
	if !common.MemoryCacheEnabled {
		return GetChannel(group, model, retry, formatGroup)
	}

	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	// Try format-matching channels first
	if formatGroup != common.FormatGroupOther {
		formatChannels := group2model2format2channels[group][model][formatGroup]
		if len(formatChannels) == 0 {
			normalizedModel := ratio_setting.FormatMatchingModelName(model)
			formatChannels = group2model2format2channels[group][normalizedModel][formatGroup]
		}
		if len(formatChannels) > 0 {
			channel, err := selectChannelFromIDs(formatChannels, retry)
			if err != nil {
				return nil, err
			}
			if channel != nil {
				return channel, nil
			}
		}
		return nil, nil
	}

	// First, try to find channels with the exact model name.
	channels := group2model2channels[group][model]

	// If no channels found, try to find channels with the normalized model name.
	if len(channels) == 0 {
		normalizedModel := ratio_setting.FormatMatchingModelName(model)
		channels = group2model2channels[group][normalizedModel]
	}

	if len(channels) == 0 {
		return nil, nil
	}

	return selectChannelFromIDs(channels, retry)
}

func CacheGetChannel(id int) (*Channel, error) {
	if !common.MemoryCacheEnabled {
		return GetChannelById(id, true)
	}
	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	c, ok := channelsIDM[id]
	if !ok {
		return nil, fmt.Errorf("渠道# %d，已不存在", id)
	}
	return c, nil
}

func CacheGetChannelInfo(id int) (*ChannelInfo, error) {
	if !common.MemoryCacheEnabled {
		channel, err := GetChannelById(id, true)
		if err != nil {
			return nil, err
		}
		return &channel.ChannelInfo, nil
	}
	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	c, ok := channelsIDM[id]
	if !ok {
		return nil, fmt.Errorf("渠道# %d，已不存在", id)
	}
	return &c.ChannelInfo, nil
}

func CacheUpdateChannelStatus(id int, status int) {
	if !common.MemoryCacheEnabled {
		return
	}
	channelSyncLock.Lock()
	defer channelSyncLock.Unlock()
	if channel, ok := channelsIDM[id]; ok {
		channel.Status = status
	}
	if status != common.ChannelStatusEnabled {
		removeChannelFromCacheIndex(id, group2model2channels)
		for group, model2format := range group2model2format2channels {
			for model, format2channels := range model2format {
				removeChannelFromFormatCacheIndex(id, group, model, format2channels)
			}
		}
	}
}

func removeChannelFromCacheIndex(id int, index map[string]map[string][]int) {
	for group, model2channels := range index {
		for model, channels := range model2channels {
			index[group][model] = removeChannelID(channels, id)
		}
	}
}

func removeChannelFromFormatCacheIndex(id int, group string, model string, format2channels map[common.APIFormatGroup][]int) {
	for formatGroup, channels := range format2channels {
		group2model2format2channels[group][model][formatGroup] = removeChannelID(channels, id)
	}
}

func removeChannelID(channels []int, id int) []int {
	for i := 0; i < len(channels); i++ {
		if channels[i] != id {
			continue
		}
		channels = append(channels[:i], channels[i+1:]...)
		i--
	}
	return channels
}

func CacheUpdateChannel(channel *Channel) {
	if !common.MemoryCacheEnabled {
		return
	}
	channelSyncLock.Lock()
	defer channelSyncLock.Unlock()
	if channel == nil {
		return
	}

	println("CacheUpdateChannel:", channel.Id, channel.Name, channel.Status, channel.ChannelInfo.MultiKeyPollingIndex)

	println("before:", channelsIDM[channel.Id].ChannelInfo.MultiKeyPollingIndex)
	channelsIDM[channel.Id] = channel
	println("after :", channelsIDM[channel.Id].ChannelInfo.MultiKeyPollingIndex)
}
