package openaicompat

import (
	"testing"

	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/stretchr/testify/require"
)

func TestShouldResponsesUseChatCompletionsPolicy(t *testing.T) {
	base := model_setting.ResponsesToChatCompletionsPolicy{
		Enabled: true, AllChannels: false, ChannelIDs: []int{7}, ChannelTypes: []int{2},
		ModelPatterns: []string{`^gpt-4o`, `deepseek`},
	}

	require.True(t, ShouldResponsesUseChatCompletionsPolicy(base, 7, 99, "gpt-4o-mini"))
	require.True(t, ShouldResponsesUseChatCompletionsPolicy(base, 99, 2, "deepseek-chat"))
	require.False(t, ShouldResponsesUseChatCompletionsPolicy(base, 99, 3, "gpt-4o"))
	require.False(t, ShouldResponsesUseChatCompletionsPolicy(base, 7, 3, "claude"))

	base.Enabled = false
	require.False(t, ShouldResponsesUseChatCompletionsPolicy(base, 7, 2, "gpt-4o"))
	base.Enabled = true
	base.AllChannels = true
	require.True(t, ShouldResponsesUseChatCompletionsPolicy(base, 999, 999, "gpt-4o"))
}
