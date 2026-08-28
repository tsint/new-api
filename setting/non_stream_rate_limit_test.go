package setting

import (
	"math"
	"strings"
	"testing"
)

// ---- F5: 非流请求每分钟限速配置 ----

func TestCheckNonStreamRateLimitGroup(t *testing.T) {
	tests := []struct {
		name    string
		jsonStr string
		wantErr bool
	}{
		{"valid with zero-unlimited entry", `{"vip":5,"svip":0}`, false},
		{"empty object", `{}`, false},
		{"empty string parses to nil map", ``, false},
		{"negative rejected", `{"vip":-1}`, true},
		{"overflow rejected", `{"vip":2147483648}`, true},
		{"max int32 allowed", `{"vip":2147483647}`, false},
		{"bad json rejected", `{vip:5}`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckNonStreamRateLimitGroup(tt.jsonStr)
			if tt.wantErr && err == nil {
				t.Fatalf("expected error for %q", tt.jsonStr)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error for %q: %v", tt.jsonStr, err)
			}
		})
	}
}

func TestUpdateAndGetNonStreamRateLimit(t *testing.T) {
	t.Cleanup(func() {
		_ = UpdateNonStreamRateLimitGroupByJSONString(`{}`)
	})

	if err := UpdateNonStreamRateLimitGroupByJSONString(`{"vip":5,"svip":0}`); err != nil {
		t.Fatalf("update failed: %v", err)
	}

	if count, found := GetNonStreamRateLimit("vip"); !found || count != 5 {
		t.Fatalf("vip = (%d,%v), want (5,true)", count, found)
	}
	// 组条目 0 => 该组不限制（found 仍为 true 以覆盖全局默认）
	if count, found := GetNonStreamRateLimit("svip"); !found || count != 0 {
		t.Fatalf("svip = (%d,%v), want (0,true)", count, found)
	}
	if _, found := GetNonStreamRateLimit("default"); found {
		t.Fatal("missing group must not be found (global default applies)")
	}

	// Roundtrip: JSON 序列化后再读回一致
	jsonStr := NonStreamRateLimitGroup2JSONString()
	if !strings.Contains(jsonStr, `"vip":5`) {
		t.Fatalf("roundtrip json missing vip entry: %s", jsonStr)
	}
	if err := UpdateNonStreamRateLimitGroupByJSONString(jsonStr); err != nil {
		t.Fatalf("roundtrip update failed: %v", err)
	}
	if count, found := GetNonStreamRateLimit("vip"); !found || count != 5 {
		t.Fatalf("after roundtrip vip = (%d,%v), want (5,true)", count, found)
	}

	if math.MaxInt32 <= 0 {
		t.Fatal("sanity")
	}
}
