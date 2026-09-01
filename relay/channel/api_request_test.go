package channel

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestProcessHeaderOverride_ChannelTestSkipsPassthroughRules(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("X-Trace-Id", "trace-123")

	info := &relaycommon.RelayInfo{
		IsChannelTest: true,
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"*": "",
			},
		},
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Empty(t, headers)
}

func TestProcessHeaderOverride_ChannelTestSkipsClientHeaderPlaceholder(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("X-Trace-Id", "trace-123")

	info := &relaycommon.RelayInfo{
		IsChannelTest: true,
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"X-Upstream-Trace": "{client_header:X-Trace-Id}",
			},
		},
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	_, ok := headers["x-upstream-trace"]
	require.False(t, ok)
}

func TestProcessHeaderOverride_NonTestKeepsClientHeaderPlaceholder(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("X-Trace-Id", "trace-123")

	info := &relaycommon.RelayInfo{
		IsChannelTest: false,
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"X-Upstream-Trace": "{client_header:X-Trace-Id}",
			},
		},
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Equal(t, "trace-123", headers["x-upstream-trace"])
}

func TestProcessHeaderOverride_RuntimeOverrideIsFinalHeaderMap(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	info := &relaycommon.RelayInfo{
		IsChannelTest:             false,
		UseRuntimeHeadersOverride: true,
		RuntimeHeadersOverride: map[string]any{
			"x-static":  "runtime-value",
			"x-runtime": "runtime-only",
		},
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"X-Static": "legacy-value",
				"X-Legacy": "legacy-only",
			},
		},
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Equal(t, "runtime-value", headers["x-static"])
	require.Equal(t, "runtime-only", headers["x-runtime"])
	_, exists := headers["x-legacy"]
	require.False(t, exists)
}

func TestProcessHeaderOverride_PassthroughSkipsAcceptEncoding(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("X-Trace-Id", "trace-123")
	ctx.Request.Header.Set("Accept-Encoding", "gzip")

	info := &relaycommon.RelayInfo{
		IsChannelTest: false,
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"*": "",
			},
		},
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Equal(t, "trace-123", headers["x-trace-id"])

	_, hasAcceptEncoding := headers["accept-encoding"]
	require.False(t, hasAcceptEncoding)
}

func TestProcessHeaderOverride_PassHeadersTemplateSetsRuntimeHeaders(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	ctx.Request.Header.Set("Originator", "Codex CLI")
	ctx.Request.Header.Set("Session_id", "sess-123")

	info := &relaycommon.RelayInfo{
		IsChannelTest: false,
		RequestHeaders: map[string]string{
			"Originator": "Codex CLI",
			"Session_id": "sess-123",
		},
		ChannelMeta: &relaycommon.ChannelMeta{
			ParamOverride: map[string]any{
				"operations": []any{
					map[string]any{
						"mode":  "pass_headers",
						"value": []any{"Originator", "Session_id", "X-Codex-Beta-Features"},
					},
				},
			},
			HeadersOverride: map[string]any{
				"X-Static": "legacy-value",
			},
		},
	}

	_, err := relaycommon.ApplyParamOverrideWithRelayInfo([]byte(`{"model":"gpt-4.1"}`), info)
	require.NoError(t, err)
	require.True(t, info.UseRuntimeHeadersOverride)
	require.Equal(t, "Codex CLI", info.RuntimeHeadersOverride["originator"])
	require.Equal(t, "sess-123", info.RuntimeHeadersOverride["session_id"])
	_, exists := info.RuntimeHeadersOverride["x-codex-beta-features"]
	require.False(t, exists)
	require.Equal(t, "legacy-value", info.RuntimeHeadersOverride["x-static"])

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Equal(t, "Codex CLI", headers["originator"])
	require.Equal(t, "sess-123", headers["session_id"])
	_, exists = headers["x-codex-beta-features"]
	require.False(t, exists)

	upstreamReq := httptest.NewRequest(http.MethodPost, "https://example.com/v1/responses", nil)
	applyHeaderOverrideToRequest(upstreamReq, headers)
	require.Equal(t, "Codex CLI", upstreamReq.Header.Get("Originator"))
	require.Equal(t, "sess-123", upstreamReq.Header.Get("Session_id"))
	require.Empty(t, upstreamReq.Header.Get("X-Codex-Beta-Features"))
}

