package ollama

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/setting"

	"github.com/stretchr/testify/require"
)

// F6: FetchOllamaModels / PullOllamaModel 注入全局系统请求头，且不覆盖 Authorization
func TestFetchOllamaModels_AppliesSystemRequestHeaders(t *testing.T) {
	require.NoError(t, setting.UpdateSystemRequestHeadersByJSONString(`{"X-System-Flag":"f6","Authorization":"Bearer global-must-not-win"}`))
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateSystemRequestHeadersByJSONString(""))
	})

	received := make(chan http.Header, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- r.Header
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"models":[]}`))
	}))
	t.Cleanup(upstream.Close)

	_, err := FetchOllamaModels(upstream.URL, "ollama-key")
	require.NoError(t, err)

	h := <-received
	require.Equal(t, "f6", h.Get("X-System-Flag"))
	require.Equal(t, "Bearer ollama-key", h.Get("Authorization"))
}

func TestPullOllamaModel_AppliesSystemRequestHeaders(t *testing.T) {
	require.NoError(t, setting.UpdateSystemRequestHeadersByJSONString(`{"X-System-Flag":"f6"}`))
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateSystemRequestHeadersByJSONString(""))
	})

	received := make(chan http.Header, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- r.Header
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success"}`))
	}))
	t.Cleanup(upstream.Close)

	require.NoError(t, PullOllamaModel(upstream.URL, "ollama-key", "llama3"))

	h := <-received
	require.Equal(t, "f6", h.Get("X-System-Flag"))
	require.Equal(t, "Bearer ollama-key", h.Get("Authorization"))
}
