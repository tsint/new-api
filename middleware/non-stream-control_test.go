package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

// ---- F4: 渠道级禁用非流请求 ----

func newNonStreamTestContext(t *testing.T, setting any) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	if setting != nil {
		common.SetContextKey(c, constant.ContextKeyChannelSetting, setting)
	}
	return c
}

func TestCheckChannelNonStreamSupport(t *testing.T) {
	disabled := dto.ChannelSettings{DisableNonStreaming: true}
	tests := []struct {
		name        string
		relayFormat types.RelayFormat
		isStream    bool
		setting     any
		wantErr     bool
	}{
		{"non-stream on disabled channel rejected", types.RelayFormatOpenAI, false, disabled, true},
		{"stream passes on disabled channel", types.RelayFormatOpenAI, true, disabled, false},
		{"realtime ws exempt on disabled channel", types.RelayFormatOpenAIRealtime, false, disabled, false},
		{"non-stream on normal channel passes", types.RelayFormatOpenAI, false, dto.ChannelSettings{}, false},
		{"missing setting fails open", types.RelayFormatOpenAI, false, nil, false},
		{"corrupted setting fails open", types.RelayFormatOpenAI, false, "not-json", false},
		{"claude format non-stream rejected", types.RelayFormatClaude, false, disabled, true},
		{"gemini format non-stream rejected", types.RelayFormatGemini, false, disabled, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newNonStreamTestContext(t, tt.setting)
			err := CheckChannelNonStreamSupport(c, tt.relayFormat, tt.isStream)
			if !tt.wantErr {
				if err != nil {
					t.Fatalf("expected pass, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected rejection, got nil")
			}
			if err.StatusCode != http.StatusForbidden {
				t.Fatalf("status=%d want 403", err.StatusCode)
			}
			if err.GetErrorCode() != types.ErrorCodeNonStreamingDisabled {
				t.Fatalf("code=%q want %q", err.GetErrorCode(), types.ErrorCodeNonStreamingDisabled)
			}
			if !types.IsSkipRetryError(err) {
				t.Fatal("rejection must carry skip-retry")
			}
			if types.IsChannelError(err) {
				t.Fatal("code must not carry channel: prefix (would trigger auto channel retry)")
			}
			if !strings.Contains(err.Error(), "非流") {
				t.Fatalf("message should mention 非流, got %q", err.Error())
			}
		})
	}
}

func TestCheckChannelNonStreamSupport_ResponseShape(t *testing.T) {
	// 验证经 Relay 兜底输出后 body 形态：type=new_api_error, code=non_streaming_disabled
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	common.SetContextKey(c, constant.ContextKeyChannelSetting, dto.ChannelSettings{DisableNonStreaming: true})

	err := CheckChannelNonStreamSupport(c, types.RelayFormatOpenAI, false)
	if err == nil {
		t.Fatal("expected rejection")
	}
	openaiErr := err.ToOpenAIError()
	if fmt.Sprintf("%v", openaiErr.Code) != string(types.ErrorCodeNonStreamingDisabled) {
		t.Fatalf("body code=%v want non_streaming_disabled", openaiErr.Code)
	}
	if openaiErr.Type != string(types.ErrorTypeNewAPIError) {
		t.Fatalf("body type=%v want new_api_error", openaiErr.Type)
	}
	if err.StatusCode != http.StatusForbidden {
		t.Fatalf("status=%d want 403", err.StatusCode)
	}
}

// ---- F5: 非流请求每分钟限速 ----

type fakeMinuteStore struct {
	calls  atomic.Int32
	mu     sync.Mutex
	counts map[string]int64
	fail   bool
}

