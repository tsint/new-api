package suno

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"

	"github.com/stretchr/testify/require"
)

// F6: FetchTask 注入全局系统请求头，且不覆盖 Authorization
func TestFetchTask_AppliesSystemRequestHeaders(t *testing.T) {
	service.InitHttpClient()
	require.NoError(t, setting.UpdateSystemRequestHeadersByJSONString(`{"X-System-Flag":"f6","Authorization":"Bearer global-must-not-win"}`))
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateSystemRequestHeadersByJSONString(""))
	})

	received := make(chan http.Header, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- r.Header
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"success","data":{}}`))
	}))
	t.Cleanup(upstream.Close)

	a := &TaskAdaptor{}
	resp, err := a.FetchTask(upstream.URL, "suno-key", map[string]any{"task_id": "t1"}, "")
	require.NoError(t, err)
	defer resp.Body.Close()
	_, _ = io.ReadAll(resp.Body)

	h := <-received
	require.Equal(t, "f6", h.Get("X-System-Flag"))
	require.Equal(t, "Bearer suno-key", h.Get("Authorization"))
}
