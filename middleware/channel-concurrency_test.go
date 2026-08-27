package middleware

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

// ---- 渠道并发任务数限制（F3） ----

var errInjectedStoreFailure = errors.New("injected store failure")

type fakeErrStore struct{ calls atomic.Int32 }

func (f *fakeErrStore) AcquireConcurrency(string) (int64, error) {
	f.calls.Add(1)
	return 0, errInjectedStoreFailure
}
func (f *fakeErrStore) ReleaseConcurrency(string)          {}
func (f *fakeErrStore) GetConcurrency(string) int64        { return 0 }
func (f *fakeErrStore) IncSecondRate(string, time.Time) (int64, error) {
	return 0, errInjectedStoreFailure
}

func TestTryAcquireChannelConcurrency_Unlimited(t *testing.T) {
	store := service.NewUserRateLimitStore(nil)
	acquired, counted := acquireChannelConcurrencySlot(store, 5, 0)
	if !acquired || counted {
		t.Fatalf("unlimited channel must pass without counting, got acquired=%v counted=%v", acquired, counted)
	}
	if v := store.GetConcurrency(channelConcurrencyKey(5)); v != 0 {
		t.Fatalf("counter side-effect detected: %d", v)
	}
}

func TestTryAcquireChannelConcurrency_BlocksAtLimitThenReleases(t *testing.T) {
	store := service.NewUserRateLimitStore(nil)
	const (
		id    = 12
		limit = int64(2)
	)

	for i := 0; i < 2; i++ {
		acquired, counted := acquireChannelConcurrencySlot(store, id, limit)
		if !acquired || !counted {
			t.Fatalf("slot %d should be acquirable and counted, got %v/%v", i+1, acquired, counted)
		}
	}

	// 在途名额未释放前，第 3 个必须被拒（流式长响应等价场景）
	if acquired, _ := acquireChannelConcurrencySlot(store, id, limit); acquired {
		t.Fatal("third acquire over limit must be rejected")
	}
	if v := store.GetConcurrency(channelConcurrencyKey(id)); v != 2 {
		t.Fatalf("after reject counter = %d want 2 (rejection must not alter count)", v)
	}

	releaseChannelConcurrencySlot(store, id)
	waitChannelCounter(t, store, id, 1)

	if acquired, counted := acquireChannelConcurrencySlot(store, id, limit); !acquired || !counted {
		t.Fatalf("acquire after one completion should succeed, got %v/%v", acquired, counted)
	}
	releaseChannelConcurrencySlot(store, id) // 归还原占用的 B 名额
	releaseChannelConcurrencySlot(store, id) // 归还本轮新占用的名额
	waitChannelCounter(t, store, id, 0)
}

func waitChannelCounter(t *testing.T, store service.UserRateLimitStore, id int, want int64) {
	t.Helper()
	key := channelConcurrencyKey(id)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if store.GetConcurrency(key) == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("counter at key %s never reached %d", key, want)
}

func TestAcquireChannelConcurrency_StoreErrorDegradesToAllow(t *testing.T) {
	store := &fakeErrStore{}
	acquired, counted := acquireChannelConcurrencySlot(store, 21, 3)
	if !acquired {
		t.Fatal("store failure must degrade to allow")
	}
	if counted {
		t.Fatal("degraded allow must not require release")
	}
	if store.calls.Load() != 1 {
		t.Fatalf("expected single acquire attempt, got %d", store.calls.Load())
	}
}

func TestAcquireChannelConcurrency_ConcurrentBurstExactBound(t *testing.T) {
	const (
		id    = 33
		limit = int64(3)
		total = 40
	)
	store := service.NewUserRateLimitStore(nil)

	var wg sync.WaitGroup
	results := make([]string, total)
	for i := 0; i < total; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			acquired, counted := acquireChannelConcurrencySlot(store, id, limit)
			switch {
			case acquired && counted:
				results[idx] = "held"
			case acquired && !counted:
				results[idx] = "degraded"
			default:
				results[idx] = "rejected"
			}
		}(i)
	}
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("burst did not settle")
	}

	var held, degraded, rejected int
	for _, r := range results {
		switch r {
		case "held":
			held++
		case "degraded":
			degraded++
		case "rejected":
			rejected++
		default:
			t.Fatalf("unexpected result %q", r)
		}
	}
	if degraded != 0 {
		t.Fatalf("memory store never fails, got %d degraded", degraded)
	}
	if held != int(limit) {
		t.Fatalf("memory store is exact-bound while slots unreleased: held=%d want %d", held, limit)
	}
	if held+rejected != total {
		t.Fatalf("results missing: %d+%d != %d", held, rejected, total)
	}

	for i := 0; i < int(limit); i++ { // 清理持有名额，避免影响其他用例
		releaseChannelConcurrencySlot(store, id)
	}
	waitChannelCounter(t, store, id, 0)
}

