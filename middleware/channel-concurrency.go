package middleware

import (
	"net/http"
	"strconv"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

var (
	ccOnce  sync.Once
	ccStore service.UserRateLimitStore
)

func getCCStore() service.UserRateLimitStore {
	ccOnce.Do(func() {
		if common.RedisEnabled && common.RDB != nil {
			ccStore = service.NewUserRateLimitStore(common.RDB)
		} else {
			ccStore = service.NewUserRateLimitStore(nil)
		}
	})
	return ccStore
}

const channelConcurrencyKeyPrefix = "chan:"

func channelConcurrencyKey(channelId int) string {
	return channelConcurrencyKeyPrefix + strconv.Itoa(channelId)
}

// acquireChannelConcurrencySlot 渠道在途并发名额先增后判（内存实现持锁、Redis INCR 原子）。
// acquired=false 表示已满需拒绝（计数未被改动）；
// counted=true 表示占用成功，请求结束时必须调用 releaseChannelConcurrencySlot 归还。
// store 故障时降级放行（counted=false），宁可漏限不可错杀。
func acquireChannelConcurrencySlot(store service.UserRateLimitStore, channelId int, limit int64) (acquired bool, counted bool) {
	if limit <= 0 || channelId <= 0 {
		return true, false
	}
	current, err := store.AcquireConcurrency(channelConcurrencyKey(channelId))
	if err != nil {
		common.SysError("channel concurrency acquire failed (degrade to allow): " + err.Error())
		return true, false
	}
	if current > limit {
		store.ReleaseConcurrency(channelConcurrencyKey(channelId))
		return false, false
	}
	return true, true
}

func releaseChannelConcurrencySlot(store service.UserRateLimitStore, channelId int) {
	store.ReleaseConcurrency(channelConcurrencyKey(channelId))
}

func abortChannelConcurrencyReached(c *gin.Context, channelId int, limit int64) {
	message := "当前渠道并发任务数已达上限（" + strconv.FormatInt(limit, 10) + "），请稍后重试或更换其他可用服务"
	c.JSON(http.StatusTooManyRequests, gin.H{
		"error": gin.H{
			"message":           common.MessageWithRequestId(message, c.GetString(common.RequestIdKey)),
			"type":              "channel_concurrency_exceeded",
			"code":              "concurrency_limit_exceeded",
			"channel_id":        channelId,
			"concurrency_limit": limit,
		},
	})
	c.Abort()
}

// handleChannelConcurrency distributor 选路完成后、进入 relay 前调用：
// ok=false 表示该渠道并发已满并已写出 429 并 Abort，调用方应立即返回；
// 否则 release 非 nil 时调用方必须 defer release()（覆盖流式/WS 全生命周期）。
func handleChannelConcurrency(c *gin.Context, store service.UserRateLimitStore, channelId int, limit int64) (ok bool, release func()) {
	acquired, counted := acquireChannelConcurrencySlot(store, channelId, limit)
	if !acquired {
		abortChannelConcurrencyReached(c, channelId, limit)
		return false, nil
	}
	if !counted {
		return true, nil
	}
	return true, func() { releaseChannelConcurrencySlot(store, channelId) }
}
