package common

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
)

func TestRelayFormat2FormatGroup(t *testing.T) {
	tests := []struct {
		name     string
		format   string
		expected APIFormatGroup
	}{
		{"openai chat", RelayFormatOpenAI, FormatGroupOpenAI},
		{"openai audio", RelayFormatOpenAIAudio, FormatGroupOpenAI},
		{"openai image", RelayFormatOpenAIImage, FormatGroupOpenAI},
		{"openai responses", RelayFormatOpenAIResponses, FormatGroupOpenAI},
		{"openai responses compaction", RelayFormatOpenAIResponsesCompaction, FormatGroupOpenAI},
		{"openai realtime", RelayFormatOpenAIRealtime, FormatGroupOpenAI},
		{"rerank", RelayFormatRerank, FormatGroupOpenAI},
		{"embedding", RelayFormatEmbedding, FormatGroupOpenAI},
		{"claude", RelayFormatClaude, FormatGroupClaude},
		{"gemini", RelayFormatGemini, FormatGroupGemini},
		{"unknown", "unknown", FormatGroupOther},
		{"empty", "", FormatGroupOther},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RelayFormat2FormatGroup(tt.format)
			if result != tt.expected {
				t.Errorf("RelayFormat2FormatGroup(%q) = %v, want %v", tt.format, result, tt.expected)
			}
		})
	}
}

func TestAPIType2FormatGroup(t *testing.T) {
	tests := []struct {
		name     string
		apiType  int
		expected APIFormatGroup
	}{
		{"OpenAI", constant.APITypeOpenAI, FormatGroupOpenAI},
		{"Azure maps to OpenAI", constant.APITypeOpenAI, FormatGroupOpenAI},
		{"OpenRouter", constant.APITypeOpenRouter, FormatGroupOpenAI},
		{"AWS", constant.APITypeAws, FormatGroupOpenAI},
		{"DeepSeek", constant.APITypeDeepSeek, FormatGroupOpenAI},
		{"Moonshot", constant.APITypeMoonshot, FormatGroupOpenAI},
		{"Anthropic", constant.APITypeAnthropic, FormatGroupClaude},
		{"Gemini", constant.APITypeGemini, FormatGroupGemini},
		{"Zhipu", constant.APITypeZhipu, FormatGroupOther},
		{"Baidu", constant.APITypeBaidu, FormatGroupOther},
		{"Ali", constant.APITypeAli, FormatGroupOther},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := APIType2FormatGroup(tt.apiType)
			if result != tt.expected {
				t.Errorf("APIType2FormatGroup(%d) = %v, want %v", tt.apiType, result, tt.expected)
			}
		})
	}
}

func TestChannelType2FormatGroups(t *testing.T) {
	tests := []struct {
		name          string
		channelType   int
		expectedLen   int
		expectedFirst APIFormatGroup
	}{
		{"Moonshot", constant.ChannelTypeMoonshot, 2, FormatGroupOpenAI},
		{"Anthropic", constant.ChannelTypeAnthropic, 1, FormatGroupClaude},
		{"Gemini", constant.ChannelTypeGemini, 1, FormatGroupGemini},
		{"OpenAI", constant.ChannelTypeOpenAI, 1, FormatGroupOpenAI},
		{"Azure", constant.ChannelTypeAzure, 1, FormatGroupOpenAI},
		{"OpenRouter", constant.ChannelTypeOpenRouter, 1, FormatGroupOpenAI},
		{"AWS", constant.ChannelTypeAws, 1, FormatGroupOpenAI},
		{"DeepSeek", constant.ChannelTypeDeepSeek, 1, FormatGroupOpenAI},
		{"Zhipu", constant.ChannelTypeZhipu, 1, FormatGroupOther},
		{"Baidu", constant.ChannelTypeBaidu, 1, FormatGroupOther},
		{"Ali", constant.ChannelTypeAli, 1, FormatGroupOther},
		{"Xunfei", constant.ChannelTypeXunfei, 1, FormatGroupOther},
		{"Tencent", constant.ChannelTypeTencent, 1, FormatGroupOther},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ChannelType2FormatGroups(tt.channelType)
			if len(result) != tt.expectedLen {
				t.Errorf("ChannelType2FormatGroups(%d) len = %d, want %d", tt.channelType, len(result), tt.expectedLen)
			}
			if len(result) > 0 && result[0] != tt.expectedFirst {
				t.Errorf("ChannelType2FormatGroups(%d)[0] = %v, want %v", tt.channelType, result[0], tt.expectedFirst)
			}
		})
	}
}

