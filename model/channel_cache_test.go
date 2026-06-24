package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
)

func TestGetRandomSatisfiedChannelWithFormatGroupDoesNotFallbackToWrongFormat(t *testing.T) {
	oldMemoryCacheEnabled := common.MemoryCacheEnabled
	oldGroup2Model2Channels := group2model2channels
	oldGroup2Model2Format2Channels := group2model2format2channels
	oldChannelsIDM := channelsIDM
	t.Cleanup(func() {
		common.MemoryCacheEnabled = oldMemoryCacheEnabled
		group2model2channels = oldGroup2Model2Channels
		group2model2format2channels = oldGroup2Model2Format2Channels
		channelsIDM = oldChannelsIDM
	})

	common.MemoryCacheEnabled = true
	channelsIDM = map[int]*Channel{
		1: {Id: 1, Type: constant.ChannelTypeAnthropic},
	}
	group2model2channels = map[string]map[string][]int{
		"default": {
			"claude-3-5-sonnet": {1},
		},
	}
	group2model2format2channels = map[string]map[string]map[common.APIFormatGroup][]int{
		"default": {
			"claude-3-5-sonnet": {
				common.FormatGroupClaude: {1},
			},
		},
	}

	channel, err := GetRandomSatisfiedChannel("default", "claude-3-5-sonnet", 0, common.FormatGroupOpenAI)
	if err != nil {
		t.Fatalf("GetRandomSatisfiedChannel returned unexpected error: %v", err)
	}
	if channel != nil {
		t.Fatalf("GetRandomSatisfiedChannel returned channel #%d with type %d, want nil", channel.Id, channel.Type)
	}
}

func TestCacheUpdateChannelStatusRemovesDisabledChannelFromFormatGroupCache(t *testing.T) {
	oldMemoryCacheEnabled := common.MemoryCacheEnabled
	oldGroup2Model2Channels := group2model2channels
	oldGroup2Model2Format2Channels := group2model2format2channels
	oldChannelsIDM := channelsIDM
	t.Cleanup(func() {
		common.MemoryCacheEnabled = oldMemoryCacheEnabled
		group2model2channels = oldGroup2Model2Channels
		group2model2format2channels = oldGroup2Model2Format2Channels
		channelsIDM = oldChannelsIDM
	})

	common.MemoryCacheEnabled = true
	channelsIDM = map[int]*Channel{
		1: {Id: 1, Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled},
	}
	group2model2channels = map[string]map[string][]int{
		"default": {
			"gpt-4o": {1},
		},
	}
	group2model2format2channels = map[string]map[string]map[common.APIFormatGroup][]int{
		"default": {
			"gpt-4o": {
				common.FormatGroupOpenAI: {1},
			},
		},
	}

	CacheUpdateChannelStatus(1, common.ChannelStatusManuallyDisabled)

	channel, err := GetRandomSatisfiedChannel("default", "gpt-4o", 0, common.FormatGroupOpenAI)
	if err != nil {
		t.Fatalf("GetRandomSatisfiedChannel returned unexpected error: %v", err)
	}
	if channel != nil {
		t.Fatalf("disabled channel #%d was selected from format-group cache", channel.Id)
	}
}
