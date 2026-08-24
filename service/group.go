package service

import (
	"strings"

	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

func GetUserUsableGroupsByGroups(userGroups []string) map[string]string {
	groupsCopy := setting.GetUserUsableGroupsCopy()
	if len(userGroups) == 0 {
		return groupsCopy
	}
	groupsToAdd := make(map[string]string)
	groupsToRemove := make(map[string]bool)
	special := ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup
	for _, userGroup := range userGroups {
		specialSettings, ok := special.Get(userGroup)
		if !ok {
			continue
		}
		for specialGroup, desc := range specialSettings {
			if strings.HasPrefix(specialGroup, "-:") {
				groupsToRemove[strings.TrimPrefix(specialGroup, "-:")] = true
			} else if strings.HasPrefix(specialGroup, "+:") {
				groupsToAdd[strings.TrimPrefix(specialGroup, "+:")] = desc
			} else {
				groupsToAdd[specialGroup] = desc
			}
		}
	}
	for g, desc := range groupsToAdd {
		groupsCopy[g] = desc
	}
	for g := range groupsToRemove {
		delete(groupsCopy, g)
	}
	for _, userGroup := range userGroups {
		if userGroup == "" {
			continue
		}
		if _, ok := groupsCopy[userGroup]; !ok {
			groupsCopy[userGroup] = "用户分组"
		}
	}
	return groupsCopy
}

func GetUserUsableGroups(userGroup string) map[string]string {
	return GetUserUsableGroupsByGroups([]string{userGroup})
}

func GroupInUserUsableGroups(userGroup, groupName string) bool {
	return GroupInUserUsableGroupsByGroups([]string{userGroup}, groupName)
}

func GroupInUserUsableGroupsByGroups(userGroups []string, groupName string) bool {
	_, ok := GetUserUsableGroupsByGroups(userGroups)[groupName]
	return ok
}

func GetUserAutoGroupByGroups(userGroups []string) []string {
	groups := GetUserUsableGroupsByGroups(userGroups)
	autoGroups := make([]string, 0)
	for _, group := range setting.GetAutoGroups() {
		if _, ok := groups[group]; ok {
			autoGroups = append(autoGroups, group)
		}
	}
	return autoGroups
}

func GetUserAutoGroup(userGroup string) []string {
	return GetUserAutoGroupByGroups([]string{userGroup})
}

// GetUserGroupRatio 获取用户使用某个分组的倍率
// userGroup 用户分组
// group 需要获取倍率的分组
func GetUserGroupRatio(userGroup, group string) float64 {
	ratio, ok := ratio_setting.GetGroupGroupRatio(userGroup, group)
	if ok {
		return ratio
	}
	return ratio_setting.GetGroupRatio(group)
}
