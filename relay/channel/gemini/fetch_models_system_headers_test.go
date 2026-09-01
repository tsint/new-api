package gemini

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"

	"github.com/stretchr/testify/require"
)

func initHttpClientForTest(t *testing.T) {
	t.Helper()
	service.InitHttpClient()
}

// F6: FetchGeminiModels 注入全局系统请求头，且不覆盖 x-goog-api-key
func TestFetchGeminiModels_AppliesSystemRequestHeaders(t *testing.T) {
	initHttpClientForTest(t)
	require.NoError(t, setting.UpdateSystemRequestHeadersByJSONString(`{"X-System-Flag":"f6","x-goog-api-key":"global-must-not-win"}`))
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

	_, err := FetchGeminiModels(upstream.URL, "gemini-key", "")
	require.NoError(t, err)

	h := <-received
	require.Equal(t, "f6", h.Get("X-System-Flag"))
	require.Equal(t, "gemini-key", h.Get("x-goog-api-key"))
}

func TestFetchGeminiModels_NoSystemRequestHeadersByDefault(t *testing.T) {
	initHttpClientForTest(t)
	received := make(chan http.Header, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- r.Header
		_, _ = w.Write([]byte(`{"models":[]}`))
	}))
	t.Cleanup(upstream.Close)

	_, err := FetchGeminiModels(upstream.URL, "gemini-key", "")
	require.NoError(t, err)

	h := <-received
	require.Empty(t, h.Get("X-System-Flag"))
}
