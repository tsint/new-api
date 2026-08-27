package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupModelListTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	gin.SetMode(gin.TestMode)
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false
	model.InitCol()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite db: %v", err)
	}
	model.DB = db
	model.LOG_DB = db

	if err := db.AutoMigrate(&model.User{}, &model.Ability{}); err != nil {
		t.Fatalf("failed to migrate tables: %v", err)
	}

	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})

	return db
}

func seedMultiGroupAbilities(t *testing.T, db *gorm.DB) {
	t.Helper()

	abilities := []model.Ability{
		{Group: "g1", Model: "g1-only-model", ChannelId: 1, Enabled: true},
		{Group: "g1", Model: "shared-model", ChannelId: 1, Enabled: true},
		{Group: "g2", Model: "g2-only-model", ChannelId: 2, Enabled: true},
		{Group: "g2", Model: "shared-model", ChannelId: 2, Enabled: true},
	}
	for i := range abilities {
		if err := db.Create(&abilities[i]).Error; err != nil {
			t.Fatalf("failed to seed ability %+v: %v", abilities[i], err)
		}
	}
}

func seedMultiGroupUser(t *testing.T, db *gorm.DB) model.User {
	t.Helper()

	user := model.User{
		Username: "multi_group_user",
		Group:    "g1",
		Groups:   "g1,g2",
		Status:   common.UserStatusEnabled,
		Password: "pass123456",
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("failed to seed user: %v", err)
	}
	return user
}

// callListModelsAnthropic simulates GET /v1/models with x-api-key + anthropic-version
// headers after TokenAuth middleware has populated context.
func callListModelsAnthropic(t *testing.T, userId int, tokenGroup string) []string {
	t.Helper()

	prevSelfUse := operation_setting.SelfUseModeEnabled
	operation_setting.SelfUseModeEnabled = true
	t.Cleanup(func() { operation_setting.SelfUseModeEnabled = prevSelfUse })

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	ctx.Request.Header.Set("x-api-key", "sk-test")
	ctx.Request.Header.Set("anthropic-version", "2023-06-01")
	ctx.Set("id", userId)
	if tokenGroup != "" {
		common.SetContextKey(ctx, constant.ContextKeyTokenGroup, tokenGroup)
	}

	ListModels(ctx, constant.ChannelTypeAnthropic)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := common.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to decode response: %v (body: %s)", err, recorder.Body.String())
	}
	ids := make([]string, 0, len(response.Data))
	for _, m := range response.Data {
		ids = append(ids, m.ID)
	}
	sort.Strings(ids)
	return ids
}

func TestListModelsAnthropicTokenWithoutGroupUnionsAllUserGroups(t *testing.T) {
	db := setupModelListTestDB(t)
	seedMultiGroupAbilities(t, db)
	user := seedMultiGroupUser(t, db)

	ids := callListModelsAnthropic(t, user.Id, "")

	want := []string{"g1-only-model", "g2-only-model", "shared-model"}
	if !reflect.DeepEqual(ids, want) {
		t.Errorf("token without group: got %v, want %v", ids, want)
	}
}

func TestListModelsAnthropicExplicitTokenGroupReturnsThatGroupOnly(t *testing.T) {
	db := setupModelListTestDB(t)
	seedMultiGroupAbilities(t, db)
	user := seedMultiGroupUser(t, db)

	ids := callListModelsAnthropic(t, user.Id, "g1")

	want := []string{"g1-only-model", "shared-model"}
	if !reflect.DeepEqual(ids, want) {
		t.Errorf("token bound to g1: got %v, want %v", ids, want)
	}
}

func TestListModelsAnthropicNoAccessibleModelsReturnsEmptyNotPanic(t *testing.T) {
	db := setupModelListTestDB(t)
	user := seedMultiGroupUser(t, db)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	ctx.Set("id", user.Id)

	ListModels(ctx, constant.ChannelTypeAnthropic)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := common.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to decode response: %v (body: %s)", err, recorder.Body.String())
	}
	if len(response.Data) != 0 {
		t.Errorf("expected empty data, got %+v", response.Data)
	}
}
