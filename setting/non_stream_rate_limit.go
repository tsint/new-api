package setting

import (
	"fmt"
	"math"
	"sync"

	"github.com/QuantumNous/new-api/common"
)

// F5：非流请求每用户每分钟限速配置
var NonStreamRequestRateLimitEnabled = false
var NonStreamRequestRateLimitCount = 0         // 每用户每分钟非流请求上限，0=不限制
var NonStreamRateLimitGroup = map[string]int{} // 组覆盖；组条目 0=该组不限制
var NonStreamRateLimitMutex sync.RWMutex

func NonStreamRateLimitGroup2JSONString() string {
	NonStreamRateLimitMutex.RLock()
	defer NonStreamRateLimitMutex.RUnlock()

	jsonBytes, err := common.Marshal(NonStreamRateLimitGroup)
	if err != nil {
		common.SysLog("error marshalling non-stream rate limit group: " + err.Error())
	}
	return string(jsonBytes)
}

func UpdateNonStreamRateLimitGroupByJSONString(jsonStr string) error {
	NonStreamRateLimitMutex.Lock()
	defer NonStreamRateLimitMutex.Unlock()

	NonStreamRateLimitGroup = make(map[string]int)
	if jsonStr == "" {
		return nil
	}
	return common.Unmarshal([]byte(jsonStr), &NonStreamRateLimitGroup)
}

// GetNonStreamRateLimit found=true 时 count 覆盖全局默认（count 可为 0，表示该组不限制）
func GetNonStreamRateLimit(group string) (count int, found bool) {
	NonStreamRateLimitMutex.RLock()
	defer NonStreamRateLimitMutex.RUnlock()

	if NonStreamRateLimitGroup == nil {
		return 0, false
	}
	count, found = NonStreamRateLimitGroup[group]
	return count, found
}

func CheckNonStreamRateLimitGroup(jsonStr string) error {
	checkGroup := make(map[string]int)
	if jsonStr == "" {
		return nil
	}
	if err := common.Unmarshal([]byte(jsonStr), &checkGroup); err != nil {
		return err
	}
	for group, count := range checkGroup {
		if count < 0 {
			return fmt.Errorf("group %s has negative rate limit value: %d", group, count)
		}
		if count > math.MaxInt32 {
			return fmt.Errorf("group %s has max rate limit value 2147483647", group)
		}
	}
	return nil
}
