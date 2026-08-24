package service

import (
	"testing"

	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
)

func withSpecialUsableGroups(t *testing.T, special map[string]map[string]string) {
	t.Helper()
	old := ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup
	restore := types.NewRWMap[string, map[string]string]()
	restore.AddAll(old.ReadAll())
	t.Cleanup(func() { ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup = restore })
	newMap := types.NewRWMap[string, map[string]string]()
	newMap.AddAll(special)
	ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup = newMap
}

func TestGetUserUsableGroupsByGroups(t *testing.T) {
	t.Run("union of special configs across groups", func(t *testing.T) {
		withSpecialUsableGroups(t, map[string]map[string]string{
			"ga": {"+:ga_extra": "GA extra"},
			"gb": {"+:gb_extra": "GB extra"},
		})
		got := GetUserUsableGroupsByGroups([]string{"ga", "gb"})
		for _, want := range []string{"ga", "gb", "ga_extra", "gb_extra"} {
			if _, ok := got[want]; !ok {
				t.Errorf("union missing %q, got %v", want, got)
			}
		}
	})
	t.Run("removal from any bound group wins", func(t *testing.T) {
		withSpecialUsableGroups(t, map[string]map[string]string{
			"ga": {"-:gb_extra": ""},
			"gb": {"+:gb_extra": "GB extra"},
		})
		got := GetUserUsableGroupsByGroups([]string{"ga", "gb"})
		if _, ok := got["gb_extra"]; ok {
			t.Errorf("gb_extra should be removed, got %v", got)
		}
	})
	t.Run("bound group always included", func(t *testing.T) {
		withSpecialUsableGroups(t, nil)
		got := GetUserUsableGroupsByGroups([]string{"ga"})
		if _, ok := got["ga"]; !ok {
			t.Errorf("bound group ga missing, got %v", got)
		}
	})
	t.Run("single group equals legacy behavior", func(t *testing.T) {
		withSpecialUsableGroups(t, map[string]map[string]string{
			"vip": {"+:vip_extra": "VIP extra"},
		})
		legacy := GetUserUsableGroups("vip")
		byGroups := GetUserUsableGroupsByGroups([]string{"vip"})
		if len(legacy) != len(byGroups) {
			t.Errorf("legacy %v != byGroups %v", legacy, byGroups)
		}
		for k := range legacy {
			if _, ok := byGroups[k]; !ok {
				t.Errorf("byGroups missing %q", k)
			}
		}
	})
}

func TestGetUserAutoGroupByGroups(t *testing.T) {
	// setting.GetAutoGroups() 默认为 ["default"]；默认 userUsableGroups 含 default
	t.Run("intersects with usable union", func(t *testing.T) {
		withSpecialUsableGroups(t, nil)
		got := GetUserAutoGroupByGroups([]string{"ga"})
		if len(got) != 1 || got[0] != "default" {
			t.Errorf("got %v, want [default]", got)
		}
	})
	t.Run("excludes unusable auto groups", func(t *testing.T) {
		withSpecialUsableGroups(t, map[string]map[string]string{
			"ga": {"-:default": ""},
		})
		got := GetUserAutoGroupByGroups([]string{"ga"})
		if len(got) != 0 {
			t.Errorf("got %v, want empty", got)
		}
	})
}

func TestGroupInUserUsableGroupsByGroups(t *testing.T) {
	withSpecialUsableGroups(t, map[string]map[string]string{
		"ga": {"+:shared": "shared"},
	})
	if !GroupInUserUsableGroupsByGroups([]string{"ga", "gb"}, "shared") {
		t.Error("shared should be usable via ga")
	}
	if GroupInUserUsableGroupsByGroups([]string{"gb"}, "shared") {
		t.Error("shared should not be usable with only gb")
	}
}
