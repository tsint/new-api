package common

import "github.com/QuantumNous/new-api/constant"

type APIFormatGroup int

const (
	FormatGroupOpenAI APIFormatGroup = iota
	FormatGroupClaude
	FormatGroupGemini
	FormatGroupOther
)

// Relay format string constants (mirroring types.RelayFormat to avoid import cycle).
const (
	RelayFormatOpenAI                    = "openai"
	RelayFormatClaude                    = "claude"
	RelayFormatGemini                    = "gemini"
	RelayFormatOpenAIResponses           = "openai_responses"
	RelayFormatOpenAIResponsesCompaction = "openai_responses_compaction"
	RelayFormatOpenAIAudio               = "openai_audio"
	RelayFormatOpenAIImage               = "openai_image"
	RelayFormatOpenAIRealtime            = "openai_realtime"
	RelayFormatRerank                    = "rerank"
	RelayFormatEmbedding                 = "embedding"
)

func RelayFormat2FormatGroup(format string) APIFormatGroup {
	switch format {
	case RelayFormatOpenAI,
		RelayFormatOpenAIAudio,
		RelayFormatOpenAIImage,
		RelayFormatOpenAIResponses,
		RelayFormatOpenAIResponsesCompaction,
		RelayFormatOpenAIRealtime,
		RelayFormatRerank,
		RelayFormatEmbedding:
		return FormatGroupOpenAI
	case RelayFormatClaude:
		return FormatGroupClaude
	case RelayFormatGemini:
		return FormatGroupGemini
	default:
		return FormatGroupOther
	}
}

func APIType2FormatGroup(apiType int) APIFormatGroup {
	switch apiType {
	case constant.APITypeOpenAI,
		constant.APITypeOpenRouter,
		constant.APITypeAws,
		constant.APITypeDeepSeek,
		constant.APITypeMoonshot,
		constant.APITypeXai,
		constant.APITypeSiliconFlow,
		constant.APITypeMistral,
		constant.APITypeCohere,
		constant.APITypePerplexity,
		constant.APITypeOllama,
		constant.APITypeXinference,
		constant.APITypeMiniMax,
		constant.APITypeReplicate,
		constant.APITypeCodex,
		constant.APITypeSubmodel:
		return FormatGroupOpenAI
	case constant.APITypeAnthropic:
		return FormatGroupClaude
	case constant.APITypeGemini:
		return FormatGroupGemini
	default:
		return FormatGroupOther
	}
}

// ChannelType2FormatGroups 基于 channel.Type 判断该 channel 支持哪些格式分组。
// 一个 channel 可以同时支持多个格式（如 Moonshot 同时支持 OpenAI 和 Claude）。
// 渠道管理中用户选择的"类型"直接决定格式分组，不需要检查 base URL。
func ChannelType2FormatGroups(channelType int) []APIFormatGroup {
	switch channelType {
	case constant.ChannelTypeMoonshot:
		// Moonshot 同时兼容 OpenAI 和 Claude 格式
		return []APIFormatGroup{FormatGroupOpenAI, FormatGroupClaude}
	case constant.ChannelTypeAnthropic:
		return []APIFormatGroup{FormatGroupClaude}
	case constant.ChannelTypeGemini:
		return []APIFormatGroup{FormatGroupGemini}
	case constant.ChannelTypeOpenAI,
		constant.ChannelTypeAzure,
		constant.ChannelTypeOpenRouter,
		constant.ChannelTypeAws,
		constant.ChannelTypeDeepSeek,
		constant.ChannelTypeXai,
		constant.ChannelTypeSiliconFlow,
		constant.ChannelTypeMistral,
		constant.ChannelTypeCohere,
		constant.ChannelTypePerplexity,
		constant.ChannelTypeOllama,
		constant.ChannelTypeXinference,
		constant.ChannelTypeMiniMax,
		constant.ChannelTypeReplicate,
		constant.ChannelTypeCodex,
		constant.ChannelTypeSubmodel:
		return []APIFormatGroup{FormatGroupOpenAI}
	default:
		// zhipu, baidu, ali, xunfei 等有歧义或不标准的类型
		return []APIFormatGroup{FormatGroupOther}
	}
}

// FormatGroup2ChannelTypes 返回支持指定格式分组的所有 channel type 值。
// 用于 DB 查询时的 IN 条件过滤。
func FormatGroup2ChannelTypes(fg APIFormatGroup) []int {
	var result []int
	for ct := 0; ct < constant.ChannelTypeDummy; ct++ {
		groups := ChannelType2FormatGroups(ct)
		for _, g := range groups {
			if g == fg {
				result = append(result, ct)
				break
			}
		}
	}
	return result
}

func ChannelTypeSupportsFormatGroup(channelType int, formatGroup APIFormatGroup) bool {
	if formatGroup == FormatGroupOther {
		return true
	}
	for _, group := range ChannelType2FormatGroups(channelType) {
		if group == formatGroup {
			return true
		}
	}
	return false
}
