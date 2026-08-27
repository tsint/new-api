package service

import (
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/gin-gonic/gin"
)

var (
	cqOnce  sync.Once
	cqStore ChannelQuotaStore
)

// GetChannelQuotaStore 返回全局渠道配额计数器（Redis 可用则精确，否则内存）
func GetChannelQuotaStore() ChannelQuotaStore {
	cqOnce.Do(func() {
		if common.RedisEnabled && common.RDB != nil {
			cqStore = NewChannelQuotaStore(common.RDB)
		} else {
			cqStore = NewChannelQuotaStore(nil)
		}
	})
	return cqStore
}

// ChannelQuotaExceededInfo 触发渠道模型时段限额的详情
type ChannelQuotaExceededInfo struct {
	Limit   int64 `json:"quota_limit"`
	Used    int64 `json:"quota_used"`
	ResetAt int64 `json:"quota_reset_at"` // unix 秒
}

// CheckChannelModelQuota 入方向限额检查：
// 匹配到正值限额且当前块用量已达上限时返回详情，否则返回 nil。
func CheckChannelModelQuota(channelId int, rawSettings, group, model string, now time.Time) *ChannelQuotaExceededInfo {
	limit := MatchChannelModelQuotaLimitFromRaw(rawSettings, group, model)
	if limit <= 0 {
		return nil
	}
	idx := QuotaBlockIndex(now)
	used := GetChannelQuotaStore().GetUsed(channelId, group, model, idx)
	if used < limit {
		return nil
	}
	return &ChannelQuotaExceededInfo{
		Limit:   limit,
		Used:    used,
		ResetAt: NextQuotaBlockStart(now),
	}
}

// ResolveRequestGroupFromCtx 解析请求实际生效分组：
// 选路产生的 AutoGroup 优先于 UsingGroup。
func ResolveRequestGroupFromCtx(c *gin.Context) string {
	if g := common.GetContextKeyString(c, constant.ContextKeyAutoGroup); g != "" {
		return g
	}
	return common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
}

// RecordChannelQuotaUsage 出方向记账：prompt+completion 累加到指定分块计数器
func RecordChannelQuotaUsage(store ChannelQuotaStore, channelId int, group, model string, blockIdx int64, promptTokens, completionTokens int64) {
	store.Add(channelId, group, model, blockIdx, promptTokens+completionTokens)
}

// recordChannelModelQuotaFromCtx 在消费出口按 渠道×组×模型 记录 token 消耗
func recordChannelModelQuotaFromCtx(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, usage *dto.Usage) {
	if usage == nil || relayInfo == nil || relayInfo.ChannelId == 0 {
		return
	}
	group := ResolveRequestGroupFromCtx(ctx)
	if group == "" {
		return
	}
	modelName := relayInfo.OriginModelName
	if modelName == "" {
		modelName = relayInfo.UpstreamModelName
	}
	if modelName == "" {
		return
	}
	store := GetChannelQuotaStore()
	RecordChannelQuotaUsage(store, relayInfo.ChannelId, group, modelName, QuotaBlockIndex(time.Now()),
		int64(usage.PromptTokens), int64(usage.CompletionTokens))
}
