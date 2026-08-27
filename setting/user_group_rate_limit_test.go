package setting

import (
	"strings"
	"testing"
)

func TestUpdateUserGroupRateLimitSettingsValidJSON(t *testing.T) {
	valid := `{"vip":{"concurrency":10,"connections_per_second":5},"default":{"concurrency":3}}`
	if err := UpdateUserGroupRateLimitSettingsByJSONString(valid); err != nil {
		t.Fatalf("expected valid json accepted, got error: %v", err)
	}
	got, found := GetUserGroupRateLimit("vip")
	if !found {
		t.Fatal("expected vip entry present")
	}
	if got.Concurrency != 10 || got.ConnectionsPerSecond != 5 {
		t.Fatalf("vip entry mismatch: %+v", got)
	}
	def, _ := GetUserGroupRateLimit("default")
	if def.Concurrency != 3 || def.ConnectionsPerSecond != 0 {
		t.Fatalf("default entry mismatch (cps must default to unlimited=0): %+v", def)
	}
}

func TestUpdateUserGroupRateLimitSettingsRoundtrip(t *testing.T) {
	src := `{"vip":{"concurrency":7}}`
	if err := UpdateUserGroupRateLimitSettingsByJSONString(src); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out, err := UserGroupRateLimitSettings2JSONString()
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	if err := UpdateUserGroupRateLimitSettingsByJSONString(out); err != nil {
		t.Fatalf("roundtrip output not re-parseable: %v, out=%s", err, out)
	}
	if _, found := GetUserGroupRateLimit("vip"); !found {
		t.Fatal("vip entry lost after roundtrip")
	}
}

func TestUpdateUserGroupRateLimitSettingsRejects(t *testing.T) {
	cases := []struct {
		name   string
		json   string
		errMsg string
	}{
		{"invalid json", `{vip`, ""},
		{"negative concurrency", `{"a":{"concurrency":-1}}`, "negative"},
		{"negative cps", `{"a":{"connections_per_second":-2}}`, "negative"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := UpdateUserGroupRateLimitSettingsByJSONString(tc.json)
			if err == nil {
				t.Fatalf("expected error for %s", tc.json)
			}
			if tc.errMsg != "" && !strings.Contains(err.Error(), tc.errMsg) {
				t.Fatalf("error %q should mention %q", err.Error(), tc.errMsg)
			}
		})
	}
}

func TestResolveEffectiveLimits(t *testing.T) {
	tests := []struct {
		name          string
		settings      string
		groups        []string
		wantConc      int
		wantCps       int
	}{
		{name: "no groups configured means unlimited", settings: `{}`, groups: []string{"default"}, wantConc: 0, wantCps: 0},
		{name: "empty user groups means unlimited", settings: `{"a":{"concurrency":5}}`, groups: nil, wantConc: 0, wantCps: 0},
		{
			name:     "min across multiple groups",
			settings: `{"free":{"concurrency":2,"connections_per_second":1},"vip":{"concurrency":10,"connections_per_second":8}}`,
			groups:   []string{"free", "vip"},
			wantConc: 2, wantCps: 1,
		},
		{
			name:     "missing item in one group falls back to other group's value",
			settings: `{"free":{"concurrency":2},"vip":{"concurrency":10,"connections_per_second":4}}`,
			groups:   []string{"free", "vip"},
			wantConc: 2, wantCps: 4,
		},
		{
			name:     "zero entries are ignored as unlimited",
			settings: `{"a":{"concurrency":0},"b":{"concurrency":6}}`,
			groups:   []string{"a", "b"},
			wantConc: 6, wantCps: 0,
		},
		{
			name:     "all values zero means unlimited",
			settings: `{"a":{"concurrency":0}}`,
			groups:   []string{"a"},
			wantConc: 0, wantCps: 0,
		},
		{
			name:     "unknown group names are skipped",
			settings: `{"x":{"concurrency":9}}`,
			groups:   []string{"y", "z"},
			wantConc: 0, wantCps: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := UpdateUserGroupRateLimitSettingsByJSONString(tt.settings); err != nil {
				t.Fatalf("setup failed: %v", err)
			}
			conc, cps := ResolveEffectiveUserGroupLimits(tt.groups)
			if conc != tt.wantConc || cps != tt.wantCps {
				t.Fatalf("got (conc=%d,cps=%d), want (%d,%d)", conc, cps, tt.wantConc, tt.wantCps)
			}
		})
	}
}
