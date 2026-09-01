package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func setSystemRequestHeadersForTest(t *testing.T, jsonStr string) {
	t.Helper()
	require.NoError(t, setting.UpdateSystemRequestHeadersByJSONString(jsonStr))
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateSystemRequestHeadersByJSONString(""))
	})
}

// F6: GetResponseBody（拉模型/计费查询共用出口）注入全局系统请求头，set-if-absent
func TestGetResponseBody_AppliesSystemRequestHeaders(t *testing.T) {
	setSystemRequestHeadersForTest(t, `{"X-System-Flag":"f6","User-Agent":"new-api-system/1.0","Authorization":"Bearer global-must-not-win"}`)

	received := make(chan http.Header, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- r.Header
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(upstream.Close)

	channel := &model.Channel{}
	_, err := GetResponseBody(http.MethodGet, upstream.URL, channel, GetAuthHeader("channel-key"))
	require.NoError(t, err)

	h := <-received
	require.Equal(t, "f6", h.Get("X-System-Flag"))
	require.Equal(t, "new-api-system/1.0", h.Get("User-Agent"))
	require.Equal(t, "Bearer channel-key", h.Get("Authorization"))
}

func TestGetResponseBody_NoSystemRequestHeadersByDefault(t *testing.T) {
	received := make(chan http.Header, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- r.Header
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(upstream.Close)

	channel := &model.Channel{}
	_, err := GetResponseBody(http.MethodGet, upstream.URL, channel, GetAuthHeader("channel-key"))
	require.NoError(t, err)

	h := <-received
	require.Empty(t, h.Get("X-System-Flag"))
	require.NotEqual(t, "new-api-system/1.0", h.Get("User-Agent"))
}

// F6: FetchModels（未保存渠道的临时拉取）通用分支注入全局系统请求头
func TestFetchModels_AppliesSystemRequestHeaders(t *testing.T) {
	setSystemRequestHeadersForTest(t, `{"X-System-Flag":"f6","Authorization":"Bearer global-must-not-win"}`)

	received := make(chan http.Header, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- r.Header
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"gpt-4o-mini"}]}`))
	}))
	t.Cleanup(upstream.Close)

	body := fmt.Sprintf(`{"base_url":%q,"type":%d,"key":"channel-key"}`, upstream.URL, constant.ChannelTypeOpenAI)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/fetch_models", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	FetchModels(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	h := <-received
	require.Equal(t, "f6", h.Get("X-System-Flag"))
	require.Equal(t, "Bearer channel-key", h.Get("Authorization"))
}