var httpClientOnce sync.Once

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

func newClientHeaderTestContext(headers map[string]string) *gin.Context {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader("{}"))
	for k, v := range headers {
		ctx.Request.Header.Set(k, v)
	}
	return ctx
}

func TestApplyClientHeadersPassthrough_CopiesClientHeaders(t *testing.T) {
	t.Parallel()

	ctx := newClientHeaderTestContext(map[string]string{
		"User-Agent":      "codex-cli/1.0.0",
		"X-Custom-Header": "custom-value",
		"Originator":      "codex_cli_rs",
	})

	target := http.Header{}
	applyClientHeadersPassthrough(ctx, &target, &relaycommon.RelayInfo{})

	require.Equal(t, "codex-cli/1.0.0", target.Get("User-Agent"))
	require.Equal(t, "custom-value", target.Get("X-Custom-Header"))
	require.Equal(t, "codex_cli_rs", target.Get("Originator"))
}

func TestApplyClientHeadersPassthrough_SkipsUnsafeHeaders(t *testing.T) {
	t.Parallel()

	ctx := newClientHeaderTestContext(map[string]string{
		"User-Agent":        "codex-cli/1.0.0",
		"Authorization":     "Bearer client-token",
		"X-Api-Key":         "client-key",
		"X-Goog-Api-Key":    "client-goog-key",
		"Cookie":            "session=abc",
		"Accept-Encoding":   "gzip",
		"Content-Length":    "123",
		"Connection":        "keep-alive",
		"Host":              "example.com",
		"Transfer-Encoding": "chunked",
	})

	target := http.Header{}
	applyClientHeadersPassthrough(ctx, &target, &relaycommon.RelayInfo{})

	require.Equal(t, "codex-cli/1.0.0", target.Get("User-Agent"))
	require.Empty(t, target.Get("Authorization"))
	require.Empty(t, target.Get("X-Api-Key"))
	require.Empty(t, target.Get("X-Goog-Api-Key"))
	require.Empty(t, target.Get("Cookie"))
	require.Empty(t, target.Get("Accept-Encoding"))
	require.Empty(t, target.Get("Content-Length"))
	require.Empty(t, target.Get("Connection"))
	require.Empty(t, target.Get("Transfer-Encoding"))
}

func TestApplyClientHeadersPassthrough_SkipsChannelTest(t *testing.T) {
	t.Parallel()

	ctx := newClientHeaderTestContext(map[string]string{
		"User-Agent": "Mozilla/5.0 (admin-browser)",
	})

	target := http.Header{}
	applyClientHeadersPassthrough(ctx, &target, &relaycommon.RelayInfo{IsChannelTest: true})

	require.Empty(t, target.Get("User-Agent"))
}

func TestApplyClientHeadersPassthrough_NilSafety(t *testing.T) {
	t.Parallel()

	target := http.Header{}
	require.NotPanics(t, func() {
		applyClientHeadersPassthrough(nil, &target, nil)
		applyClientHeadersPassthrough(nil, nil, nil)
	})
	require.Empty(t, target)
}

// passthroughTestAdaptor is a minimal Adaptor implementation for testing the
// header layering in DoApiRequest: client passthrough -> adaptor setup -> override.
type passthroughTestAdaptor struct{}

func (a *passthroughTestAdaptor) Init(info *relaycommon.RelayInfo) {}

func (a *passthroughTestAdaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	return info.ChannelBaseUrl + "/v1/chat/completions", nil
}

