package openaicompat

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/require"
)

func raw(t *testing.T, value any) []byte {
	t.Helper()
	b, err := common.Marshal(value)
	require.NoError(t, err)
	return b
}

func TestResponsesRequestToChatCompletionsRequestPreservesPortableFields(t *testing.T) {
	stream := false
	zeroUint := uint(0)
	zeroFloat := 0.0
	zeroInt := 0
	req := &dto.OpenAIResponsesRequest{
		Model:             "gpt-test",
		Input:             raw(t, "hello"),
		Instructions:      raw(t, "be concise"),
		MaxOutputTokens:   &zeroUint,
		Temperature:       &zeroFloat,
		TopP:              &zeroFloat,
		TopLogProbs:       &zeroInt,
		Stream:            &stream,
		ParallelToolCalls: raw(t, false),
		Store:             raw(t, false),
		Metadata:          raw(t, map[string]string{"trace": "abc"}),
		User:              raw(t, "user-1"),
		Reasoning:         &dto.Reasoning{Effort: "low"},
	}

	got, err := ResponsesRequestToChatCompletionsRequest(req)
	require.NoError(t, err)
	require.Equal(t, "gpt-test", got.Model)
	require.Len(t, got.Messages, 2)
	require.Equal(t, "developer", got.Messages[0].Role)
	require.Equal(t, "be concise", got.Messages[0].StringContent())
	require.Equal(t, "user", got.Messages[1].Role)
	require.Equal(t, "hello", got.Messages[1].StringContent())
	require.NotNil(t, got.MaxCompletionTokens)
	require.Zero(t, *got.MaxCompletionTokens)
	require.NotNil(t, got.Temperature)
	require.Zero(t, *got.Temperature)
	require.NotNil(t, got.TopP)
	require.Zero(t, *got.TopP)
	require.NotNil(t, got.TopLogProbs)
	require.Zero(t, *got.TopLogProbs)
	require.NotNil(t, got.LogProbs)
	require.True(t, *got.LogProbs)
	require.NotNil(t, got.Stream)
	require.False(t, *got.Stream)
	require.NotNil(t, got.ParallelTooCalls)
	require.False(t, *got.ParallelTooCalls)
	require.Equal(t, "low", got.ReasoningEffort)
	require.JSONEq(t, "false", string(got.Store))
}

func TestResponsesRequestToChatCompletionsRequestMapsItemsToolsAndFormat(t *testing.T) {
	req := &dto.OpenAIResponsesRequest{
		Model: "gpt-test",
		Input: raw(t, []any{
			map[string]any{
				"type": "message", "role": "user",
				"content": []any{
					map[string]any{"type": "input_text", "text": "look"},
					map[string]any{"type": "input_image", "image_url": "https://example.com/a.png", "detail": "low"},
				},
			},
			map[string]any{"type": "function_call", "call_id": "call_1", "name": "weather", "arguments": "{\"city\":\"Paris\"}"},
			map[string]any{"type": "function_call_output", "call_id": "call_1", "output": "sunny"},
		}),
		Tools: raw(t, []any{
			map[string]any{
				"type": "function", "name": "weather", "description": "Get weather",
				"parameters": map[string]any{"type": "object"}, "strict": false,
			},
		}),
		ToolChoice: raw(t, map[string]any{"type": "function", "name": "weather"}),
		Text: raw(t, map[string]any{
			"format": map[string]any{
				"type": "json_schema", "name": "answer", "strict": true,
				"schema": map[string]any{"type": "object"},
			},
		}),
	}

	got, err := ResponsesRequestToChatCompletionsRequest(req)
	require.NoError(t, err)
	require.Len(t, got.Messages, 3)
	content := got.Messages[0].ParseContent()
	require.Len(t, content, 2)
	require.Equal(t, dto.ContentTypeText, content[0].Type)
	require.Equal(t, "look", content[0].Text)
	require.Equal(t, dto.ContentTypeImageURL, content[1].Type)
	require.Equal(t, "https://example.com/a.png", content[1].GetImageMedia().Url)
	require.Equal(t, "low", content[1].GetImageMedia().Detail)
	require.Equal(t, "assistant", got.Messages[1].Role)
	require.Equal(t, "call_1", got.Messages[1].ParseToolCalls()[0].ID)
	require.Equal(t, "tool", got.Messages[2].Role)
	require.Equal(t, "call_1", got.Messages[2].ToolCallId)
	require.Equal(t, "sunny", got.Messages[2].StringContent())
	require.Len(t, got.Tools, 1)
	require.Equal(t, "weather", got.Tools[0].Function.Name)
	require.Equal(t, false, got.Tools[0].Function.Parameters == nil)
	require.JSONEq(t, `false`, string(got.Tools[0].Function.Strict))
	require.NotNil(t, got.ResponseFormat)
	require.Equal(t, "json_schema", got.ResponseFormat.Type)
	require.JSONEq(t, `{"type":"function","function":{"name":"weather"}}`, mustJSON(t, got.ToolChoice))
}

