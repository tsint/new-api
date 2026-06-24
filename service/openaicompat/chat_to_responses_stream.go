package openaicompat

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/dto"
)

type chatToResponsesTextState struct {
	itemID      string
	outputIndex int
	text        strings.Builder
}

type chatToResponsesToolState struct {
	chatIndex   int
	outputIndex int
	itemID      string
	callID      string
	name        string
	arguments   strings.Builder
}

type ChatToResponsesStreamConverter struct {
	responseID   string
	chatID       string
	model        string
	createdAt    int
	sequence     int
	started      bool
	finalized    bool
	finishReason string
	nextOutput   int
	text         *chatToResponsesTextState
	tools        map[int]*chatToResponsesToolState
	usage        *dto.Usage
}

func NewChatToResponsesStreamConverter(responseID string) *ChatToResponsesStreamConverter {
	return &ChatToResponsesStreamConverter{responseID: responseID, tools: make(map[int]*chatToResponsesToolState)}
}

func (c *ChatToResponsesStreamConverter) ProcessChunk(chunk *dto.ChatCompletionsStreamResponse) ([]map[string]any, error) {
	if chunk == nil {
		return nil, errors.New("chat completion stream chunk is nil")
	}
	if c.finalized {
		return nil, errors.New("chat completion stream is already finalized")
	}
	if chunk.Id != "" {
		c.chatID = chunk.Id
	}
	if chunk.Model != "" {
		c.model = chunk.Model
	}
	if chunk.Created != 0 {
		c.createdAt = int(chunk.Created)
	}
	if chunk.Usage != nil {
		usage := normalizeChatUsage(*chunk.Usage)
		c.usage = &usage
	}

	events := make([]map[string]any, 0)
	if !c.started {
		c.started = true
		events = append(events,
			c.event("response.created", map[string]any{"response": c.response("in_progress", nil)}),
			c.event("response.in_progress", map[string]any{"response": c.response("in_progress", nil)}),
		)
	}

	for _, choice := range chunk.Choices {
		if choice.Index != 0 {
			return nil, fmt.Errorf("Chat Completions stream choice index %d is not supported", choice.Index)
		}
		if choice.Delta.Content != nil && *choice.Delta.Content != "" {
			if c.text == nil {
				c.text = &chatToResponsesTextState{itemID: "msg_" + c.responseID, outputIndex: c.nextOutput}
				c.nextOutput++
				item := c.textOutput("in_progress")
				events = append(events,
					c.event("response.output_item.added", map[string]any{"output_index": c.text.outputIndex, "item": item}),
					c.event("response.content_part.added", map[string]any{
						"item_id": c.text.itemID, "output_index": c.text.outputIndex, "content_index": 0,
						"part": dto.ResponsesOutputContent{Type: "output_text", Text: "", Annotations: []interface{}{}},
					}),
				)
			}
			delta := *choice.Delta.Content
			c.text.text.WriteString(delta)
			events = append(events, c.event("response.output_text.delta", map[string]any{
				"item_id": c.text.itemID, "output_index": c.text.outputIndex, "content_index": 0, "delta": delta,
			}))
		}

		for fallbackIndex, call := range choice.Delta.ToolCalls {
			chatIndex := fallbackIndex
			if call.Index != nil {
				chatIndex = *call.Index
			}
			state, exists := c.tools[chatIndex]
			if !exists {
				state = &chatToResponsesToolState{
					chatIndex: chatIndex, outputIndex: c.nextOutput,
					itemID: fmt.Sprintf("fc_%s_%d", c.responseID, chatIndex),
					callID: call.ID, name: call.Function.Name,
				}
				c.nextOutput++
				c.tools[chatIndex] = state
				item := c.toolOutput(state, "in_progress")
				events = append(events, c.event("response.output_item.added", map[string]any{"output_index": state.outputIndex, "item": item}))
			} else {
				if call.ID != "" {
					state.callID += call.ID
				}
				if call.Function.Name != "" {
					state.name += call.Function.Name
				}
			}
			if call.Function.Arguments != "" {
				state.arguments.WriteString(call.Function.Arguments)
				events = append(events, c.event("response.function_call_arguments.delta", map[string]any{
					"item_id": state.itemID, "output_index": state.outputIndex, "delta": call.Function.Arguments,
				}))
			}
		}
		if choice.FinishReason != nil && *choice.FinishReason != "" {
			c.finishReason = *choice.FinishReason
		}
	}
	return events, nil
}