func TestReleaseChannelConcurrencySlot_NeverNegativeOrLingering(t *testing.T) {
	store := service.NewUserRateLimitStore(nil)
	const id = 44
	for i := 0; i < 3; i++ {
		releaseChannelConcurrencySlot(store, id)
	}
	if v := store.GetConcurrency(channelConcurrencyKey(id)); v < 0 {
		t.Fatalf("counter went negative: %d", v)
	}
	if v := store.GetConcurrency(channelConcurrencyKey(id)); v != 0 {
		t.Fatalf("idle channel counter should be absent/zero, got %d", v)
	}
}

func TestAbortChannelConcurrencyReached_WritesOpenAiStyle429(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Set(common.RequestIdKey, "req-test-1")

	abortChannelConcurrencyReached(c, 77, 9)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("status=%d want 429", w.Code)
	}
	var body map[string]interface{}
	if err := common.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("body not json: %v raw=%s", err, w.Body.String())
	}
	errObj, ok := body["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing error object: %s", w.Body.String())
	}
	if typ, _ := errObj["type"].(string); typ != "channel_concurrency_exceeded" {
		t.Fatalf("type=%q want channel_concurrency_exceeded", typ)
	}
	if code, _ := errObj["code"].(string); code != "concurrency_limit_exceeded" {
		t.Fatalf("code=%q want concurrency_limit_exceeded", code)
	}
	if msg, _ := errObj["message"].(string); !strings.Contains(msg, "并发") || !strings.Contains(msg, "9") {
		t.Fatalf("message should mention 并发上限与数值9, got %q", msg)
	}
	if cid, _ := errObj["channel_id"].(float64); cid != 77 {
		t.Fatalf("channel_id=%v want 77", cid)
	}
	if cl, _ := errObj["concurrency_limit"].(float64); cl != 9 {
		t.Fatalf("concurrency_limit=%v want 9", cl)
	}
	if !c.IsAborted() {
		t.Fatal("context must be aborted")
	}
}

func TestHandleChannelConcurrency_CallSiteContract(t *testing.T) {
	// 复现 distributor 调用形态：首次占用阻塞期间后续请求被 429，
	// 首个请求完成后名额归还且链路继续可用。
	gin.SetMode(gin.TestMode)
	store := service.NewUserRateLimitStore(nil)
	r := gin.New()

	blocker := make(chan struct{})
	r.POST("/relay", func(c *gin.Context) {
		ok, release := handleChannelConcurrency(c, store, 55, 1)
		if !ok {
			c.Status(http.StatusTeapot)
			return
		}
		if release != nil {
			defer release()
		}
		<-blocker
		c.Status(http.StatusOK)
	})

	first := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/relay", nil))
		first <- w
	}()
	waitChannelCounter(t, store, 55, 1)

	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, httptest.NewRequest(http.MethodPost, "/relay", nil))
	if w2.Code != http.StatusTooManyRequests {
		t.Fatalf("second concurrent got %d want 429 body=%s", w2.Code, w2.Body.String())
	}

	close(blocker)
	select {
	case w := <-first:
		if w.Code != http.StatusOK {
			t.Fatalf("first request finished with %d body=%s", w.Code, w.Body.String())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("first request never completed")
	}
	waitChannelCounter(t, store, 55, 0)
}

func TestChannelConcurrencyUsesIndependentKeyspace(t *testing.T) {
	store := service.NewUserRateLimitStore(nil)
	acquireChannelConcurrencySlot(store, 88, 5)
	defer releaseChannelConcurrencySlot(store, 88)

	key := channelConcurrencyKey(88)
	if v := store.GetConcurrency(key); v != 1 {
		t.Fatalf("channel key counter = %d want 1", v)
	}
	if v := store.GetConcurrency("88"); v != 0 {
		t.Fatalf("user-style bare key unexpectedly touched: %d", v)
	}
	if strings.HasPrefix(key, "ugrl") {
		t.Fatalf("must not collide with user-group prefix: %q", key)
	}
}
