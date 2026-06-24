package openaicompat

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
)

func ResponsesRequestToChatCompletionsRequest(req *dto.OpenAIResponsesRequest) (*dto.GeneralOpenAIRequest, error) {
	if req == nil {
		return nil, errors.New("request is nil")
	}
	if strings.TrimSpace(req.Model) == "" {
		return nil, errors.New("model is required")
	}
	if req.PreviousResponseID != "" {
		return nil, errors.New("previous_response_id is not supported by Chat Completions compatibility mode")
	}
	if hasJSONValue(req.Conversation) {
		return nil, errors.New("conversation is not supported by Chat Completions compatibility mode")
	}
	if hasJSONValue(req.ContextManagement) {
		return nil, errors.New("context_management is not supported by Chat Completions compatibility mode")
	}
	if hasJSONValue(req.Include) {
		return nil, errors.New("include is not supported by Chat Completions compatibility mode")
	}
	if hasJSONValue(req.Prompt) {
		return nil, errors.New("prompt templates are not supported by Chat Completions compatibility mode")
	}
	if req.MaxToolCalls != nil {
		return nil, errors.New("max_tool_calls is not supported by Chat Completions compatibility mode")
	}

	out := &dto.GeneralOpenAIRequest{
		Model:                req.Model,
		Stream:               req.Stream,
		MaxCompletionTokens:  req.MaxOutputTokens,
		Temperature:          req.Temperature,
		TopP:                 req.TopP,
		TopLogProbs:          req.TopLogProbs,
		Store:                cloneRaw(req.Store),
		Metadata:             cloneRaw(req.Metadata),
		User:                 cloneRaw(req.User),
		PromptCacheRetention: cloneRaw(req.PromptCacheRetention),
		SafetyIdentifier:     cloneRaw(req.SafetyIdentifier),
	}
	if req.ServiceTier != "" {
		out.ServiceTier = cloneAnyRaw(req.ServiceTier)
	}
	if req.PromptCacheKey != nil {
		var key string
		if err := common.Unmarshal(req.PromptCacheKey, &key); err != nil {
			return nil, fmt.Errorf("prompt_cache_key must be a string: %w", err)
		}
		out.PromptCacheKey = key
	}
	if req.TopLogProbs != nil {
		out.LogProbs = common.GetPointer(true)
	}
	if req.Reasoning != nil {
		if req.Reasoning.Summary != "" {
			return nil, errors.New("reasoning.summary is not supported by Chat Completions compatibility mode")
		}
		out.ReasoningEffort = req.Reasoning.Effort
	}
	out.EnableThinking = cloneRaw(req.EnableThinking)
	if hasJSONValue(req.Preset) {
		return nil, errors.New("preset is not supported by Chat Completions compatibility mode")
	}
	if hasJSONValue(req.ParallelToolCalls) {
		var value bool
		if err := common.Unmarshal(req.ParallelToolCalls, &value); err != nil {
			return nil, fmt.Errorf("parallel_tool_calls must be a boolean: %w", err)
		}
		out.ParallelTooCalls = &value
	}
	if req.StreamOptions != nil && req.StreamOptions.IncludeObfuscation {
		return nil, errors.New("stream_options.include_obfuscation is not supported by Chat Completions compatibility mode")
	}

	if hasJSONValue(req.Instructions) {
		var instructions string
		if err := common.Unmarshal(req.Instructions, &instructions); err != nil {
			return nil, fmt.Errorf("instructions must be a string: %w", err)
		}
		out.Messages = append(out.Messages, dto.Message{Role: "developer", Content: instructions})
	}
	messages, err := responsesInputToChatMessages(req.Input)
	if err != nil {
		return nil, err
	}
	out.Messages = append(out.Messages, messages...)

	tools, err := responsesToolsToChatTools(req.Tools)
	if err != nil {
		return nil, err
	}
	out.Tools = tools
	toolChoice, err := responsesToolChoiceToChat(req.ToolChoice)
	if err != nil {
		return nil, err
	}
	out.ToolChoice = toolChoice
	responseFormat, err := responsesTextToChatFormat(req.Text)
	if err != nil {
		return nil, err
	}
	out.ResponseFormat = responseFormat

	if hasJSONValue(req.Truncation) {
		var truncation string
		if err := common.Unmarshal(req.Truncation, &truncation); err != nil || (truncation != "" && truncation != "disabled") {
			return nil, errors.New("only truncation=disabled is supported by Chat Completions compatibility mode")
		}
	}
	return out, nil
}

