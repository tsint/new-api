package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/setting"

	"github.com/stretchr/testify/require"
)

func setSystemRequestHeadersForTest(t *testing.T, jsonStr string) {
	t.Helper()
	require.NoError(t, setting.UpdateSystemRequestHeadersByJSONString(jsonStr))
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateSystemRequestHeadersByJSONString(""))
	})
}

// F6: Codex 用量查询注入全局系统请求头，且不覆盖 Authorization / originator
func TestFetchCodexWhamUsage_AppliesSystemRequestHeaders(t *testing.T) {
	setSystemRequestHeadersForTest(t, `{"X-System-Flag":"f6","Authorization":"Bearer global-must-not-win","originator":"global-must-not-win"}`)

	received := make(chan http.Header, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- r.Header
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(upstream.Close)

	_, _, err := FetchCodexWhamUsage(context.Background(), &http.Client{}, upstream.URL, "access-token", "acc-1")
	require.NoError(t, err)

	h := <-received
	require.Equal(t, "f6", h.Get("X-System-Flag"))
	require.Equal(t, "Bearer access-token", h.Get("Authorization"))
	require.Equal(t, "codex_cli_rs", h.Get("originator"))
}

// F6: Codex OAuth 刷新注入全局系统请求头
func TestRefreshCodexOAuthToken_AppliesSystemRequestHeaders(t *testing.T) {
	setSystemRequestHeadersForTest(t, `{"X-System-Flag":"f6","Content-Type":"global-must-not-win"}`)

	received := make(chan http.Header, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- r.Header
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"a","refresh_token":"r","expires_in":3600}`))
	}))
	t.Cleanup(upstream.Close)

	_, err := refreshCodexOAuthToken(context.Background(), &http.Client{}, upstream.URL, "cid", "rt")
	require.NoError(t, err)

	h := <-received
	require.Equal(t, "f6", h.Get("X-System-Flag"))
	require.Equal(t, "application/x-www-form-urlencoded", h.Get("Content-Type"))
}

// F6: Codex 授权码交换注入全局系统请求头
func TestExchangeCodexAuthorizationCode_AppliesSystemRequestHeaders(t *testing.T) {
	setSystemRequestHeadersForTest(t, `{"X-System-Flag":"f6"}`)

	received := make(chan http.Header, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- r.Header
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"a","refresh_token":"r","expires_in":3600}`))
	}))
	t.Cleanup(upstream.Close)

	_, err := exchangeCodexAuthorizationCode(context.Background(), &http.Client{}, upstream.URL, "cid", "code", "verifier", "http://localhost/cb")
	require.NoError(t, err)

	h := <-received
	require.Equal(t, "f6", h.Get("X-System-Flag"))
}
