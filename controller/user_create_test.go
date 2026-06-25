package controller

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupUserCreateTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	gin.SetMode(gin.TestMode)
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite db: %v", err)
	}
	model.DB = db
	model.LOG_DB = db

	if err := db.AutoMigrate(&model.User{}); err != nil {
		t.Fatalf("failed to migrate users table: %v", err)
	}

	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})

	return db
}

func TestCreateUserPersistsAdminProvidedProfileFields(t *testing.T) {
	db := setupUserCreateTestDB(t)
	payload := map[string]any{
		"username":     "created_user",
		"password":     "password123",
		"display_name": "创建用户显示名",
		"remark":       "管理员备注",
		"group":        "vip",
		"role":         common.RoleCommonUser,
	}
	body, err := common.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to marshal request payload: %v", err)
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/user/", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("role", common.RoleRootUser)

	CreateUser(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}
	var response struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	if err := common.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !response.Success {
		t.Fatalf("expected successful response, got message %q", response.Message)
	}

	var created model.User
	if err := db.Where("username = ?", "created_user").First(&created).Error; err != nil {
		t.Fatalf("failed to fetch created user: %v", err)
	}
	if created.DisplayName != "创建用户显示名" {
		t.Fatalf("expected display name to be persisted, got %q", created.DisplayName)
	}
	if created.Remark != "管理员备注" {
		t.Fatalf("expected remark to be persisted, got %q", created.Remark)
	}
	if created.Group != "vip" {
		t.Fatalf("expected group to be persisted, got %q", created.Group)
	}
}
