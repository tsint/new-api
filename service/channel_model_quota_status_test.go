package service

import (
	"testing"
	"time"
)

var statusNow = time.Unix(1800000000, 0)

func qsChan(id int, name, group, models, settings string) QuotaStatusChannel {
	return QuotaStatusChannel{Id: id, Name: name, Group: group, Models: models, ModelQuotaSettings: settings}
}

func alwaysServes(QuotaStatusChannel, string, string) bool { return true }

func TestBuildChannelQuotaStatusRows(t *testing.T) {
	orig := cqStore
	store := NewChannelQuotaStore(nil)
	cqStore = store
	t.Cleanup(func() { cqStore = orig })

	idx := QuotaBlockIndex(statusNow)
	store.Add(1, "default", "gpt-4o", idx, 800000)

	channels := []QuotaStatusChannel{
		qsChan(1, "chan-a", "default,vip", "gpt-4o,claude", `{"default":{"gpt-4o":1000000,"claude":500}}`),
		qsChan(2, "chan-b", "vip", "*", `{"vip":{"*":77}}`),
		qsChan(3, "chan-c", "default", "m", ``),          // 未配置 → 无行
		qsChan(4, "chan-d", "default", "m", `{"vip":..}`), // 畸形JSON → 跳过该渠道
	}

	rows := BuildChannelQuotaStatusRows(channels, []string{"default"}, statusNow, store, alwaysServes)

	if len(rows) != 2 {
		t.Fatalf("rows=%d want 2 (only user's groups with positive limits), got %+v", len(rows), rows)
	}

	byKey := func(r ChannelModelQuotaStatusRow) string { return r.ChannelName + "/" + r.Model }
	first := map[string]*ChannelModelQuotaStatusRow{}
	for i := range rows {
		first[byKey(rows[i])] = &rows[i]
	}

	a := first["chan-a/gpt-4o"]
	if a == nil {
		t.Fatal("missing chan-a/gpt-4o")
	}
	if a.Limit4h != 1000000 || a.UsedCurrentBlock != 800000 || a.Remaining != 200000 {
		t.Fatalf("chan-a numbers wrong: %+v", a)
	}
	if a.Status != "normal" {
		t.Fatalf("chan-a status=%q want normal", a.Status)
	}
	if a.ResetAt != NextQuotaBlockStart(statusNow) {
		t.Fatalf("resetAt mismatch: %d", a.ResetAt)
	}

	c := first["chan-a/claude"]
	if c == nil || c.Limit4h != 500 || c.Status != "normal" {
		t.Fatalf("chan-a/claude wrong: %+v", c)
	}

	// vip 组不属于该用户 → chan-b 不出现
	for _, r := range rows {
		if r.ChannelId == 2 {
			t.Fatalf("chan-b leaked into other-group view: %+v", r)
		}
	}

	// 用尽状态
	store.Add(5, "default", "x", idx, 10)
	rows2 := BuildChannelQuotaStatusRows(
		[]QuotaStatusChannel{qsChan(5, "chan-e", "default", "x", `{"default":{"x":9}}`)},
		[]string{"default"}, statusNow, store, alwaysServes,
	)
	if len(rows2) != 1 || rows2[0].Status != "exhausted" || rows2[0].Remaining != 0 {
		t.Fatalf("exhausted row wrong: %+v", rows2)
	}
}

func TestBuildRowsRespectsServesPredicate(t *testing.T) {
	channels := []QuotaStatusChannel{
		qsChan(7, "chan-f", "default", "gpt-4o", `{"default":{"gpt-4o":10}}`),
	}
	serves := func(_ QuotaStatusChannel, group, model string) bool {
		return !(group == "default" && model == "gpt-4o") // 渠道能力表说不可用
	}
	rows := BuildChannelQuotaStatusRows(channels, []string{"default"}, statusNow, NewChannelQuotaStore(nil), serves)
	if len(rows) != 0 {
		t.Fatalf("predicate gate failed, rows=%d", len(rows))
	}
}
