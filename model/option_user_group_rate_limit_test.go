package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
)

func TestUserGroupRateLimitOptionWiring(t *testing.T) {
	common.OptionMap = make(map[string]string)

	t.Run("default option registered by InitOptionMap", func(t *testing.T) {
		InitOptionMap()
		if _, exists := common.OptionMap["UserGroupRateLimitSettings"]; !exists {
			t.Fatal("InitOptionMap must register default UserGroupRateLimitSettings")
		}
	})

	t.Run("update applies valid json", func(t *testing.T) {
		if err := updateOptionMap("UserGroupRateLimitSettings", `{"vip":{"concurrency":4}}`); err != nil {
			t.Fatalf("valid update failed: %v", err)
		}
		entry, found := setting.GetUserGroupRateLimit("vip")
		if !found || entry.Concurrency != 4 {
			t.Fatalf("setting not applied: %+v found=%v", entry, found)
		}
	})

	t.Run("update rejects invalid json keeping old value", func(t *testing.T) {
		err := updateOptionMap("UserGroupRateLimitSettings", `{"vip":bad`)
		if err == nil {
			t.Fatal("invalid json must return error")
		}
		entry, _ := setting.GetUserGroupRateLimit("vip")
		if entry.Concurrency != 4 {
			t.Fatalf("old config must survive failed update, got %+v", entry)
		}
	})

	t.Run("update rejects negative values", func(t *testing.T) {
		if err := updateOptionMap("UserGroupRateLimitSettings", `{"a":{"connections_per_second":-1}}`); err == nil {
			t.Fatal("negative value must be rejected")
		}
	})
}
