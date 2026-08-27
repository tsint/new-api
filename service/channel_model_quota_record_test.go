package service

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"

	"github.com/gin-gonic/gin"
)

func httptestRequest() *http.Request {
	return httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
}

func newGroupCtx(usingGroup, autoGroup string) *gin.Context {
	gin.SetMode(gin.TestMode)
	c := &gin.Context{}
	c.Request = httptestRequest()
	if usingGroup != "" {
		common.SetContextKey(c, constant.ContextKeyUsingGroup, usingGroup)
	}
	if autoGroup != "" {
		common.SetContextKey(c, constant.ContextKeyAutoGroup, autoGroup)
	}
	return c
}

func TestResolveRequestGroupFromCtx(t *testing.T) {
	if got := ResolveRequestGroupFromCtx(newGroupCtx("vip", "")); got != "vip" {
		t.Fatalf("using only = %q want vip", got)
	}
	if got := ResolveRequestGroupFromCtx(newGroupCtx("vip", "g2")); got != "g2" {
		t.Fatalf("autogroup must win = %q want g2", got)
	}
	if got := ResolveRequestGroupFromCtx(newGroupCtx("", "")); got != "" {
		t.Fatalf("empty ctx = %q want \"\"", got)
	}
}

func TestRecordChannelQuotaUsage(t *testing.T) {
	orig := cqStore
	store := NewChannelQuotaStore(nil)
	cqStore = store
	t.Cleanup(func() { cqStore = orig })

	now := time.Unix(1800000000, 0)
	idx := QuotaBlockIndex(now)

	RecordChannelQuotaUsage(store, 9, "default", "gpt-4o", idx, 700, 300)

	if got := store.GetUsed(9, "default", "gpt-4o", idx); got != 1000 {
		t.Fatalf("recorded=%d want 1000(prompt+completion)", got)
	}
}