func (c *ChatToResponsesStreamConverter) Finalize() ([]map[string]any, *dto.Usage, error) {
	if c.finalized {
		return nil, c.usage, errors.New("chat completion stream is already finalized")
	}
	if !c.started {
		return nil, nil, errors.New("chat completion stream contained no chunks")
	}
	if c.finishReason == "" {
		return nil, c.usage, errors.New("chat completion stream ended without a finish reason")
	}
	c.finalized = true
	events := make([]map[string]any, 0)
	if c.text != nil {
		text := c.text.text.String()
		part := dto.ResponsesOutputContent{Type: "output_text", Text: text, Annotations: []interface{}{}}
		events = append(events,
			c.event("response.output_text.done", map[string]any{
				"item_id": c.text.itemID, "output_index": c.text.outputIndex, "content_index": 0, "text": text,
			}),
			c.event("response.content_part.done", map[string]any{
				"item_id": c.text.itemID, "output_index": c.text.outputIndex, "content_index": 0, "part": part,
			}),
			c.event("response.output_item.done", map[string]any{
				"output_index": c.text.outputIndex, "item": c.textOutput("completed"),
			}),
		)
	}
	tools := c.sortedTools()
	for _, tool := range tools {
		arguments := tool.arguments.String()
		events = append(events,
			c.event("response.function_call_arguments.done", map[string]any{
				"item_id": tool.itemID, "output_index": tool.outputIndex, "arguments": arguments,
			}),
			c.event("response.output_item.done", map[string]any{
				"output_index": tool.outputIndex, "item": c.toolOutput(tool, "completed"),
			}),
		)
	}
	status, incomplete := responsesStatusFromFinishReason(c.finishReason)
	terminalType := "response.completed"
	if status != "completed" {
		terminalType = "response.incomplete"
	}
	events = append(events, c.event(terminalType, map[string]any{"response": c.response(status, incomplete)}))
	if c.usage == nil {
		c.usage = &dto.Usage{}
	}
	return events, c.usage, nil
}

func (c *ChatToResponsesStreamConverter) event(eventType string, fields map[string]any) map[string]any {
	event := make(map[string]any, len(fields)+2)
	event["type"] = eventType
	event["sequence_number"] = c.sequence
	c.sequence++
	for key, value := range fields {
		event[key] = value
	}
	return event
}

func (c *ChatToResponsesStreamConverter) response(status string, incomplete *dto.IncompleteDetails) *dto.OpenAIResponsesResponse {
	response := &dto.OpenAIResponsesResponse{
		ID: c.responseID, Object: "response", CreatedAt: c.createdAt, Model: c.model,
		Status: cloneAnyRaw(status), IncompleteDetails: incomplete, Usage: c.usage,
	}
	response.Output = make([]dto.ResponsesOutput, c.nextOutput)
	if c.text != nil {
		response.Output[c.text.outputIndex] = c.textOutput("completed")
	}
	for _, tool := range c.tools {
		response.Output[tool.outputIndex] = c.toolOutput(tool, "completed")
	}
	return response
}

func (c *ChatToResponsesStreamConverter) textOutput(status string) dto.ResponsesOutput {
	text := ""
	if c.text != nil {
		text = c.text.text.String()
	}
	return dto.ResponsesOutput{
		Type: "message", ID: c.text.itemID, Status: status, Role: "assistant",
		Content: []dto.ResponsesOutputContent{{Type: "output_text", Text: text, Annotations: []interface{}{}}},
	}
}

func (c *ChatToResponsesStreamConverter) toolOutput(tool *chatToResponsesToolState, status string) dto.ResponsesOutput {
	return dto.ResponsesOutput{
		Type: "function_call", ID: tool.itemID, Status: status,
		CallId: tool.callID, Name: tool.name, Arguments: tool.arguments.String(),
	}
}

func (c *ChatToResponsesStreamConverter) sortedTools() []*chatToResponsesToolState {
	tools := make([]*chatToResponsesToolState, 0, len(c.tools))
	for _, tool := range c.tools {
		tools = append(tools, tool)
	}
	sort.Slice(tools, func(i, j int) bool { return tools[i].outputIndex < tools[j].outputIndex })
	return tools
}

func normalizeChatUsage(usage dto.Usage) dto.Usage {
	usage.InputTokens = usage.PromptTokens
	usage.OutputTokens = usage.CompletionTokens
	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.InputTokens + usage.OutputTokens
	}
	usage.InputTokensDetails = &usage.PromptTokensDetails
	return usage
}

func responsesStatusFromFinishReason(finishReason string) (string, *dto.IncompleteDetails) {
	switch finishReason {
	case "stop", "tool_calls", "function_call":
		return "completed", nil
	case "length":
		return "incomplete", &dto.IncompleteDetails{Reasoning: "max_output_tokens"}
	default:
		return "incomplete", &dto.IncompleteDetails{Reasoning: finishReason}
	}
}
