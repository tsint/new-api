package service

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"

	"github.com/glebarez/sqlite"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func setupChannelSelectEnv(t *testing.T) {
	t.Helper()
	oldDB, oldMem := model.DB, common.MemoryCacheEnabled
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.Channel{}, &model.Ability{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	model.DB = db
	common.MemoryCacheEnabled = true
	t.Cleanup(func() {
		model.DB = oldDB
		common.MemoryCacheEnabled = oldMem
	})
}

func csSeedChannel(t *testing.T, id int, group string, enabled bool, priority int64) {
	t.Helper()
	status := 1
	if !enabled {
		status = 2
	}
	ch := model.Channel{
		Id: id, Name: fmt.Sprintf("ch-%d", id), Type: constant.ChannelTypeOpenAI,
		Status: status, Group: group, Models: "test-model", Key: "sk-test",
		Priority: &priority,
	}
	if err := model.DB.Create(&ch).Error; err != nil {
		t.Fatalf("seed channel %d: %v", id, err)
	}
	for _, g := range strings.Split(group, ",") {
		ab := model.Ability{Group: g, Model: "test-model", ChannelId: id, Enabled: enabled, Priority: &priority, Weight: 0}
		if err := model.DB.Create(&ab).Error; err != nil {
			t.Fatalf("seed ability %d/%s: %v", id, g, err)
		}
	}
	model.InitChannelCache()
}

func newSelectCtx(t *testing.T, userGroups []string) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	common.SetContextKey(c, constant.ContextKeyUserGroup, userGroups[0])
	common.SetContextKey(c, constant.ContextKeyUserGroupList, userGroups)
	return c
}

func multiGroupParam(c *gin.Context) *RetryParam {
	return &RetryParam{
		Ctx:         c,
		ModelName:   "test-model",
		TokenGroup:  "",
		Retry:       common.GetPointer(0),
		FormatGroup: common.FormatGroupOther,
	}
}

func TestMultiGroupSelectsFirstAvailableGroup(t *testing.T) {
	setupChannelSelectEnv(t)
	csSeedChannel(t, 1, "ga", true, 10)
	csSeedChannel(t, 2, "gb", true, 10)
	c := newSelectCtx(t, []string{"ga", "gb"})
	channel, selectGroup, err := CacheGetRandomSatisfiedChannel(multiGroupParam(c))
	if err != nil || channel == nil {
		t.Fatalf("channel=%v err=%v", channel, err)
	}
	if channel.Id != 1 || selectGroup != "ga" {
		t.Errorf("got (ch#%d, %s), want (ch#1, ga)", channel.Id, selectGroup)
	}
	if v, _ := common.GetContextKey(c, constant.ContextKeyAutoGroup); v != "ga" {
		t.Errorf("ContextKeyAutoGroup = %v, want ga", v)
	}
}

func TestMultiGroupFallsThroughToNextGroup(t *testing.T) {
	setupChannelSelectEnv(t)
	csSeedChannel(t, 1, "ga", false, 10) // ga 渠道禁用
	csSeedChannel(t, 2, "gb", true, 10)
	c := newSelectCtx(t, []string{"ga", "gb"})
	channel, selectGroup, err := CacheGetRandomSatisfiedChannel(multiGroupParam(c))
	if err != nil || channel == nil {
		t.Fatalf("channel=%v err=%v", channel, err)
	}
	if channel.Id != 2 || selectGroup != "gb" {
		t.Errorf("got (ch#%d, %s), want (ch#2, gb)", channel.Id, selectGroup)
	}
}

func TestMultiGroupAllExhaustedReturnsNil(t *testing.T) {
	setupChannelSelectEnv(t)
	csSeedChannel(t, 1, "ga", false, 10)
	c := newSelectCtx(t, []string{"ga", "gb"})
	channel, _, err := CacheGetRandomSatisfiedChannel(multiGroupParam(c))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if channel != nil {
		t.Errorf("channel = #%d, want nil", channel.Id)
	}
}

func TestMultiGroupRetryResumesFromRecordedIndex(t *testing.T) {
	setupChannelSelectEnv(t)
	csSeedChannel(t, 1, "ga", false, 10)          // ga 无可用渠道
	csSeedChannel(t, 2, "gb", true, 10)           // gb 高优先级
	csSeedChannel(t, 3, "gb", true, 5)            // gb 低优先级
	c := newSelectCtx(t, []string{"ga", "gb"})
	channel, selectGroup, err := CacheGetRandomSatisfiedChannel(multiGroupParam(c))
	if err != nil || channel == nil {
		t.Fatalf("first call channel=%v err=%v", channel, err)
	}
	if channel.Id != 2 || selectGroup != "gb" {
		t.Fatalf("first call got (ch#%d, %s), want (ch#2, gb)", channel.Id, selectGroup)
	}
	// 重试：应从记录的 gb 索引继续、在 gb 内降级到低优先级渠道，而不是回到 ga
	retry := 1
	p := multiGroupParam(c)
	p.Retry = &retry
	channel, selectGroup, err = CacheGetRandomSatisfiedChannel(p)
	if err != nil || channel == nil {
		t.Fatalf("retry channel=%v err=%v", channel, err)
	}
	if channel.Id != 3 || selectGroup != "gb" {
		t.Errorf("retry got (ch#%d, %s), want (ch#3, gb)", channel.Id, selectGroup)
	}
}

func TestUserGroupListFromCtxFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	common.SetContextKey(c, constant.ContextKeyUserGroup, "solo")
	got := UserGroupListFromCtx(c)
	if len(got) != 1 || got[0] != "solo" {
		t.Errorf("got %v, want [solo]", got)
	}
}

func TestMatchGroupForChannel(t *testing.T) {
	setupChannelSelectEnv(t)
	csSeedChannel(t, 1, "gb", true, 10)
	got, ok := MatchGroupForChannel([]string{"ga", "gb"}, "test-model", 1)
	if !ok || got != "gb" {
		t.Errorf("got (%q,%v), want (gb,true)", got, ok)
	}
	if _, ok := MatchGroupForChannel([]string{"ga"}, "test-model", 1); ok {
		t.Error("ga should not match channel 1")
	}
	if _, ok := MatchGroupForChannel([]string{"ga", "gb"}, "other-model", 1); ok {
		t.Error("model mismatch should not match")
	}
}
