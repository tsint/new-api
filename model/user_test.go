package model

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupUserTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite db: %v", err)
	}
	DB = db
	LOG_DB = db

	if err := db.AutoMigrate(&User{}); err != nil {
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

func seedUsersForFilterTest(t *testing.T, db *gorm.DB) {
	t.Helper()

	now := time.Now()
	users := []User{
		{Username: "enabled_user", Status: common.UserStatusEnabled, Password: "pass", AffCode: "af1"},
		{Username: "disabled_user", Status: common.UserStatusDisabled, Password: "pass", AffCode: "af2"},
		{Username: "deleted_user", Status: common.UserStatusEnabled, Password: "pass", AffCode: "af3", DeletedAt: gorm.DeletedAt{Time: now, Valid: true}},
		{Username: "disabled_deleted_user", Status: common.UserStatusDisabled, Password: "pass", AffCode: "af4", DeletedAt: gorm.DeletedAt{Time: now, Valid: true}},
	}

	for i := range users {
		if err := db.Create(&users[i]).Error; err != nil {
			t.Fatalf("failed to seed user %s: %v", users[i].Username, err)
		}
	}
}

func TestGetAllUsersActiveFilter(t *testing.T) {
	db := setupUserTestDB(t)
	seedUsersForFilterTest(t, db)

	t.Run("status=active returns only enabled and not deleted users", func(t *testing.T) {
		pageInfo := &common.PageInfo{Page: 1, PageSize: 10}
		users, total, err := GetAllUsers(pageInfo, "active")
		if err != nil {
			t.Fatalf("GetAllUsers returned error: %v", err)
		}
		if total != 1 {
			t.Errorf("total = %d, want 1", total)
		}
		if len(users) != 1 {
			t.Errorf("len(users) = %d, want 1", len(users))
		}
		if len(users) > 0 && users[0].Username != "enabled_user" {
			t.Errorf("got user %q, want enabled_user", users[0].Username)
		}
	})

	t.Run("status=all returns all users including disabled and deleted", func(t *testing.T) {
		pageInfo := &common.PageInfo{Page: 1, PageSize: 10}
		users, total, err := GetAllUsers(pageInfo, "all")
		if err != nil {
			t.Fatalf("GetAllUsers returned error: %v", err)
		}
		if total != 4 {
			t.Errorf("total = %d, want 4", total)
		}
		if len(users) != 4 {
			t.Errorf("len(users) = %d, want 4", len(users))
		}
	})

	t.Run("empty status filter behaves like all", func(t *testing.T) {
		pageInfo := &common.PageInfo{Page: 1, PageSize: 10}
		users, total, err := GetAllUsers(pageInfo, "")
		if err != nil {
			t.Fatalf("GetAllUsers returned error: %v", err)
		}
		if total != 4 {
			t.Errorf("total = %d, want 4", total)
		}
		if len(users) != 4 {
			t.Errorf("len(users) = %d, want 4", len(users))
		}
	})
}

func TestSearchUsersActiveFilter(t *testing.T) {
	db := setupUserTestDB(t)
	seedUsersForFilterTest(t, db)

	t.Run("status=active filters search results", func(t *testing.T) {
		users, total, err := SearchUsers("user", "", 0, 10, "active")
		if err != nil {
			t.Fatalf("SearchUsers returned error: %v", err)
		}
		if total != 1 {
			t.Errorf("total = %d, want 1", total)
		}
		if len(users) != 1 || users[0].Username != "enabled_user" {
			t.Errorf("got %+v, want enabled_user", users)
		}
	})

	t.Run("status=all returns matching disabled and deleted users", func(t *testing.T) {
		users, total, err := SearchUsers("user", "", 0, 10, "all")
		if err != nil {
			t.Fatalf("SearchUsers returned error: %v", err)
		}
		if total != 4 {
			t.Errorf("total = %d, want 4", total)
		}
		if len(users) != 4 {
			t.Errorf("len(users) = %d, want 4", len(users))
		}
	})

	t.Run("status=active combines with keyword and group", func(t *testing.T) {
		users, total, err := SearchUsers("enabled_user", "", 0, 10, "active")
		if err != nil {
			t.Fatalf("SearchUsers returned error: %v", err)
		}
		if total != 1 {
			t.Errorf("total = %d, want 1", total)
		}
		if len(users) != 1 || users[0].Username != "enabled_user" {
			t.Errorf("got %+v, want enabled_user", users)
		}
	})
}