func (a *passthroughTestAdaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	SetupApiRequestHeader(info, c, req)
	req.Set("Authorization", "Bearer "+info.ApiKey)
	return nil
}

func (a *passthroughTestAdaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	return request, nil
}
func (a *passthroughTestAdaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, nil
}
func (a *passthroughTestAdaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	return nil, nil
}
func (a *passthroughTestAdaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	return nil, nil
}
func (a *passthroughTestAdaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	return nil, nil
}
func (a *passthroughTestAdaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	return nil, nil
}
func (a *passthroughTestAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return nil, nil
}
func (a *passthroughTestAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	return nil, nil
}
func (a *passthroughTestAdaptor) GetModelList() []string { return nil }
func (a *passthroughTestAdaptor) GetChannelName() string { return "passthrough-test" }
func (a *passthroughTestAdaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ClaudeRequest) (any, error) {
	return nil, nil
}
func (a *passthroughTestAdaptor) ConvertGeminiRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeminiChatRequest) (any, error) {
	return nil, nil
}

func doApiRequestWithCapturedUpstream(t *testing.T, clientHeaders map[string]string, info *relaycommon.RelayInfo) http.Header {
	t.Helper()

	httpClientOnce.Do(service.InitHttpClient)
	received := make(chan http.Header, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- r.Header
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(upstream.Close)

	ctx := newClientHeaderTestContext(clientHeaders)
	if info.ChannelMeta == nil {
		info.ChannelMeta = &relaycommon.ChannelMeta{}
	}
	info.ChannelBaseUrl = upstream.URL
	info.ApiKey = "channel-key"

	resp, err := DoApiRequest(&passthroughTestAdaptor{}, ctx, info, strings.NewReader(`{"model":"gpt-4"}`))
	require.NoError(t, err)
	require.NotNil(t, resp)
	service.CloseResponseBodyGracefully(resp)

	select {
	case h := <-received:
		return h
	default:
		t.Fatal("upstream did not receive request")
		return nil
	}
}

func TestDoApiRequest_PassesThroughClientHeadersByDefault(t *testing.T) {
	t.Parallel()

	upstreamHeaders := doApiRequestWithCapturedUpstream(t, map[string]string{
		"User-Agent":      "codex-cli/1.0.0",
		"X-Custom-Header": "custom-value",
		"Content-Type":    "application/json",
	}, &relaycommon.RelayInfo{})

	require.Equal(t, "codex-cli/1.0.0", upstreamHeaders.Get("User-Agent"))
	require.Equal(t, "custom-value", upstreamHeaders.Get("X-Custom-Header"))
	// The adaptor must replace the client credential with the channel key.
	require.Equal(t, "Bearer channel-key", upstreamHeaders.Get("Authorization"))
	require.Equal(t, "application/json", upstreamHeaders.Get("Content-Type"))
}

func TestDoApiRequest_DoesNotSendGoDefaultUserAgent(t *testing.T) {
	t.Parallel()

	upstreamHeaders := doApiRequestWithCapturedUpstream(t, map[string]string{
		"User-Agent":   "claude-cli/2.0.0",
		"Content-Type": "application/json",
	}, &relaycommon.RelayInfo{})

	require.NotEqual(t, "Go-http-client/1.1", upstreamHeaders.Get("User-Agent"))
	require.Equal(t, "claude-cli/2.0.0", upstreamHeaders.Get("User-Agent"))
}

func TestDoApiRequest_HeaderOverrideOnlyOverridesSpecifiedKeys(t *testing.T) {
	t.Parallel()

	upstreamHeaders := doApiRequestWithCapturedUpstream(t, map[string]string{
		"User-Agent":      "codex-cli/1.0.0",
		"X-Custom-Header": "custom-value",
		"X-Trace-Id":      "trace-abc",
		"Content-Type":    "application/json",
	}, &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"User-Agent": "my-gateway/9.9",
			},
		},
	})

	// The configured override wins for that key only.
	require.Equal(t, "my-gateway/9.9", upstreamHeaders.Get("User-Agent"))
	// Other client headers must pass through untouched.
	require.Equal(t, "custom-value", upstreamHeaders.Get("X-Custom-Header"))
	require.Equal(t, "trace-abc", upstreamHeaders.Get("X-Trace-Id"))
	// Client credential must never leak upstream; the channel key is used.
	require.Equal(t, "Bearer channel-key", upstreamHeaders.Get("Authorization"))
}

