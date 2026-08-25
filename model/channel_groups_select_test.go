package model

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// setupGroupsSelectCache swaps the in-memory channel cache with test fixtures.
func setupGroupsSelectCache(t *testing.T, channels map[int]*Channel, groupModel map[string]map[string][]int) {
	t.Helper()
	oldMemoryCacheEnabled := common.MemoryCacheEnabled
	oldGroup2Model2Channels := group2model2channels
	oldChannelsIDM := channelsIDM
	t.Cleanup(func() {
		common.MemoryCacheEnabled = oldMemoryCacheEnabled
		group2model2channels = oldGroup2Model2Channels
		channelsIDM = oldChannelsIDM
	})
	common.MemoryCacheEnabled = true
	channelsIDM = channels
	group2model2channels = groupModel
}

func TestGetRandomSatisfiedChannelByGroups_FirstGroupWins(t *testing.T) {
	p1, p2 := int64(10), int64(10)
	setupGroupsSelectCache(t,
		map[int]*Channel{
			1: {Id: 1, Priority: &p1},
			2: {Id: 2, Priority: &p2},
		},
		map[string]map[string][]int{
			"ga": {"m": {1}},
			"gb": {"m": {2}},
		},
	)

	channel, group, err := GetRandomSatisfiedChannelByGroups([]string{"ga", "gb"}, "m", 0, common.FormatGroupOther)
	if err != nil || channel == nil {
		t.Fatalf("channel=%v err=%v", channel, err)
	}
	if channel.Id != 1 || group != "ga" {
		t.Errorf("got (ch#%d, %s), want (ch#1, ga)", channel.Id, group)
	}
}

func TestGetRandomSatisfiedChannelByGroups_FallsThroughInOrder(t *testing.T) {
	p := int64(10)
	setupGroupsSelectCache(t,
		map[int]*Channel{2: {Id: 2, Priority: &p}},
		map[string]map[string][]int{
			"gb": {"m": {2}},
		},
	)

	channel, group, err := GetRandomSatisfiedChannelByGroups([]string{"ga", "gb"}, "m", 0, common.FormatGroupOther)
	if err != nil || channel == nil {
		t.Fatalf("channel=%v err=%v", channel, err)
	}
	if channel.Id != 2 || group != "gb" {
		t.Errorf("got (ch#%d, %s), want (ch#2, gb)", channel.Id, group)
	}
}

func TestGetRandomSatisfiedChannelByGroups_AllMissReturnsNil(t *testing.T) {
	setupGroupsSelectCache(t, map[int]*Channel{}, map[string]map[string][]int{})

	channel, group, err := GetRandomSatisfiedChannelByGroups([]string{"ga", "gb"}, "m", 0, common.FormatGroupOther)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if channel != nil || group != "" {
		t.Errorf("got (ch#%v, %q), want (nil, \"\")", channel, group)
	}
}

func TestGetRandomSatisfiedChannelByGroups_RetryAppliesWithinFirstGroup(t *testing.T) {
	high, low := int64(10), int64(5)
	setupGroupsSelectCache(t,
		map[int]*Channel{
			1: {Id: 1, Priority: &high},
			2: {Id: 2, Priority: &low},
			3: {Id: 3, Priority: &high},
		},
		map[string]map[string][]int{
			"ga": {"m": {1, 2}},
			"gb": {"m": {3}},
		},
	)

	// retry=1 应在 ga 组内降级到低优先级渠道，而不是切到 gb
	channel, group, err := GetRandomSatisfiedChannelByGroups([]string{"ga", "gb"}, "m", 1, common.FormatGroupOther)
	if err != nil || channel == nil {
		t.Fatalf("channel=%v err=%v", channel, err)
	}
	if channel.Id != 2 || group != "ga" {
		t.Errorf("got (ch#%d, %s), want (ch#2, ga)", channel.Id, group)
	}
}

func TestGetRandomSatisfiedChannelByGroups_RetryBeyondPrioritiesClamps(t *testing.T) {
	p := int64(10)
	setupGroupsSelectCache(t,
		map[int]*Channel{
			1: {Id: 1, Priority: &p},
			2: {Id: 2, Priority: &p},
		},
		map[string]map[string][]int{
			"ga": {"m": {1}},
			"gb": {"m": {2}},
		},
	)

	// retry 超过 ga 的优先级层数时应收敛到 ga 的最低优先级，而不是穿透到 gb
	channel, group, err := GetRandomSatisfiedChannelByGroups([]string{"ga", "gb"}, "m", 5, common.FormatGroupOther)
	if err != nil || channel == nil {
		t.Fatalf("channel=%v err=%v", channel, err)
	}
	if channel.Id != 1 || group != "ga" {
		t.Errorf("got (ch#%d, %s), want (ch#1, ga)", channel.Id, group)
	}
}

func TestGetRandomSatisfiedChannelByGroups_EmptyGroupsReturnsNil(t *testing.T) {
	setupGroupsSelectCache(t, map[int]*Channel{}, map[string]map[string][]int{})
	channel, group, err := GetRandomSatisfiedChannelByGroups(nil, "m", 0, common.FormatGroupOther)
	if err != nil || channel != nil || group != "" {
		t.Errorf("got (%v, %q, %v), want (nil, \"\", nil)", channel, group, err)
	}
}