func (f *fakeMinuteStore) AcquireConcurrency(string) (int64, error) { return 0, nil }
func (f *fakeMinuteStore) ReleaseConcurrency(string)                {}
func (f *fakeMinuteStore) GetConcurrency(string) int64              { return 0 }
func (f *fakeMinuteStore) IncSecondRate(string, time.Time) (int64, error) {
	return 0, nil
}
func (f *fakeMinuteStore) IncMinuteRate(key string, now time.Time) (int64, error) {
	f.calls.Add(1)
	if f.fail {
		return 0, errInjectedStoreFailure
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.counts == nil {
		f.counts = make(map[string]int64)
	}
	f.counts[key]++
	return f.counts[key], nil
}

func newRateLimitTestContext(t *testing.T, userId int, tokenGroup, userGroup string) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Set("id", userId)
	if tokenGroup != "" {
		common.SetContextKey(c, constant.ContextKeyTokenGroup, tokenGroup)
	}
	if userGroup != "" {
		common.SetContextKey(c, constant.ContextKeyUserGroup, userGroup)
	}
	return c
}

// withNonStreamRateLimit 临时改写全局限速配置，测试结束恢复
func withNonStreamRateLimit(t *testing.T, enabled bool, count int, groupJSON string) {
	t.Helper()
	oldEnabled, oldCount, oldGroup := setting.NonStreamRequestRateLimitEnabled,
		setting.NonStreamRequestRateLimitCount, setting.NonStreamRateLimitGroup2JSONString()
	setting.NonStreamRequestRateLimitEnabled = enabled
	setting.NonStreamRequestRateLimitCount = count
	if err := setting.UpdateNonStreamRateLimitGroupByJSONString(groupJSON); err != nil {
		t.Fatalf("bad group json in test setup: %v", err)
	}
	t.Cleanup(func() {
		setting.NonStreamRequestRateLimitEnabled = oldEnabled
		setting.NonStreamRequestRateLimitCount = oldCount
		if err := setting.UpdateNonStreamRateLimitGroupByJSONString(oldGroup); err != nil {
			t.Fatalf("restore group failed: %v", err)
		}
	})
}

func TestCheckNonStreamRateLimit_CountsUntilLimitThenRejects(t *testing.T) {
	withNonStreamRateLimit(t, true, 2, `{}`)
	store := &fakeMinuteStore{}

	for i := 0; i < 2; i++ {
		c := newRateLimitTestContext(t, 1, "", "")
		if err := CheckNonStreamRateLimit(c, types.RelayFormatOpenAI, false, store); err != nil {
			t.Fatalf("call %d should pass, got %v", i+1, err)
		}
	}

	c := newRateLimitTestContext(t, 1, "", "")
	err := CheckNonStreamRateLimit(c, types.RelayFormatOpenAI, false, store)
	if err == nil {
		t.Fatal("third call over limit must be rejected")
	}
	if err.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status=%d want 429", err.StatusCode)
	}
	if err.GetErrorCode() != types.ErrorCodeNonStreamRateLimitExceed {
		t.Fatalf("code=%q want non_stream_rate_limit_exceeded", err.GetErrorCode())
	}
	if !types.IsSkipRetryError(err) {
		t.Fatal("429 must carry skip-retry")
	}
	if types.IsChannelError(err) {
		t.Fatal("must not trigger channel retry")
	}
	if msg := err.Error(); !strings.Contains(msg, "2") || !strings.Contains(msg, "非流") {
		t.Fatalf("message should mention limit 2 and 非流, got %q", msg)
	}
}

func TestCheckNonStreamRateLimit_Exemptions(t *testing.T) {
	withNonStreamRateLimit(t, true, 1, `{}`)
	store := &fakeMinuteStore{}

	// 流式请求豁免且不计数
	c := newRateLimitTestContext(t, 1, "", "")
	if err := CheckNonStreamRateLimit(c, types.RelayFormatOpenAI, true, store); err != nil {
		t.Fatalf("stream must be exempt, got %v", err)
	}
	// realtime 豁免
	c = newRateLimitTestContext(t, 1, "", "")
	if err := CheckNonStreamRateLimit(c, types.RelayFormatOpenAIRealtime, false, store); err != nil {
		t.Fatalf("realtime must be exempt, got %v", err)
	}
	if store.calls.Load() != 0 {
		t.Fatalf("exempt requests must not count, calls=%d", store.calls.Load())
	}
}

