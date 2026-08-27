package service

import (
	"strings"
	"testing"
	"time"
)

const sampleQuotaSettings = `{
  "default": { "gpt-4o": 5000000, "*": 1000000 },
  "svip":    { "*": -1 }
}`

func mustParse(t *testing.T, s string) ChannelModelQuotaConfig {
	t.Helper()
	cfg, err := ParseChannelModelQuotaSettings(s)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	return cfg
}

func TestParseChannelModelQuotaSettings(t *testing.T) {
	cfg := mustParse(t, sampleQuotaSettings)
	if got := cfg["default"]["gpt-4o"]; got != 5000000 {
		t.Fatalf("default/gpt-4o = %d want 5000000", got)
	}
	if _, err := ParseChannelModelQuotaSettings(`{bad`); err == nil {
		t.Fatal("invalid json must error")
	}
	if _, err := ParseChannelModelQuotaSettings(""); err != nil {
		t.Fatalf("empty string treated as empty config, got %v", err)
	}
}

func TestMatchChannelModelQuotaLimit(t *testing.T) {
	tests := []struct {
		name     string
		settings string
		group    string
		model    string
		want     int64
	}{
		{"exact model wins", sampleQuotaSettings, "default", "gpt-4o", 5000000},
		{"wildcard fallback within group", sampleQuotaSettings, "default", "claude-3", 1000000},
		{"unconfigured group no limit", sampleQuotaSettings, "other", "gpt-4o", 0},
		{"negative means explicit unlimited", sampleQuotaSettings, "svip", "anything", 0},
		{"exact -1 pins unlimited over wildcard", `{"a":{"m":-1,"*":9}}`, "a", "m", 0},
		{"zero value means unlimited", `{"a":{"m":0}}`, "a", "m", 0},
		{"no group sub-map no limit", `{}`, "x", "y", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cfg ChannelModelQuotaConfig
			if strings.TrimSpace(tt.settings) != "" {
				if tt.settings == sampleQuotaSettings {
					cfg = mustParse(t, tt.settings)
				} else {
					var err error
					cfg, err = ParseChannelModelQuotaSettings(tt.settings)
					if err != nil {
						t.Fatalf("setup: %v", err)
					}
				}
			}
			got := MatchChannelModelQuotaLimit(cfg, tt.group, tt.model)
			if got != tt.want {
				t.Fatalf("limit(%s,%s) = %d want %d", tt.group, tt.model, got, tt.want)
			}
		})
	}
}

func TestMatchFromRawJsonString(t *testing.T) {
	got := MatchChannelModelQuotaLimitFromRaw(sampleQuotaSettings, "svip", "gpt")
	if got != 0 {
		t.Fatalf("raw svip gpt = %d want 0(unlimited)", got)
	}
	if got := MatchChannelModelQuotaLimitFromRaw(`broken`, "a", "b"); got != 0 {
		t.Fatalf("broken json must degrade to no limit, got %d", got)
	}
}

func TestQuotaBlockIndexAndNextReset(t *testing.T) {
	blockSecs := int64(14400)

	cases := []struct {
		ts      int64
		wantIdx int64
	}{
		{0, 0},
		{blockSecs - 1, 0},
		{blockSecs, 1},
		{1700000000, 1700000000 / blockSecs}, // 任意时刻
	}
	for _, tc := range cases {
		now := time.Unix(tc.ts, 0)
		if got := QuotaBlockIndex(now); got != tc.wantIdx {
			t.Fatalf("QuotaBlockIndex(%d)=%d want %d", tc.ts, got, tc.wantIdx)
		}
		next := NextQuotaBlockStart(now)
		if next <= tc.ts || (next%blockSecs) != 0 {
			t.Fatalf("NextQuotaBlockStart(%d)=%d invalid", tc.ts, next)
		}
	}
	// 整块边界时重置点=下一块
	if got := NextQuotaBlockStart(time.Unix(blockSecs, 0)); got != 2*blockSecs {
		t.Fatalf("boundary reset point = %d want %d", got, 2*blockSecs)
	}
}

func TestMemChannelQuotaStoreAddGet(t *testing.T) {
	s := NewChannelQuotaStore(nil)
	now := time.Unix(1800000000, 0)
	idx := QuotaBlockIndex(now)

	s.Add(7, "default", "gpt-4o", idx, 100)
	s.Add(7, "default", "gpt-4o", idx, 23)
	if got := s.GetUsed(7, "default", "gpt-4o", idx); got != 123 {
		t.Fatalf("used = %d want 123", got)
	}
	// 不同维度互不影响
	if got := s.GetUsed(8, "default", "gpt-4o", idx); got != 0 {
		t.Fatalf("other channel leaked: %d", got)
	}
	if got := s.GetUsed(7, "vip", "gpt-4o", idx); got != 0 {
		t.Fatalf("other group leaked: %d", got)
	}
	if got := s.GetUsed(7, "default", "claude", idx); got != 0 {
		t.Fatalf("other model leaked: %d", got)
	}
	// 跨块隔离
	next := QuotaBlockIndex(now.Add(time.Duration(14400) * time.Second))
	if got := s.GetUsed(7, "default", "gpt-4o", next); got != 0 {
		t.Fatalf("next block should start at 0, got %d", got)
	}
	// 过量释放语义不适用；Add 支持大数
	s.Add(7, "default", "gpt-4o", idx, 1<<40)
	if got := s.GetUsed(7, "default", "gpt-4o", idx); got != 123+1<<40 {
		t.Fatalf("big add lost: %d", got)
	}
}

func TestCheckChannelModelQuota(t *testing.T) {
	origStore := cqStore
	cqStore = NewChannelQuotaStore(nil)
	t.Cleanup(func() { cqStore = origStore })

	now := time.Unix(1800000000, 0) // 块内某时刻
	settings := `{"default":{"gpt-4o":1000}}`
	const ch = 42

	type args struct {
		group, model string
	}
	cases := []struct {
		name        string
		preUsed     int64
		args        args
		wantBlocked bool
		wantLimit   int64
	}{
		{"under limit passes", 999, args{"default", "gpt-4o"}, false, 0},
		{"at limit blocked", 1000, args{"default", "gpt-4o"}, true, 1000},
		{"over limit blocked", 1500, args{"default", "gpt-4o"}, true, 1000},
		{"other model no limit", 99999, args{"default", "claude"}, false, 0},
		{"other group no limit", 99999, args{"vip", "gpt-4o"}, false, 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.preUsed > 0 {
				cqStore.Add(ch, tc.args.group, tc.args.model, QuotaBlockIndex(now), tc.preUsed)
			}
			info := CheckChannelModelQuota(ch, settings, tc.args.group, tc.args.model, now)
			if tc.wantBlocked {
				if info == nil {
					t.Fatal("expected block info, got nil")
				}
				if info.Limit != tc.wantLimit {
					t.Fatalf("limit=%d want %d", info.Limit, tc.wantLimit)
				}
				if info.Used < tc.preUsed {
					t.Fatalf("used=%d want >=%d", info.Used, tc.preUsed)
				}
				if info.ResetAt != NextQuotaBlockStart(now) {
					t.Fatalf("resetAt=%d want %d", info.ResetAt, NextQuotaBlockStart(now))
				}
			} else if info != nil {
				t.Fatalf("expected nil (pass), got %+v", info)
			}
		})
	}

	// 空配置/畸形JSON永不拦截
	if got := CheckChannelModelQuota(ch, ``, "default", "gpt-4o", now); got != nil {
		t.Fatalf("empty settings must pass, got %+v", got)
	}
}