// --- DB 模式（关闭内存缓存）---

type queryCountingLogger struct {
	count *int
}

func (l queryCountingLogger) LogMode(gormlogger.LogLevel) gormlogger.Interface { return l }
func (l queryCountingLogger) Info(context.Context, string, ...interface{})     {}
func (l queryCountingLogger) Warn(context.Context, string, ...interface{})     {}
func (l queryCountingLogger) Error(context.Context, string, ...interface{})    {}
func (l queryCountingLogger) Trace(_ context.Context, _ time.Time, _ func() (string, int64), _ error) {
	*l.count++
}

func setupGroupsSelectDB(t *testing.T) *int {
	t.Helper()
	oldDB := DB
	oldMem := common.MemoryCacheEnabled
	oldUsingSQLite := common.UsingSQLite
	queryCount := 0
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: queryCountingLogger{count: &queryCount},
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&Channel{}, &Ability{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	DB = db
	common.MemoryCacheEnabled = false
	common.UsingSQLite = true
	initCol()
	t.Cleanup(func() {
		DB = oldDB
		common.MemoryCacheEnabled = oldMem
		common.UsingSQLite = oldUsingSQLite
		initCol()
	})
	return &queryCount
}

func seedGroupChannel(t *testing.T, id int, group string, enabled bool, priority int64, weight uint) {
	t.Helper()
	status := common.ChannelStatusEnabled
	if !enabled {
		status = common.ChannelStatusManuallyDisabled
	}
	ch := Channel{
		Id: id, Name: "ch", Type: constant.ChannelTypeOpenAI,
		Status: status, Group: group, Models: "m", Key: "sk-test",
		Priority: &priority, Weight: &weight,
	}
	if err := DB.Create(&ch).Error; err != nil {
		t.Fatalf("seed channel %d: %v", id, err)
	}
	ab := Ability{Group: group, Model: "m", ChannelId: id, Enabled: enabled, Priority: &priority, Weight: weight}
	if err := DB.Create(&ab).Error; err != nil {
		t.Fatalf("seed ability %d/%s: %v", id, group, err)
	}
}

func TestGetRandomSatisfiedChannelByGroups_DB_FallsThroughInOrder(t *testing.T) {
	setupGroupsSelectDB(t)
	seedGroupChannel(t, 1, "ga", false, 10, 1) // ga 无可用能力
	seedGroupChannel(t, 2, "gb", true, 10, 1)

	channel, group, err := GetRandomSatisfiedChannelByGroups([]string{"ga", "gb"}, "m", 0, common.FormatGroupOther)
	if err != nil || channel == nil {
		t.Fatalf("channel=%v err=%v", channel, err)
	}
	if channel.Id != 2 || group != "gb" {
		t.Errorf("got (ch#%d, %s), want (ch#2, gb)", channel.Id, group)
	}
}

func TestGetRandomSatisfiedChannelByGroups_DB_RetryWithinFirstGroup(t *testing.T) {
	setupGroupsSelectDB(t)
	seedGroupChannel(t, 1, "ga", true, 10, 1)
	seedGroupChannel(t, 2, "ga", true, 5, 1)
	seedGroupChannel(t, 3, "gb", true, 10, 1)

	channel, group, err := GetRandomSatisfiedChannelByGroups([]string{"ga", "gb"}, "m", 1, common.FormatGroupOther)
	if err != nil || channel == nil {
		t.Fatalf("channel=%v err=%v", channel, err)
	}
	if channel.Id != 2 || group != "ga" {
		t.Errorf("got (ch#%d, %s), want (ch#2, ga)", channel.Id, group)
	}
}

func TestGetRandomSatisfiedChannelByGroups_DB_ConstantQueryCount(t *testing.T) {
	queryCount := setupGroupsSelectDB(t)
	for i, g := range []string{"g1", "g2", "g3", "g4"} {
		seedGroupChannel(t, 100+i, g, true, 10, 1)
	}

	*queryCount = 0
	channel, group, err := GetRandomSatisfiedChannelByGroups([]string{"g1", "g2", "g3", "g4"}, "m", 1, common.FormatGroupOther)
	if err != nil || channel == nil {
		t.Fatalf("channel=%v err=%v", channel, err)
	}
	if group != "g1" {
		t.Errorf("group = %s, want g1", group)
	}
	// 批量接口必须将 N 组 N 套查询（每条含 MAX 子查询 + getPriority + 渠道回查）
	// 压缩为常数级：1 条 abilities 批量查询 + 1 条渠道回查
	if *queryCount > 2 {
		t.Errorf("query count = %d, want <= 2 for 4 groups", *queryCount)
	}
}

func TestGetRandomSatisfiedChannelByGroups_DB_AllMissReturnsNil(t *testing.T) {
	setupGroupsSelectDB(t)
	seedGroupChannel(t, 1, "ga", false, 10, 1)

	channel, group, err := GetRandomSatisfiedChannelByGroups([]string{"ga", "gb"}, "m", 0, common.FormatGroupOther)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if channel != nil || group != "" {
		t.Errorf("got (%v, %q), want (nil, \"\")", channel, group)
	}
}