func TestChannelType2FormatGroups_MoonshotMultiFormat(t *testing.T) {
	result := ChannelType2FormatGroups(constant.ChannelTypeMoonshot)
	if len(result) != 2 {
		t.Fatalf("Moonshot should support 2 format groups, got %d", len(result))
	}

	hasOpenAI := false
	hasClaude := false
	for _, g := range result {
		if g == FormatGroupOpenAI {
			hasOpenAI = true
		}
		if g == FormatGroupClaude {
			hasClaude = true
		}
	}

	if !hasOpenAI {
		t.Error("Moonshot should support FormatGroupOpenAI")
	}
	if !hasClaude {
		t.Error("Moonshot should support FormatGroupClaude")
	}
}

func TestFormatGroup2ChannelTypes(t *testing.T) {
	// Test OpenAI format group
	openAIChannels := FormatGroup2ChannelTypes(FormatGroupOpenAI)
	if len(openAIChannels) == 0 {
		t.Error("FormatGroup2ChannelTypes(FormatGroupOpenAI) should not be empty")
	}

	// Verify Moonshot is in OpenAI group
	hasMoonshot := false
	for _, ct := range openAIChannels {
		if ct == constant.ChannelTypeMoonshot {
			hasMoonshot = true
			break
		}
	}
	if !hasMoonshot {
		t.Error("Moonshot should be in FormatGroupOpenAI channel types")
	}

	// Verify Anthropic is in Claude group
	claudeChannels := FormatGroup2ChannelTypes(FormatGroupClaude)
	hasAnthropic := false
	for _, ct := range claudeChannels {
		if ct == constant.ChannelTypeAnthropic {
			hasAnthropic = true
			break
		}
	}
	if !hasAnthropic {
		t.Error("Anthropic should be in FormatGroupClaude channel types")
	}

	// Verify Moonshot is ALSO in Claude group (multi-format support)
	hasMoonshotInClaude := false
	for _, ct := range claudeChannels {
		if ct == constant.ChannelTypeMoonshot {
			hasMoonshotInClaude = true
			break
		}
	}
	if !hasMoonshotInClaude {
		t.Error("Moonshot should also be in FormatGroupClaude channel types")
	}

	// Test Other format group includes ambiguous types
	otherChannels := FormatGroup2ChannelTypes(FormatGroupOther)
	hasZhipu := false
	for _, ct := range otherChannels {
		if ct == constant.ChannelTypeZhipu {
			hasZhipu = true
			break
		}
	}
	if !hasZhipu {
		t.Error("Zhipu should be in FormatGroupOther channel types")
	}
}

func TestChannelTypeSupportsFormatGroup(t *testing.T) {
	if !ChannelTypeSupportsFormatGroup(constant.ChannelTypeMoonshot, FormatGroupOpenAI) {
		t.Fatal("Moonshot should support OpenAI format")
	}
	if !ChannelTypeSupportsFormatGroup(constant.ChannelTypeMoonshot, FormatGroupClaude) {
		t.Fatal("Moonshot should support Claude format")
	}
	if ChannelTypeSupportsFormatGroup(constant.ChannelTypeOpenAI, FormatGroupClaude) {
		t.Fatal("OpenAI-only channel should not support Claude format")
	}
	if !ChannelTypeSupportsFormatGroup(constant.ChannelTypeOpenAI, FormatGroupOther) {
		t.Fatal("FormatGroupOther should not restrict channel type")
	}
}
