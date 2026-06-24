package model

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestBuildTimeBucketExpr(t *testing.T) {
	tests := []struct {
		name        string
		granularity string
		dbType      string
		wantContain string
	}{
		{"quarter postgres", "quarter", "postgres", "(created_at / 900) * 900"},
		{"hour mysql", "hour", "mysql", "created_at % 3600"},
		{"day sqlite", "day", "sqlite", "(created_at / 86400) * 86400"},
		{"week default", "week", "", "created_at % 604800"},
		{"invalid defaults to quarter", "invalid", "postgres", "(created_at / 900) * 900"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldPg := common.UsingPostgreSQL
			oldMy := common.UsingMySQL
			oldSl := common.UsingSQLite
			defer func() {
				common.UsingPostgreSQL = oldPg
				common.UsingMySQL = oldMy
				common.UsingSQLite = oldSl
			}()

			switch tt.dbType {
			case "postgres":
				common.UsingPostgreSQL = true
				common.UsingMySQL = false
				common.UsingSQLite = false
			case "mysql":
				common.UsingPostgreSQL = false
				common.UsingMySQL = true
				common.UsingSQLite = false
			case "sqlite":
				common.UsingPostgreSQL = false
				common.UsingMySQL = false
				common.UsingSQLite = true
			default:
				common.UsingPostgreSQL = false
				common.UsingMySQL = true
				common.UsingSQLite = false
			}

			got := buildTimeBucketExpr(tt.granularity)
			if !strings.Contains(got, tt.wantContain) {
				t.Errorf("buildTimeBucketExpr(%q) = %q, want to contain %q", tt.granularity, got, tt.wantContain)
			}
		})
	}
}

func TestLogQuotaDataTruncation(t *testing.T) {
	input := int64(1713801645)
	expected := int64(1713801600)
	got := input - (input % 900)
	if got != expected {
		t.Errorf("truncation failed: got %d, want %d", got, expected)
	}
}

func TestGetAllQuotaDatesAggregatesFilteredUsernameByGranularity(t *testing.T) {
	oldDB := DB
	oldPg := common.UsingPostgreSQL
	oldMy := common.UsingMySQL
	oldSl := common.UsingSQLite
	t.Cleanup(func() {
		DB = oldDB
		common.UsingPostgreSQL = oldPg
		common.UsingMySQL = oldMy
		common.UsingSQLite = oldSl
	})

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	DB = db
	common.UsingPostgreSQL = false
	common.UsingMySQL = false
	common.UsingSQLite = true

	if err := DB.AutoMigrate(&QuotaData{}); err != nil {
		t.Fatalf("migrate quota_data: %v", err)
	}
	rows := []*QuotaData{
		{UserID: 1, Username: "alice", ModelName: "gpt-4o", Count: 1, Quota: 10, TokenUsed: 100, CreatedAt: 1713801600},
		{UserID: 1, Username: "alice", ModelName: "gpt-4o", Count: 2, Quota: 20, TokenUsed: 200, CreatedAt: 1713802500},
		{UserID: 2, Username: "bob", ModelName: "gpt-4o", Count: 8, Quota: 80, TokenUsed: 800, CreatedAt: 1713801600},
	}
	if err := DB.Create(&rows).Error; err != nil {
		t.Fatalf("seed quota_data: %v", err)
	}

	got, err := GetAllQuotaDates(1713801600, 1713805200, "alice", "hour")
	if err != nil {
		t.Fatalf("GetAllQuotaDates error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want one hourly bucket: %#v", len(got), got)
	}
	if got[0].CreatedAt != 1713801600 || got[0].Count != 3 || got[0].Quota != 30 || got[0].TokenUsed != 300 {
		t.Fatalf("got bucket = %#v, want aggregated alice hourly bucket", got[0])
	}
}