func responsesInputToChatMessages(input []byte) ([]dto.Message, error) {
	if !hasJSONValue(input) {
		return nil, errors.New("input is required")
	}
	var decoded any
	if err := common.Unmarshal(input, &decoded); err != nil {
		return nil, fmt.Errorf("invalid input: %w", err)
	}
	if text, ok := decoded.(string); ok {
		return []dto.Message{{Role: "user", Content: text}}, nil
	}
	items, ok := decoded.([]any)
	if !ok {
		return nil, errors.New("input must be a string or an array of input items")
	}
	messages := make([]dto.Message, 0, len(items))
	for index, itemValue := range items {
		item, ok := itemValue.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("input[%d] must be an object", index)
		}
		typeName := common.Interface2String(item["type"])
		switch typeName {
		case "function_call":
			callID := common.Interface2String(item["call_id"])
			name := common.Interface2String(item["name"])
			if callID == "" || name == "" {
				return nil, fmt.Errorf("input[%d] function_call requires call_id and name", index)
			}
			call := dto.ToolCallRequest{ID: callID, Type: "function", Function: dto.FunctionRequest{Name: name, Arguments: common.Interface2String(item["arguments"])}}
			if len(messages) > 0 && messages[len(messages)-1].Role == "assistant" && messages[len(messages)-1].Content == nil {
				calls := messages[len(messages)-1].ParseToolCalls()
				calls = append(calls, call)
				messages[len(messages)-1].SetToolCalls(calls)
			} else {
				message := dto.Message{Role: "assistant", Content: nil}
				message.SetToolCalls([]dto.ToolCallRequest{call})
				messages = append(messages, message)
			}
		case "function_call_output":
			callID := common.Interface2String(item["call_id"])
			if callID == "" {
				return nil, fmt.Errorf("input[%d] function_call_output requires call_id", index)
			}
			output, ok := item["output"].(string)
			if !ok {
				b, marshalErr := common.Marshal(item["output"])
				if marshalErr != nil {
					return nil, fmt.Errorf("input[%d] invalid function output: %w", index, marshalErr)
				}
				output = string(b)
			}
			messages = append(messages, dto.Message{Role: "tool", ToolCallId: callID, Content: output})
		case "", "message":
			message, convertErr := responsesMessageItemToChat(item, index)
			if convertErr != nil {
				return nil, convertErr
			}
			messages = append(messages, message)
		default:
			return nil, fmt.Errorf("input[%d] type %q is not supported by Chat Completions compatibility mode", index, typeName)
		}
	}
	return messages, nil
}

func responsesMessageItemToChat(item map[string]any, index int) (dto.Message, error) {
	role := common.Interface2String(item["role"])
	if role == "" {
		return dto.Message{}, fmt.Errorf("input[%d] message requires role", index)
	}
	if text, ok := item["content"].(string); ok {
		return dto.Message{Role: role, Content: text}, nil
	}
	parts, ok := item["content"].([]any)
	if !ok {
		return dto.Message{}, fmt.Errorf("input[%d] message content must be a string or array", index)
	}
	content := make([]dto.MediaContent, 0, len(parts))
	for partIndex, partValue := range parts {
		part, ok := partValue.(map[string]any)
		if !ok {
			return dto.Message{}, fmt.Errorf("input[%d].content[%d] must be an object", index, partIndex)
		}
		switch partType := common.Interface2String(part["type"]); partType {
		case "input_text", "output_text", "text":
			content = append(content, dto.MediaContent{Type: dto.ContentTypeText, Text: common.Interface2String(part["text"])})
		case "input_image", "image_url":
			url := common.Interface2String(part["image_url"])
			if url == "" {
				return dto.Message{}, fmt.Errorf("input[%d].content[%d] input_image requires image_url", index, partIndex)
			}
			content = append(content, dto.MediaContent{Type: dto.ContentTypeImageURL, ImageUrl: &dto.MessageImageUrl{Url: url, Detail: common.Interface2String(part["detail"])}})
		default:
			return dto.Message{}, fmt.Errorf("input[%d].content[%d] type %q is not supported by Chat Completions compatibility mode", index, partIndex, partType)
		}
	}
	message := dto.Message{Role: role}
	message.SetMediaContent(content)
	return message, nil
}

func responsesToolsToChatTools(rawTools []byte) ([]dto.ToolCallRequest, error) {
	if !hasJSONValue(rawTools) {
		return nil, nil
	}
	var tools []map[string]any
	if err := common.Unmarshal(rawTools, &tools); err != nil {
		return nil, fmt.Errorf("tools must be an array: %w", err)
	}
	out := make([]dto.ToolCallRequest, 0, len(tools))
	for i, tool := range tools {
		if common.Interface2String(tool["type"]) != "function" {
			return nil, fmt.Errorf("tools[%d] type %q is not supported by Chat Completions compatibility mode", i, common.Interface2String(tool["type"]))
		}
		name := common.Interface2String(tool["name"])
		if name == "" {
			return nil, fmt.Errorf("tools[%d] function requires name", i)
		}
		function := dto.FunctionRequest{Name: name, Description: common.Interface2String(tool["description"]), Parameters: tool["parameters"]}
		if strict, exists := tool["strict"]; exists {
			function.Strict = cloneAnyRaw(strict)
		}
		out = append(out, dto.ToolCallRequest{Type: "function", Function: function})
	}
	return out, nil
}

