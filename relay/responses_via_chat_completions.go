package relay

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

func responsesViaChatCompletions(c *gin.Context, info *relaycommon.RelayInfo, adaptor channel.Adaptor, request *dto.OpenAIResponsesRequest) (*dto.Usage, *types.NewAPIError) {
	chatRequest, err := service.ResponsesRequestToChatCompletionsRequest(request)
	if err != nil {
		return nil, types.NewErrorWithStatusCode(err, types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	applySystemPromptIfNeeded(c, info, chatRequest)
	if info.IsStream && info.SupportStreamOptions {
		chatRequest.StreamOptions = &dto.StreamOptions{IncludeUsage: true}
	}
	info.ShouldIncludeUsage = true

	savedRelayMode := info.RelayMode
	savedRequestURLPath := info.RequestURLPath
	defer func() {
		info.RelayMode = savedRelayMode
		info.RequestURLPath = savedRequestURLPath
	}()
	info.RelayMode = relayconstant.RelayModeChatCompletions
	info.RequestURLPath = "/v1/chat/completions"

	convertedRequest, err := adaptor.ConvertOpenAIRequest(c, info, chatRequest)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}
	relaycommon.AppendRequestConversionFromRequest(info, convertedRequest)
	jsonData, err := common.Marshal(convertedRequest)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}
	jsonData, err = relaycommon.RemoveDisabledFields(jsonData, info.ChannelOtherSettings, info.ChannelSetting.PassThroughBodyEnabled)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}
	if len(info.ParamOverride) > 0 {
		jsonData, err = relaycommon.ApplyParamOverrideWithRelayInfo(jsonData, info)
		if err != nil {
			return nil, newAPIErrorFromParamOverride(err)
		}
	}

	response, err := adaptor.DoRequest(c, info, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
	}
	if response == nil {
		return nil, types.NewOpenAIError(errorsNewInvalidUpstreamResponse(), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	httpResponse, ok := response.(*http.Response)
	if !ok || httpResponse == nil {
		return nil, types.NewOpenAIError(errorsNewInvalidUpstreamResponse(), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	info.IsStream = info.IsStream || strings.HasPrefix(httpResponse.Header.Get("Content-Type"), "text/event-stream")
	if httpResponse.StatusCode != http.StatusOK {
		newAPIError := service.RelayErrorHandler(c.Request.Context(), httpResponse, false)
		service.ResetStatusCode(newAPIError, c.GetString("status_code_mapping"))
		return nil, newAPIError
	}

	var usage *dto.Usage
	var responseErr *types.NewAPIError
	if info.IsStream {
		usage, responseErr = chatStreamToResponsesHandler(c, info, httpResponse)
	} else {
		usage, responseErr = chatToResponsesHandler(c, info, httpResponse)
	}
	if responseErr != nil {
		service.ResetStatusCode(responseErr, c.GetString("status_code_mapping"))
		return nil, responseErr
	}
	return usage, nil
}

func errorsNewInvalidUpstreamResponse() error {
	return fmt.Errorf("invalid upstream response")
}

func chatToResponsesHandler(c *gin.Context, info *relaycommon.RelayInfo, response *http.Response) (*dto.Usage, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(response)
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}
	var chatResponse dto.OpenAITextResponse
	if err := common.Unmarshal(body, &chatResponse); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if openAIError := chatResponse.GetOpenAIError(); openAIError != nil && openAIError.Type != "" {
		return nil, types.WithOpenAIError(*openAIError, response.StatusCode)
	}
	responseID := responsesCompatibilityID(info, chatResponse.Id)
	responsesResponse, usage, err := service.ChatCompletionsResponseToResponsesResponse(&chatResponse, responseID)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if usage.PromptTokens == 0 {
		usage.PromptTokens = info.GetEstimatePromptTokens()
		usage.InputTokens = usage.PromptTokens
	}
	if usage.CompletionTokens == 0 {
		usage.CompletionTokens = service.CountTextToken(responsesOutputText(responsesResponse), info.UpstreamModelName)
		usage.OutputTokens = usage.CompletionTokens
	}
	usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	responsesResponse.Usage = usage
	c.JSON(http.StatusOK, responsesResponse)
	return usage, nil
}

func chatStreamToResponsesHandler(c *gin.Context, info *relaycommon.RelayInfo, response *http.Response) (*dto.Usage, *types.NewAPIError) {
	converter := service.NewChatToResponsesStreamConverter(responsesCompatibilityID(info, ""))
	var conversionErr error
	helper.StreamScannerHandler(c, response, info, func(data string, result *helper.StreamResult) {
		var chunk dto.ChatCompletionsStreamResponse
		if err := common.UnmarshalJsonStr(data, &chunk); err != nil {
			conversionErr = err
			result.Error(err)
			return
		}
		events, err := converter.ProcessChunk(&chunk)
		if err != nil {
			conversionErr = err
			result.Error(err)
			return
		}
		for _, event := range events {
			if err := helper.ObjectData(c, event); err != nil {
				conversionErr = err
				result.Error(err)
				return
			}
		}
	})
	if conversionErr != nil {
		logger.LogError(c, "failed to convert Chat Completions stream to Responses: "+conversionErr.Error())
		_ = helper.ObjectData(c, map[string]any{"type": "error", "error": map[string]any{"type": "server_error", "message": conversionErr.Error()}})
		return nil, types.NewOpenAIError(conversionErr, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	events, usage, err := converter.Finalize()
	if err != nil {
		logger.LogError(c, "failed to finalize Chat Completions stream as Responses: "+err.Error())
		_ = helper.ObjectData(c, map[string]any{"type": "error", "error": map[string]any{"type": "server_error", "message": err.Error()}})
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	var terminalResponse *dto.OpenAIResponsesResponse
	for _, event := range events {
		if responseValue, ok := event["response"].(*dto.OpenAIResponsesResponse); ok {
			terminalResponse = responseValue
		}
	}
	if usage.PromptTokens == 0 {
		usage.PromptTokens = info.GetEstimatePromptTokens()
		usage.InputTokens = usage.PromptTokens
	}
	if usage.CompletionTokens == 0 && terminalResponse != nil {
		usage.CompletionTokens = service.CountTextToken(responsesOutputText(terminalResponse), info.UpstreamModelName)
		usage.OutputTokens = usage.CompletionTokens
	}
	usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	if terminalResponse != nil {
		terminalResponse.Usage = usage
	}
	for _, event := range events {
		if err := helper.ObjectData(c, event); err != nil {
			return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		}
	}
	return usage, nil
}

func responsesCompatibilityID(info *relaycommon.RelayInfo, upstreamID string) string {
	if strings.HasPrefix(upstreamID, "resp_") {
		return upstreamID
	}
	if info != nil && info.RequestId != "" {
		return "resp_" + info.RequestId
	}
	return "resp_" + common.GetRandomString(24)
}

func responsesOutputText(response *dto.OpenAIResponsesResponse) string {
	if response == nil {
		return ""
	}
	var text strings.Builder
	for _, output := range response.Output {
		for _, content := range output.Content {
			if content.Type == "output_text" {
				text.WriteString(content.Text)
			}
		}
	}
	return text.String()
}
