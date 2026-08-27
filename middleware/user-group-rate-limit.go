package middleware

import (
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

// 包级可替换依赖，便于测试注入
var (
	ugrlOnce  sync.Once
	ugrlStore service.UserRateLimitStore
	ugrlNow   = time.Now
)

func getUgrlStore() service.UserRateLimitStore {
	ugrlOnce.Do(func() {
		if common.RedisEnabled && common.RDB != nil {
			ugrlStore = service.NewUserRateLimitStore(common.RDB)
		} else {
			ugrlStore = service.NewUserRateLimitStore(nil)
		}
	})
	return ugrlStore
}

func userRateLimitKey(userId int) string {
	return strconv.Itoa(userId)
}

// UserGroupRateLimit 用户组并发与新建连接速率限制。
// 按用户计量，有效限额取所属各组配置的最小正值；默认不限制。
// 超限立即返回 429，不触发上游请求。
func UserGroupRateLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		groups := service.UserGroupListFromCtx(c)
		concLimit, cpsLimit := setting.ResolveEffectiveUserGroupLimits(groups)

		if concLimit <= 0 && cpsLimit <= 0 {
			c.Next()
			return
		}

		store := getUgrlStore()
		key := userRateLimitKey(common.GetContextKeyInt(c, constant.ContextKeyUserId))
		now := ugrlNow()

		if cpsLimit > 0 {
			count, err := store.IncSecondRate(key, now)
			if err != nil {
				common.SysError("user group rate limit check failed (degrade to allow): " + err.Error())
			} else if count > int64(cpsLimit) {
				c.Header("Retry-After", "1")
				abortWithOpenAiMessage(c, http.StatusTooManyRequests,
					fmt.Sprintf("新建连接速率达上限（每秒最多 %d 次），下一秒将自动恢复", cpsLimit),
					types.ErrorCode("rate_limit_exceeded"))
				return
			}
		}

		if concLimit > 0 {
			current, err := store.AcquireConcurrency(key)
			if err != nil {
				common.SysError("user group concurrency acquire failed (degrade to allow): " + err.Error())
			} else {
				if current > int64(concLimit) {
					store.ReleaseConcurrency(key)
					abortWithOpenAiMessage(c, http.StatusTooManyRequests,
						fmt.Sprintf("您的并发请求数已达上限（%d），请等待进行中的请求完成后再试", concLimit),
						types.ErrorCode("rate_limit_exceeded"))
					return
				}
				defer store.ReleaseConcurrency(key)
			}
		}

		c.Next()
	}
}