func responsesToolChoiceToChat(rawChoice []byte) (any, error) {
	if !hasJSONValue(rawChoice) {
		return nil, nil
	}
	var choice any
	if err := common.Unmarshal(rawChoice, &choice); err != nil {
		return nil, fmt.Errorf("invalid tool_choice: %w", err)
	}
	if value, ok := choice.(string); ok {
		switch value {
		case "auto", "none", "required":
			return value, nil
		default:
			return nil, fmt.Errorf("tool_choice %q is not supported", value)
		}
	}
	object, ok := choice.(map[string]any)
	if !ok || common.Interface2String(object["type"]) != "function" || common.Interface2String(object["name"]) == "" {
		return nil, errors.New("named tool_choice must be a function with a name")
	}
	return map[string]any{"type": "function", "function": map[string]any{"name": common.Interface2String(object["name"])}}, nil
}

func responsesTextToChatFormat(rawText []byte) (*dto.ResponseFormat, error) {
	if !hasJSONValue(rawText) {
		return nil, nil
	}
	var text map[string]any
	if err := common.Unmarshal(rawText, &text); err != nil {
		return nil, fmt.Errorf("text must be an object: %w", err)
	}
	format, ok := text["format"].(map[string]any)
	if !ok {
		return nil, errors.New("text.format must be an object")
	}
	formatType := common.Interface2String(format["type"])
	switch formatType {
	case "text", "json_object":
		return &dto.ResponseFormat{Type: formatType}, nil
	case "json_schema":
		schema := make(map[string]any, len(format)-1)
		for key, value := range format {
			if key != "type" {
				schema[key] = value
			}
		}
		return &dto.ResponseFormat{Type: formatType, JsonSchema: cloneAnyRaw(schema)}, nil
	default:
		return nil, fmt.Errorf("text.format type %q is not supported", formatType)
	}
}

func ChatCompletionsResponseToResponsesResponse(resp *dto.OpenAITextResponse, id string) (*dto.OpenAIResponsesResponse, *dto.Usage, error) {
	if resp == nil {
		return nil, nil, errors.New("response is nil")
	}
	if len(resp.Choices) != 1 {
		return nil, nil, fmt.Errorf("expected exactly one Chat Completions choice, got %d", len(resp.Choices))
	}
	choice := resp.Choices[0]
	if id == "" {
		id = resp.Id
	}
	usage := resp.Usage
	usage.InputTokens = usage.PromptTokens
	usage.OutputTokens = usage.CompletionTokens
	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.InputTokens + usage.OutputTokens
	}
	usage.InputTokensDetails = &usage.PromptTokensDetails
	status := "completed"
	var incomplete *dto.IncompleteDetails
	switch choice.FinishReason {
	case "", "stop", "tool_calls", "function_call":
	case "length":
		status = "incomplete"
		incomplete = &dto.IncompleteDetails{Reasoning: "max_output_tokens"}
	default:
		status = "incomplete"
		incomplete = &dto.IncompleteDetails{Reasoning: choice.FinishReason}
	}
	createdAt := 0
	switch created := resp.Created.(type) {
	case int:
		createdAt = created
	case int64:
		createdAt = int(created)
	case float64:
		createdAt = int(created)
	}
	out := &dto.OpenAIResponsesResponse{
		ID: id, Object: "response", CreatedAt: createdAt, Model: resp.Model,
		Status: cloneAnyRaw(status), IncompleteDetails: incomplete, Usage: &usage,
	}
	content := choice.Message.StringContent()
	if content != "" {
		out.Output = append(out.Output, dto.ResponsesOutput{
			Type: "message", ID: "msg_" + id, Status: "completed", Role: "assistant",
			Content: []dto.ResponsesOutputContent{{Type: "output_text", Text: content, Annotations: []interface{}{}}},
		})
	}
	for i, call := range choice.Message.ParseToolCalls() {
		out.Output = append(out.Output, dto.ResponsesOutput{
			Type: "function_call", ID: fmt.Sprintf("fc_%s_%d", id, i), Status: "completed",
			CallId: call.ID, Name: call.Function.Name, Arguments: call.Function.Arguments,
		})
	}
	return out, &usage, nil
}

func hasJSONValue(raw []byte) bool {
	value := strings.TrimSpace(string(raw))
	return value != "" && value != "null"
}

func cloneRaw(raw []byte) []byte {
	if raw == nil {
		return nil
	}
	return append([]byte(nil), raw...)
}

func cloneAnyRaw(value any) []byte {
	b, _ := common.Marshal(value)
	return b
}
