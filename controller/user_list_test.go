package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func seedUsersForListHandlerTest(t *testing.T, db *gorm.DB) {
	t.Helper()

	now := time.Now()
	users := []model.User{
		{Username: "enabled_user", Status: common.UserStatusEnabled, Password: "pass123456", AffCode: "af1"},
		{Username: "disabled_user", Status: common.UserStatusDisabled, Password: "pass123456", AffCode: "af2"},
		{Username: "deleted_user", Status: common.UserStatusEnabled, Password: "pass123456", AffCode: "af3", DeletedAt: gorm.DeletedAt{Time: now, Valid: true}},
	}

	for i := range users {
		if err := db.Create(&users[i]).Error; err != nil {
			t.Fatalf("failed to seed user %s: %v", users[i].Username, err)
		}
	}
}

func TestGetAllUsersHandlerStatusFilter(t *testing.T) {
	db := setupUserCreateTestDB(t)
	seedUsersForListHandlerTest(t, db)

	t.Run("status=active returns only enabled users", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodGet, "/api/user/?status=active", nil)

		GetAllUsers(ctx)

		if recorder.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", recorder.Code, recorder.Body.String())
		}
		var response struct {
			Success bool            `json:"success"`
			Data    common.PageInfo `json:"data"`
		}
		if err := common.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if !response.Success {
			t.Fatalf("expected success response, got %s", recorder.Body.String())
		}
		if response.Data.Total != 1 {
			t.Errorf("total = %d, want 1", response.Data.Total)
		}
		items, ok := response.Data.Items.([]interface{})
		if !ok {
			t.Fatalf("items is not []interface{}")
		}
		if len(items) != 1 {
			t.Errorf("len(items) = %d, want 1", len(items))
		}
	})

	t.Run("no status filter returns all users", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodGet, "/api/user/", nil)

		GetAllUsers(ctx)

		if recorder.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", recorder.Code, recorder.Body.String())
		}
		var response struct {
			Success bool            `json:"success"`
			Data    common.PageInfo `json:"data"`
		}
		if err := common.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if response.Data.Total != 3 {
			t.Errorf("total = %d, want 3", response.Data.Total)
		}
	})
}

func TestSearchUsersHandlerStatusFilter(t *testing.T) {
	db := setupUserCreateTestDB(t)
	seedUsersForListHandlerTest(t, db)

	t.Run("status=active filters search results", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodGet, "/api/user/search?keyword=user&status=active", nil)

		SearchUsers(ctx)

		if recorder.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", recorder.Code, recorder.Body.String())
		}
		var response struct {
			Success bool            `json:"success"`
			Data    common.PageInfo `json:"data"`
		}
		if err := common.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if response.Data.Total != 1 {
			t.Errorf("total = %d, want 1", response.Data.Total)
		}
	})

	t.Run("no status filter returns matching disabled and deleted users", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodGet, "/api/user/search?keyword=user", nil)

		SearchUsers(ctx)

		if recorder.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", recorder.Code, recorder.Body.String())
		}
		var response struct {
			Success bool            `json:"success"`
			Data    common.PageInfo `json:"data"`
		}
		if err := common.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if response.Data.Total != 3 {
			t.Errorf("total = %d, want 3", response.Data.Total)
		}
	})
}