func TestResponsesRequestToChatCompletionsRequestRejectsUnrepresentableFeatures(t *testing.T) {
	tests := []struct {
		name string
		req  *dto.OpenAIResponsesRequest
	}{
		{"previous response", &dto.OpenAIResponsesRequest{Model: "m", Input: raw(t, "x"), PreviousResponseID: "resp_1"}},
		{"conversation", &dto.OpenAIResponsesRequest{Model: "m", Input: raw(t, "x"), Conversation: raw(t, "conv_1")}},
		{"include", &dto.OpenAIResponsesRequest{Model: "m", Input: raw(t, "x"), Include: raw(t, []string{"reasoning.encrypted_content"})}},
		{"reasoning summary", &dto.OpenAIResponsesRequest{Model: "m", Input: raw(t, "x"), Reasoning: &dto.Reasoning{Summary: "auto"}}},
		{"built in tool", &dto.OpenAIResponsesRequest{Model: "m", Input: raw(t, "x"), Tools: raw(t, []any{map[string]any{"type": "web_search_preview"}})}},
		{"file content", &dto.OpenAIResponsesRequest{
			Model: "m",
			Input: raw(t, []any{
				map[string]any{
					"role": "user",
					"content": []any{
						map[string]any{"type": "input_file", "file_id": "file_1"},
					},
				},
			}),
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ResponsesRequestToChatCompletionsRequest(tt.req)
			require.Error(t, err)
		})
	}
}

func TestChatCompletionsResponseToResponsesResponseMapsTextToolsUsage(t *testing.T) {
	chat := &dto.OpenAITextResponse{
		Id:      "chatcmpl_1",
		Object:  "chat.completion",
		Created: 123,
		Model:   "gpt-test",
		Choices: []dto.OpenAITextResponseChoice{{
			Index:        0,
			Message:      dto.Message{Role: "assistant", Content: "hello"},
			FinishReason: "tool_calls",
		}},
		Usage: dto.Usage{PromptTokens: 10, CompletionTokens: 4, TotalTokens: 14},
	}
	chat.Choices[0].Message.SetToolCalls([]dto.ToolCallResponse{{
		ID: "call_1", Type: "function", Function: dto.FunctionResponse{Name: "weather", Arguments: `{"city":"Paris"}`},
	}})

	got, usage, err := ChatCompletionsResponseToResponsesResponse(chat, "resp_local")
	require.NoError(t, err)
	require.Equal(t, "resp_local", got.ID)
	require.Equal(t, "response", got.Object)
	require.Equal(t, 123, got.CreatedAt)
	require.JSONEq(t, `"completed"`, string(got.Status))
	require.Len(t, got.Output, 2)
	require.Equal(t, "message", got.Output[0].Type)
	require.Equal(t, "output_text", got.Output[0].Content[0].Type)
	require.Equal(t, "hello", got.Output[0].Content[0].Text)
	require.Equal(t, "function_call", got.Output[1].Type)
	require.Equal(t, "call_1", got.Output[1].CallId)
	require.Equal(t, "weather", got.Output[1].Name)
	require.Equal(t, 10, usage.InputTokens)
	require.Equal(t, 4, usage.OutputTokens)
	require.Equal(t, 14, usage.TotalTokens)
}

func TestChatCompletionsResponseToResponsesResponseMapsLengthToIncomplete(t *testing.T) {
	chat := &dto.OpenAITextResponse{
		Id: "chatcmpl_1", Model: "gpt-test",
		Choices: []dto.OpenAITextResponseChoice{{
			Index: 0, Message: dto.Message{Role: "assistant", Content: "partial"}, FinishReason: "length",
		}},
	}

	got, _, err := ChatCompletionsResponseToResponsesResponse(chat, "resp_local")
	require.NoError(t, err)
	require.JSONEq(t, `"incomplete"`, string(got.Status))
	require.NotNil(t, got.IncompleteDetails)
	require.Equal(t, "max_output_tokens", got.IncompleteDetails.Reasoning)
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	b, err := common.Marshal(value)
	require.NoError(t, err)
	return string(b)
}
