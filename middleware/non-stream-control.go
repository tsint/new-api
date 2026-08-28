package middleware

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

// CheckChannelNonStreamSupport F4：渠道级禁用非流请求检查。
// 在 relay 重试循环内、渠道选定后调用；返回 nil 表示放行。
// 流式请求与 WS realtime 豁免；setting 缺失/损坏时放行（fail-open，与限流降级策略一致）。
func CheckChannelNonStreamSupport(c *gin.Context, relayFormat types.RelayFormat, isStream bool) *types.NewAPIError {
	if isStream {
		return nil
	}
	if relayFormat == types.RelayFormatOpenAIRealtime {
		return nil
	}
	channelSetting, ok := common.GetContextKeyType[dto.ChannelSettings](c, constant.ContextKeyChannelSetting)
	if !ok || !channelSetting.DisableNonStreaming {
		return nil
	}
	return types.NewErrorWithStatusCode(
		errors.New("当前渠道已禁用非流式请求，请改用流式请求(stream=true)或更换其他渠道/服务"),
		types.ErrorCodeNonStreamingDisabled,
		http.StatusForbidden,
		types.ErrOptionWithSkipRetry(),
	)
}

var (
	nsrlOnce sync.Once
	nsrlStore service.UserRateLimitStore
)

func getNSRLStore() service.UserRateLimitStore {
	nsrlOnce.Do(func() {
		if common.RedisEnabled && common.RDB != nil {
			nsrlStore = service.NewUserRateLimitStore(common.RDB)
		} else {
			nsrlStore = service.NewUserRateLimitStore(nil)
		}
	})
	return nsrlStore
}

const nonStreamRateLimitKeyPrefix = "nsrl:"

func nonStreamRateLimitKey(userId int) string {
	return nonStreamRateLimitKeyPrefix + strconv.Itoa(userId)
}

// CheckNonStreamRateLimitForRelay controller 调用入口（注入全局 store）
func CheckNonStreamRateLimitForRelay(c *gin.Context, relayFormat types.RelayFormat, isStream bool) *types.NewAPIError {
	return CheckNonStreamRateLimit(c, relayFormat, isStream, getNSRLStore())
}

// CheckNonStreamRateLimit F5：非流请求每用户每分钟限速检查。
// 在 GenRelayInfo 之后、预扣费之前调用（入口计数，先增后判）；
// 流式请求与 WS realtime 豁免；store 故障降级放行。
func CheckNonStreamRateLimit(c *gin.Context, relayFormat types.RelayFormat, isStream bool, store service.UserRateLimitStore) *types.NewAPIError {
	if !setting.NonStreamRequestRateLimitEnabled || isStream || relayFormat == types.RelayFormatOpenAIRealtime {
		return nil
	}

	limit := setting.NonStreamRequestRateLimitCount
	group := common.GetContextKeyString(c, constant.ContextKeyTokenGroup)
	if group == "" {
		group = common.GetContextKeyString(c, constant.ContextKeyUserGroup)
	}
	if groupLimit, found := setting.GetNonStreamRateLimit(group); found {
		limit = groupLimit
	}
	if limit <= 0 {
		return nil
	}

	used, err := store.IncMinuteRate(nonStreamRateLimitKey(c.GetInt("id")), time.Now())
	if err != nil {
		common.SysError("non-stream rate limit store failure (degrade to allow): " + err.Error())
		return nil
	}
	if used > int64(limit) {
		return types.NewErrorWithStatusCode(
			fmt.Errorf("您每分钟最多发起 %d 次非流式请求，请稍后重试或改用流式请求(stream=true)", limit),
			types.ErrorCodeNonStreamRateLimitExceed,
			http.StatusTooManyRequests,
			types.ErrOptionWithSkipRetry(),
		)
	}
	return nil
}
