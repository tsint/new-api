package kling

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"

	"github.com/stretchr/testify/require"
)

// F6: FetchTask 注入全局系统请求头；adaptor 显式设置的 User-Agent（kling-sdk/1.0）不被全局覆盖
func TestFetchTask_AppliesSystemRequestHeaders(t *testing.T) {
	service.InitHttpClient()
	require.NoError(t, setting.UpdateSystemRequestHeadersByJSONString(`{"X-System-Flag":"f6","User-Agent":"global-ua-must-not-win","Authorization":"Bearer global-must-not-win"}`))
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateSystemRequestHeadersByJSONString(""))
	})

	received := make(chan http.Header, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- r.Header
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(upstream.Close)

	a := &TaskAdaptor{}
	resp, err := a.FetchTask(upstream.URL, "kling-key", map[string]any{
		"task_id": "t1",
		"action":  constant.TaskActionGenerate,
	}, "")
	require.NoError(t, err)
	defer resp.Body.Close()
	_, _ = io.ReadAll(resp.Body)

	h := <-received
	require.Equal(t, "f6", h.Get("X-System-Flag"))
	require.Equal(t, "kling-sdk/1.0", h.Get("User-Agent"))
	require.NotEqual(t, "Bearer global-must-not-win", h.Get("Authorization"))
}
