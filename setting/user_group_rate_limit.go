package setting

import (
	"fmt"
	"sync"

	"github.com/QuantumNous/new-api/common"
)

// UserGroupRateLimitEntry 用户组并发与新建连接速率限制配置。
// 字段值 <=0 表示该项对该组不限制。
type UserGroupRateLimitEntry struct {
	Concurrency          int `json:"concurrency,omitempty"`
	ConnectionsPerSecond int `json:"connections_per_second,omitempty"`
}

var (
	UserGroupRateLimitSettings = map[string]UserGroupRateLimitEntry{}
	userGroupRateLimitMutex    sync.RWMutex
)

func validateUserGroupRateLimitSettings(settings map[string]UserGroupRateLimitEntry) error {
	for group, entry := range settings {
		if entry.Concurrency < 0 || entry.ConnectionsPerSecond < 0 {
			return fmt.Errorf("group %q has negative rate limit values: %+v", group, entry)
		}
	}
	return nil
}

// UserGroupRateLimitSettings2JSONString 序列化当前配置。
func UserGroupRateLimitSettings2JSONString() (string, error) {
	userGroupRateLimitMutex.RLock()
	defer userGroupRateLimitMutex.RUnlock()

	jsonBytes, err := common.Marshal(UserGroupRateLimitSettings)
	if err != nil {
		return "", err
	}
	return string(jsonBytes), nil
}

// UpdateUserGroupRateLimitSettingsByJSONString 校验并整体替换配置。
func UpdateUserGroupRateLimitSettingsByJSONString(jsonStr string) error {
	settings := make(map[string]UserGroupRateLimitEntry)
	if err := common.Unmarshal([]byte(jsonStr), &settings); err != nil {
		return err
	}
	if err := validateUserGroupRateLimitSettings(settings); err != nil {
		return err
	}

	userGroupRateLimitMutex.Lock()
	defer userGroupRateLimitMutex.Unlock()
	UserGroupRateLimitSettings = settings
	return nil
}

// GetUserGroupRateLimit 返回指定组的配置；未配置返回 found=false。
func GetUserGroupRateLimit(group string) (UserGroupRateLimitEntry, bool) {
	userGroupRateLimitMutex.RLock()
	defer userGroupRateLimitMutex.RUnlock()

	entry, found := UserGroupRateLimitSettings[group]
	return entry, found
}

// ResolveEffectiveUserGroupLimits 计算用户的有效限额：
// 对每一项取所属各组中配置值>0 的最小值；全部无正值则返回 0（不限制）。
func ResolveEffectiveUserGroupLimits(groups []string) (concurrency int, connectionsPerSecond int) {
	userGroupRateLimitMutex.RLock()
	defer userGroupRateLimitMutex.RUnlock()

	minPositive := func(current, candidate int) int {
		if candidate <= 0 {
			return current
		}
		if current == 0 || candidate < current {
			return candidate
		}
		return current
	}

	for _, group := range groups {
		entry, found := UserGroupRateLimitSettings[group]
		if !found {
			continue
		}
		concurrency = minPositive(concurrency, entry.Concurrency)
		connectionsPerSecond = minPositive(connectionsPerSecond, entry.ConnectionsPerSecond)
	}
	return concurrency, connectionsPerSecond
}