func TestCheckNonStreamRateLimit_DisabledOrUnlimited(t *testing.T) {
	store := &fakeMinuteStore{}

	// 总开关关闭
	withNonStreamRateLimit(t, false, 2, `{}`)
	c := newRateLimitTestContext(t, 1, "", "")
	if err := CheckNonStreamRateLimit(c, types.RelayFormatOpenAI, false, store); err != nil {
		t.Fatalf("disabled switch must allow, got %v", err)
	}
	if store.calls.Load() != 0 {
		t.Fatal("disabled switch must not touch store")
	}

	// limit=0 不限制
	withNonStreamRateLimit(t, true, 0, `{}`)
	for i := 0; i < 5; i++ {
		c := newRateLimitTestContext(t, 1, "", "")
		if err := CheckNonStreamRateLimit(c, types.RelayFormatOpenAI, false, store); err != nil {
			t.Fatalf("limit 0 must be unlimited, got %v at call %d", err, i+1)
		}
	}
	if store.calls.Load() != 0 {
		t.Fatal("unlimited must not touch store")
	}
}

func TestCheckNonStreamRateLimit_GroupOverride(t *testing.T) {
	withNonStreamRateLimit(t, true, 2, `{"vip":0,"low":1}`)
	store := &fakeMinuteStore{}

	// vip 组条目 0 => 不限制（覆盖全局 2）
	for i := 0; i < 3; i++ {
		c := newRateLimitTestContext(t, 1, "vip", "")
		if err := CheckNonStreamRateLimit(c, types.RelayFormatOpenAI, false, store); err != nil {
			t.Fatalf("vip group unlimited, got %v at call %d", err, i+1)
		}
	}

	// low 组限 1 => 第 2 次被拒（覆盖全局 2 且向下生效）
	c := newRateLimitTestContext(t, 2, "low", "")
	if err := CheckNonStreamRateLimit(c, types.RelayFormatOpenAI, false, store); err != nil {
		t.Fatalf("low first call should pass, got %v", err)
	}
	c = newRateLimitTestContext(t, 2, "low", "")
	if err := CheckNonStreamRateLimit(c, types.RelayFormatOpenAI, false, store); err == nil {
		t.Fatal("low second call must be rejected")
	}

	// 无 token 组时回退用户组
	store2 := &fakeMinuteStore{}
	c = newRateLimitTestContext(t, 3, "", "low")
	if err := CheckNonStreamRateLimit(c, types.RelayFormatOpenAI, false, store2); err != nil {
		t.Fatalf("user group fallback first call, got %v", err)
	}
	c = newRateLimitTestContext(t, 3, "", "low")
	if err := CheckNonStreamRateLimit(c, types.RelayFormatOpenAI, false, store2); err == nil {
		t.Fatal("user group fallback must apply low limit")
	}
}

func TestCheckNonStreamRateLimit_PerUserIsolation(t *testing.T) {
	withNonStreamRateLimit(t, true, 1, `{}`)
	store := &fakeMinuteStore{}

	c := newRateLimitTestContext(t, 1, "", "")
	if err := CheckNonStreamRateLimit(c, types.RelayFormatOpenAI, false, store); err != nil {
		t.Fatalf("user1 first call, got %v", err)
	}
	// 用户 2 不受用户 1 影响
	c = newRateLimitTestContext(t, 2, "", "")
	if err := CheckNonStreamRateLimit(c, types.RelayFormatOpenAI, false, store); err != nil {
		t.Fatalf("user2 must have its own budget, got %v", err)
	}
	// 用户 1 已达上限
	c = newRateLimitTestContext(t, 1, "", "")
	if err := CheckNonStreamRateLimit(c, types.RelayFormatOpenAI, false, store); err == nil {
		t.Fatal("user1 second call must be rejected")
	}
}

func TestCheckNonStreamRateLimit_StoreFailureDegradesToAllow(t *testing.T) {
	withNonStreamRateLimit(t, true, 1, `{}`)
	store := &fakeMinuteStore{fail: true}

	c := newRateLimitTestContext(t, 1, "", "")
	if err := CheckNonStreamRateLimit(c, types.RelayFormatOpenAI, false, store); err != nil {
		t.Fatalf("store failure must degrade to allow, got %v", err)
	}
	if store.calls.Load() != 1 {
		t.Fatalf("expected single attempt, got %d", store.calls.Load())
	}
}