func TestDoApiRequest_ClientAuthorizationNeverLeaksUpstream(t *testing.T) {
	t.Parallel()

	upstreamHeaders := doApiRequestWithCapturedUpstream(t, map[string]string{
		"Authorization": "Bearer client-secret-token",
		"Cookie":        "session=abc",
		"Content-Type":  "application/json",
	}, &relaycommon.RelayInfo{})

	require.Equal(t, "Bearer channel-key", upstreamHeaders.Get("Authorization"))
	require.Empty(t, upstreamHeaders.Get("Cookie"))
}

func setSystemRequestHeadersForTest(t *testing.T, jsonStr string) {
	t.Helper()
	require.NoError(t, setting.UpdateSystemRequestHeadersByJSONString(jsonStr))
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateSystemRequestHeadersByJSONString(""))
	})
}

// F6: 渠道测试（IsChannelTest=true）时全局系统请求头以 set-if-absent 语义注入
func TestDoApiRequest_SystemRequestHeadersAppliedForChannelTest(t *testing.T) {
	setSystemRequestHeadersForTest(t, `{"X-System-Flag":"f6","User-Agent":"new-api-system/1.0","Authorization":"Bearer global-must-not-win"}`)

	upstreamHeaders := doApiRequestWithCapturedUpstream(t, map[string]string{
		"Content-Type": "application/json",
	}, &relaycommon.RelayInfo{IsChannelTest: true})

	require.Equal(t, "f6", upstreamHeaders.Get("X-System-Flag"))
	// 全局 UA 必须生效，Go transport 的 Go-http-client 兜底不得出现
	require.Equal(t, "new-api-system/1.0", upstreamHeaders.Get("User-Agent"))
	// 全局配置不得覆盖路径默认认证头
	require.Equal(t, "Bearer channel-key", upstreamHeaders.Get("Authorization"))
}

// F6: 用户 relay 请求（IsChannelTest=false）不得携带全局系统请求头
func TestDoApiRequest_SystemRequestHeadersSkippedForUserRelay(t *testing.T) {
	setSystemRequestHeadersForTest(t, `{"X-System-Flag":"f6","User-Agent":"new-api-system/1.0"}`)

	upstreamHeaders := doApiRequestWithCapturedUpstream(t, map[string]string{
		"Content-Type": "application/json",
	}, &relaycommon.RelayInfo{IsChannelTest: false})

	require.Empty(t, upstreamHeaders.Get("X-System-Flag"))
	require.NotEqual(t, "new-api-system/1.0", upstreamHeaders.Get("User-Agent"))
}

// F6: 渠道级 HeaderOverride 优先于全局设置
func TestDoApiRequest_ChannelOverrideBeatsSystemRequestHeaders(t *testing.T) {
	setSystemRequestHeadersForTest(t, `{"X-System-Flag":"global","X-Other":"global-other"}`)

	upstreamHeaders := doApiRequestWithCapturedUpstream(t, map[string]string{
		"Content-Type": "application/json",
	}, &relaycommon.RelayInfo{
		IsChannelTest: true,
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"X-System-Flag": "channel",
			},
		},
	})

	require.Equal(t, "channel", upstreamHeaders.Get("X-System-Flag"))
	require.Equal(t, "global-other", upstreamHeaders.Get("X-Other"))
}
