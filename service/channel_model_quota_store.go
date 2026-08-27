package service

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
)

// ChannelQuotaStore 渠道×组×模型 在指定分块内的 token 消耗计数。
// Redis 可用时全局精确；否则进程内存近似（重启清零）。
type ChannelQuotaStore interface {
	GetUsed(channelId int, group, model string, blockIdx int64) int64
	Add(channelId int, group, model string, blockIdx int64, delta int64)
}

const channelQuotaKeyPrefix = "cmq:"

func channelQuotaRedisKey(channelId int, group, model string, blockIdx int64) string {
	return fmt.Sprintf("%s%d:%s:%s:%d", channelQuotaKeyPrefix, channelId, group, model, blockIdx)
}

func NewChannelQuotaStore(rdb *redis.Client) ChannelQuotaStore {
	if rdb != nil {
		return &redisChannelQuotaStore{rdb: rdb}
	}
	return newMemChannelQuotaStore()
}

// ---------------- memory ----------------

type memChannelQuotaStore struct {
	mu    sync.Mutex
	count map[string]int64
}

func newMemChannelQuotaStore() *memChannelQuotaStore {
	return &memChannelQuotaStore{count: make(map[string]int64)}
}

func memChannelQuotaKey(channelId int, group, model string, blockIdx int64) string {
	return strconv.Itoa(channelId) + ":" + group + ":" + model + ":" + strconv.FormatInt(blockIdx, 10)
}

func (m *memChannelQuotaStore) GetUsed(channelId int, group, model string, blockIdx int64) int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.count[memChannelQuotaKey(channelId, group, model, blockIdx)]
}

func (m *memChannelQuotaStore) Add(channelId int, group, model string, blockIdx int64, delta int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.count[memChannelQuotaKey(channelId, group, model, blockIdx)] += delta
}

// ---------------- redis ----------------

type redisChannelQuotaStore struct {
	rdb *redis.Client
}

func (r *redisChannelQuotaStore) GetUsed(channelId int, group, model string, blockIdx int64) int64 {
	v, err := r.rdb.Get(context.Background(), channelQuotaRedisKey(channelId, group, model, blockIdx)).Int64()
	if err != nil {
		return 0
	}
	return v
}

func (r *redisChannelQuotaStore) Add(channelId int, group, model string, blockIdx int64, delta int64) {
	ctx := context.Background()
	key := channelQuotaRedisKey(channelId, group, model, blockIdx)
	r.rdb.IncrBy(ctx, key, delta)
	// 窗口已结束的历史块无需保留；当前块的兜底过期 5h > 4h 窗口
	if time.Now().Unix() < (blockIdx+1)*ChannelModelQuotaBlockSeconds {
		r.rdb.ExpireNX(ctx, key, 16200*time.Second)
	} else {
		r.rdb.Expire(ctx, key, time.Minute)
	}
}
