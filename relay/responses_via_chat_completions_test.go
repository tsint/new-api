package relay

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestChatToResponsesHandlerReturnsResponsesShape(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	info := &relaycommon.RelayInfo{RequestId: "request-1", ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-test"}}
	info.SetEstimatePromptTokens(3)
	upstream := &http.Response{
		StatusCode: http.StatusOK,
		Body: io.NopCloser(strings.NewReader(`{
			"id":"chatcmpl_1","object":"chat.completion","created":123,"model":"gpt-test",
			"choices":[{"index":0,"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}
		}`)),
	}

	usage, relayErr := chatToResponsesHandler(ctx, info, upstream)
	require.Nil(t, relayErr)
	require.Equal(t, 3, usage.InputTokens)
	require.Equal(t, 2, usage.OutputTokens)
	require.Equal(t, http.StatusOK, recorder.Code)
	var response dto.OpenAIResponsesResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.Equal(t, "resp_request-1", response.ID)
	require.Equal(t, "response", response.Object)
	require.Len(t, response.Output, 1)
	require.Equal(t, "hello", response.Output[0].Content[0].Text)
}

func TestChatStreamToResponsesHandlerEmitsSemanticEventsWithoutDoneSentinel(t *testing.T) {
	originalTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = originalTimeout })
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	info := &relaycommon.RelayInfo{RequestId: "request-2", IsStream: true, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-test"}}
	info.SetEstimatePromptTokens(3)
	upstream := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(
			"data: {\"id\":\"chatcmpl_2\",\"created\":123,\"model\":\"gpt-test\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"hello\"}}]}\n\n" +
				"data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n" +
				"data: {\"choices\":[],\"usage\":{\"prompt_tokens\":3,\"completion_tokens\":2,\"total_tokens\":5}}\n\n" +
				"data: [DONE]\n\n",
		)),
	}

	usage, relayErr := chatStreamToResponsesHandler(ctx, info, upstream)
	require.Nil(t, relayErr)
	require.Equal(t, 5, usage.TotalTokens)
	body := recorder.Body.String()
	require.Contains(t, body, `"type":"response.created"`)
	require.Contains(t, body, `"type":"response.output_text.delta"`)
	require.Contains(t, body, `"type":"response.completed"`)
	require.NotContains(t, body, "[DONE]")
}
