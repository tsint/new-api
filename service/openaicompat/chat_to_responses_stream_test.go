package openaicompat

import (
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/require"
)

func eventTypes(events []map[string]any) []string {
	types := make([]string, 0, len(events))
	for _, event := range events {
		types = append(types, event["type"].(string))
	}
	return types
}

func TestChatToResponsesStreamConverterTextLifecycle(t *testing.T) {
	converter := NewChatToResponsesStreamConverter("resp_1")
	text1 := "hel"
	text2 := "lo"
	stop := "stop"

	events, err := converter.ProcessChunk(&dto.ChatCompletionsStreamResponse{
		Id: "chatcmpl_1", Created: 123, Model: "gpt-test",
		Choices: []dto.ChatCompletionsStreamResponseChoice{{
			Index: 0, Delta: dto.ChatCompletionsStreamResponseChoiceDelta{Role: "assistant", Content: &text1},
		}},
	})
	require.NoError(t, err)
	require.Equal(t, []string{
		"response.created", "response.in_progress", "response.output_item.added",
		"response.content_part.added", "response.output_text.delta",
	}, eventTypes(events))

	events, err = converter.ProcessChunk(&dto.ChatCompletionsStreamResponse{
		Id: "chatcmpl_1", Created: 123, Model: "gpt-test",
		Choices: []dto.ChatCompletionsStreamResponseChoice{{
			Index: 0, Delta: dto.ChatCompletionsStreamResponseChoiceDelta{Content: &text2}, FinishReason: &stop,
		}},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"response.output_text.delta"}, eventTypes(events))

	events, usage, err := converter.Finalize()
	require.NoError(t, err)
	require.Equal(t, []string{
		"response.output_text.done", "response.content_part.done",
		"response.output_item.done", "response.completed",
	}, eventTypes(events))
	require.Equal(t, "hello", events[0]["text"])
	require.NotNil(t, usage)
	terminal := events[len(events)-1]["response"].(*dto.OpenAIResponsesResponse)
	require.Len(t, terminal.Output, 1)
	require.Equal(t, "hello", terminal.Output[0].Content[0].Text)
	require.JSONEq(t, `"completed"`, string(terminal.Status))
}

func TestChatToResponsesStreamConverterFragmentedParallelToolsAndUsage(t *testing.T) {
	converter := NewChatToResponsesStreamConverter("resp_2")
	zero, one := 0, 1
	toolFinish := "tool_calls"

	events, err := converter.ProcessChunk(&dto.ChatCompletionsStreamResponse{
		Id: "chatcmpl_2", Created: 456, Model: "gpt-test",
		Choices: []dto.ChatCompletionsStreamResponseChoice{{
			Index: 0,
			Delta: dto.ChatCompletionsStreamResponseChoiceDelta{ToolCalls: []dto.ToolCallResponse{
				{Index: &zero, ID: "call_1", Type: "function", Function: dto.FunctionResponse{Name: "weather", Arguments: `{"city":"Par`}},
				{Index: &one, ID: "call_2", Type: "function", Function: dto.FunctionResponse{Name: "time", Arguments: `{"zone":"UT`}},
			}},
		}},
	})
	require.NoError(t, err)
	require.Equal(t, []string{
		"response.created", "response.in_progress",
		"response.output_item.added", "response.function_call_arguments.delta",
		"response.output_item.added", "response.function_call_arguments.delta",
	}, eventTypes(events))

	_, err = converter.ProcessChunk(&dto.ChatCompletionsStreamResponse{
		Choices: []dto.ChatCompletionsStreamResponseChoice{{
			Index: 0, FinishReason: &toolFinish,
			Delta: dto.ChatCompletionsStreamResponseChoiceDelta{ToolCalls: []dto.ToolCallResponse{
				{Index: &zero, Function: dto.FunctionResponse{Arguments: `is"}`}},
				{Index: &one, Function: dto.FunctionResponse{Arguments: `C"}`}},
			}},
		}},
	})
	require.NoError(t, err)

	_, err = converter.ProcessChunk(&dto.ChatCompletionsStreamResponse{
		Usage: &dto.Usage{PromptTokens: 12, CompletionTokens: 8, TotalTokens: 20},
	})
	require.NoError(t, err)

	events, usage, err := converter.Finalize()
	require.NoError(t, err)
	require.Equal(t, []string{
		"response.function_call_arguments.done", "response.output_item.done",
		"response.function_call_arguments.done", "response.output_item.done",
		"response.completed",
	}, eventTypes(events))
	require.Equal(t, 12, usage.InputTokens)
	require.Equal(t, 8, usage.OutputTokens)
	terminal := events[len(events)-1]["response"].(*dto.OpenAIResponsesResponse)
	require.Len(t, terminal.Output, 2)
	require.Equal(t, `{"city":"Paris"}`, terminal.Output[0].Arguments)
	require.Equal(t, `{"zone":"UTC"}`, terminal.Output[1].Arguments)
}

func TestChatToResponsesStreamConverterLengthIsIncomplete(t *testing.T) {
	converter := NewChatToResponsesStreamConverter("resp_3")
	partial := "partial"
	length := "length"
	_, err := converter.ProcessChunk(&dto.ChatCompletionsStreamResponse{
		Choices: []dto.ChatCompletionsStreamResponseChoice{{
			Delta: dto.ChatCompletionsStreamResponseChoiceDelta{Content: &partial}, FinishReason: &length,
		}},
	})
	require.NoError(t, err)
	events, _, err := converter.Finalize()
	require.NoError(t, err)
	require.Equal(t, "response.incomplete", events[len(events)-1]["type"])
	terminal := events[len(events)-1]["response"].(*dto.OpenAIResponsesResponse)
	require.Equal(t, "max_output_tokens", terminal.IncompleteDetails.Reasoning)
}
