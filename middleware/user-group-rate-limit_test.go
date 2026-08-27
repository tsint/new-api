package middleware

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"

	"github.com/gin-gonic/gin"
)

// ---- 测试基建 ----

type relayFixture struct {
	router *gin.Engine
	store  service.UserRateLimitStore
}

// gateHandler 用闸门控制终端处理器的放行时机。
type gateHandler struct {
	arrived chan struct{}
	release chan struct{}
	once    sync.Once
}

func newGate() *gateHandler {
	return &gateHandler{arrived: make(chan struct{}), release: make(chan struct{})}
}

func setSetting(t *testing.T, jsonStr string) {
	t.Helper()
	if err := setting.UpdateUserGroupRateLimitSettingsByJSONString(jsonStr); err != nil {
		t.Fatalf("setup settings failed: %v", err)
	}
	t.Cleanup(func() { _ = setting.UpdateUserGroupRateLimitSettingsByJSONString(`{}`) })
}

func newRelayRouter(groups []string, userId int, gate *gateHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(func(c *gin.Context) {
		common.SetContextKey(c, constant.ContextKeyUserId, userId)
		common.SetContextKey(c, constant.ContextKeyUserGroupList, groups)
		c.Next()
	})
	r.Use(UserGroupRateLimit())

	if gate == nil {
		r.POST("/relay", func(c *gin.Context) { c.Status(http.StatusOK) })
		return r
	}

	r.POST("/relay", func(c *gin.Context) {
		gate.once.Do(func() { close(gate.arrived) })
		<-gate.release
		c.Status(http.StatusOK)
	})
	return r
}

func doOn(r *gin.Engine) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/relay", nil)
	r.ServeHTTP(w, req)
	return w
}

func decodeErrorBody(t *testing.T, w *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var body map[string]interface{}
	if err := common.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("body not json: %v, raw=%s", err, w.Body.String())
	}
	errObj, ok := body["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing error object: %s", w.Body.String())
	}
	return errObj
}

func waitArrived(t *testing.T, ch chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("handler never arrived")
	}
}

// ---- 用例 ----

func TestUserGroupRateLimitUnconfiguredPassesThrough(t *testing.T) {
	setSetting(t, `{}`)
	r := newRelayRouter([]string{"vip"}, 1, nil)
	if w := doOn(r); w.Code != http.StatusOK {
		t.Fatalf("unconfigured group should pass, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestUserGroupRateLimitConcurrencyBlocksThenReleases(t *testing.T) {
	setSetting(t, `{"vip":{"concurrency":1}}`)
	ugrlStore = service.NewUserRateLimitStore(nil)

	const userId = 7
	gate := newGate()
	r := newRelayRouter([]string{"vip"}, userId, gate)

	go doOn(r) // 第一个请求占用唯一名额并阻塞
	waitArrived(t, gate.arrived)

	w2 := doOn(r) // 并发第二个请求应被立即拒绝
	if w2.Code != http.StatusTooManyRequests {
		t.Fatalf("second concurrent request got %d want 429", w2.Code)
	}
	errObj := decodeErrorBody(t, w2)
	if code, _ := errObj["code"].(string); code != "rate_limit_exceeded" {
		t.Fatalf("error code = %q want rate_limit_exceeded", code)
	}
	if msg, _ := errObj["message"].(string); !strings.Contains(msg, "并发") {
		t.Fatalf("concurrency message should mention 并发, got %q", msg)
	}

	close(gate.release)         // 释放第一个请求
	waitSlotReleased(t, userId) // 等待名额归还

	w3 := doOn(r)
	if w3.Code != http.StatusOK {
		t.Fatalf("request after release got %d want 200", w3.Code)
	}
}

// waitSlotReleased 轮询等待并发名额归还（带超时）。
func waitSlotReleased(t *testing.T, userId int) {
	t.Helper()
	key := strconv.Itoa(userId)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if ugrlStore.GetConcurrency(key) == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("concurrency slot never released")
}

func TestUserGroupRateLimitPerSecondRate(t *testing.T) {
	setSetting(t, `{"vip":{"connections_per_second":2}}`)
	ugrlStore = service.NewUserRateLimitStore(nil)
	origNow := ugrlNow
	t.Cleanup(func() { ugrlNow = origNow })

	r := newRelayRouter([]string{"vip"}, 9, nil)

	base := time.Unix(1800000000, 0)
	now := base
	ugrlNow = func() time.Time { return now }

	for i := 0; i < 2; i++ {
		if w := doOn(r); w.Code != http.StatusOK {
			t.Fatalf("req %d in-window got %d want 200", i+1, w.Code)
		}
	}
	now = base.Add(300 * time.Millisecond)
	w := doOn(r)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("third request same second got %d want 429", w.Code)
	}
	if ra := w.Header().Get("Retry-After"); ra != "1" {
		t.Fatalf("Retry-After = %q want 1", ra)
	}
	errObj := decodeErrorBody(t, w)
	if msg, _ := errObj["message"].(string); !strings.Contains(msg, "速率") {
		t.Fatalf("rate message should mention 速率, got %q", msg)
	}

	now = base.Add(1 * time.Second) // 下一秒恢复
	if w := doOn(r); w.Code != http.StatusOK {
		t.Fatalf("first request next second got %d want 200", w.Code)
	}
}

func TestUserGroupRateLimitMinAcrossGroups(t *testing.T) {
	setSetting(t, `{"free":{"concurrency":2},"vip":{"concurrency":10}}`)
	ugrlStore = service.NewUserRateLimitStore(nil)

	const userId = 11
	gates := [2]*gateHandler{newGate(), newGate()}
	routers := [2]*gin.Engine{
		newRelayRouter([]string{"free", "vip"}, userId, gates[0]),
		newRelayRouter([]string{"free", "vip"}, userId, gates[1]),
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); doOn(routers[0]) }()
	go func() { defer wg.Done(); doOn(routers[1]) }()

	// 第一个到达的属于哪个 router 不影响断言：两个都必须到齐
	waitArrived(t, gates[0].arrived)
	waitArrived(t, gates[1].arrived)

	w3 := doOn(routers[0])
	if w3.Code != http.StatusTooManyRequests {
		t.Fatalf("third concurrent (min limit=2) got %d want 429", w3.Code)
	}

	close(gates[0].release)
	close(gates[1].release)
	wg.Wait()

	if got := ugrlStore.GetConcurrency(strconv.Itoa(userId)); got != 0 {
		t.Fatalf("slots leaked: %d", got)
	}
}

func TestUserGroupRateLimitBurstRespectsLimit(t *testing.T) {
	setSetting(t, `{"vip":{"connections_per_second":5}}`)
	ugrlStore = service.NewUserRateLimitStore(nil)

	r := newRelayRouter([]string{"vip"}, 12, nil)

	const total = 40
	var passed atomic.Int32
	var unexpected atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < total; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := doOn(r)
			switch w.Code {
			case http.StatusOK:
				passed.Add(1)
			case http.StatusTooManyRequests:
			default:
				unexpected.Add(1)
			}
		}()
	}
	wg.Wait()

	if p := passed.Load(); p < 1 || p > 5 {
		t.Fatalf("passed=%d must be within [1,5] for fixed 1s window", p)
	}
	if unexpected.Load() > 0 {
		t.Fatalf("%d requests returned unexpected status", unexpected.Load())
	}
	if got := ugrlStore.GetConcurrency(strconv.Itoa(12)); got != 0 {
		t.Fatalf("slots leaked after burst: %d", got)
	}
}
