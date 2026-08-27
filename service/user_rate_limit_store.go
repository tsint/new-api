package service

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
)

// UserRateLimitStore 用户级限流计数：并发占用 + 自然秒窗口计数。
// Redis 可用时为多节点全局精确；否则进程内存近似（单节点准确，多节点为节点本地）。
type UserRateLimitStore interface {
	// AcquireConcurrency 并发数 +1，返回累加后的值
	AcquireConcurrency(key string) (int64, error)
	ReleaseConcurrency(key string)
	GetConcurrency(key string) int64
	// IncSecondRate 在 now 所在自然秒的窗口内 +1，返回窗口内累计值
	IncSecondRate(key string, now time.Time) (int64, error)
}

const userConcKeyPrefix = "ugrl:conc:"
const userRateKeyPrefix = "ugrl:rate:"

func NewUserRateLimitStore(rdb *redis.Client) UserRateLimitStore {
	if rdb != nil {
		return &redisUserRateLimitStore{rdb: rdb}
	}
	return newMemUserRateLimitStore()
}

// ---------------- memory ----------------

type memUserRateLimitStore struct {
	mu      sync.Mutex
	conc    map[string]int64
	rateCur map[string]int64
	curSec  int64
}

func newMemUserRateLimitStore() *memUserRateLimitStore {
	return &memUserRateLimitStore{
		conc:    make(map[string]int64),
		rateCur: make(map[string]int64),
	}
}

func (m *memUserRateLimitStore) AcquireConcurrency(key string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.conc[key]++
	return m.conc[key], nil
}

func (m *memUserRateLimitStore) ReleaseConcurrency(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if v := m.conc[key]; v > 0 {
		m.conc[key] = v - 1
	}
	if m.conc[key] == 0 {
		delete(m.conc, key)
	}
}

func (m *memUserRateLimitStore) GetConcurrency(key string) int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.conc[key]
}

func (m *memUserRateLimitStore) IncSecondRate(key string, now time.Time) (int64, error) {
	sec := now.Unix()
	m.mu.Lock()
	defer m.mu.Unlock()
	if sec != m.curSec {
		m.rateCur = make(map[string]int64)
		m.curSec = sec
	}
	m.rateCur[key]++
	return m.rateCur[key], nil
}

// ---------------- redis ----------------

type redisUserRateLimitStore struct {
	rdb *redis.Client
}

func (r *redisUserRateLimitStore) AcquireConcurrency(key string) (int64, error) {
	ctx := context.Background()
	k := userConcKeyPrefix + key
	v, err := r.rdb.Incr(ctx, k).Result()
	if err != nil {
		return 0, err
	}
	// 防御节点崩溃导致的计数泄漏：1小时兜底过期
	r.rdb.ExpireNX(ctx, k, time.Hour)
	return v, nil
}

func (r *redisUserRateLimitStore) ReleaseConcurrency(key string) {
	r.rdb.Decr(context.Background(), userConcKeyPrefix+key)
}

func (r *redisUserRateLimitStore) GetConcurrency(key string) int64 {
	v, err := r.rdb.Get(context.Background(), userConcKeyPrefix+key).Result()
	if err != nil {
		return 0
	}
	n, _ := strconv.ParseInt(v, 10, 64)
	if n < 0 {
		return 0
	}
	return n
}

func (r *redisUserRateLimitStore) IncSecondRate(key string, now time.Time) (int64, error) {
	ctx := context.Background()
	k := fmt.Sprintf("%s%s:%d", userRateKeyPrefix, key, now.Unix())
	v, err := r.rdb.Incr(ctx, k).Result()
	if err != nil {
		return 0, err
	}
	r.rdb.ExpireNX(ctx, k, 2*time.Second)
	return v, nil
}
