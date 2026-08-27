package model

import (
	"reflect"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestParseGroupList(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"empty", "", []string{}},
		{"spaces only", " , ,", []string{}},
		{"single", "default", []string{"default"}},
		{"multiple", "vip,svip", []string{"vip", "svip"}},
		{"trims spaces", " vip , svip ,", []string{"vip", "svip"}},
		{"dedupe keeps first", "a,b,a,b", []string{"a", "b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ParseGroupList(tt.in); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseGroupList(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestNormalizeGroups(t *testing.T) {
	t.Run("syncs primary group to first", func(t *testing.T) {
		u := &User{Group: "old", Groups: "b , a,b"}
		u.NormalizeGroups()
		if u.Groups != "b,a" {
			t.Errorf("Groups = %q, want %q", u.Groups, "b,a")
		}
		if u.Group != "b" {
			t.Errorf("Group = %q, want %q", u.Group, "b")
		}
	})
	t.Run("empty list falls back to default", func(t *testing.T) {
		u := &User{}
		u.NormalizeGroups()
		if u.Group != "default" || u.Groups != "" {
			t.Errorf("got (%q,%q), want (default,\"\")", u.Group, u.Groups)
		}
	})
	t.Run("only legacy group set", func(t *testing.T) {
		u := &User{Group: "vip"}
		u.NormalizeGroups()
		if u.Group != "vip" || u.Groups != "vip" {
			t.Errorf("got (%q,%q), want (vip,vip)", u.Group, u.Groups)
		}
	})
}

func TestGetGroupListFallbackChain(t *testing.T) {
	cases := []struct {
		name string
		user User
		want []string
	}{
		{"groups wins", User{Group: "x", Groups: "a,b"}, []string{"a", "b"}},
		{"legacy group fallback", User{Group: "vip"}, []string{"vip"}},
		{"final default", User{}, []string{"default"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.user.GetGroupList(); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("GetGroupList() = %v, want %v", got, tc.want)
			}
			base := UserBase{Group: tc.user.Group, Groups: tc.user.Groups}
			if got := base.GetGroupList(); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("UserBase.GetGroupList() = %v, want %v", got, tc.want)
			}
		})
	}
}

func setupUserGroupsTestDB(t *testing.T) {
	t.Helper()
	oldDB := DB
	oldUsingSQLite := common.UsingSQLite
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&User{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	DB = db
	common.UsingSQLite = true
	InitCol()
	t.Cleanup(func() {
		DB = oldDB
		common.UsingSQLite = oldUsingSQLite
		InitCol()
	})
}

func TestMigrateUserGroupsBackfillsAndIsIdempotent(t *testing.T) {
	setupUserGroupsTestDB(t)
	user := User{Username: "u1", Password: "12345678", Group: "ga"}
	if err := DB.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := DB.Exec("UPDATE users SET groups = NULL").Error; err != nil {
		t.Fatalf("null out groups: %v", err)
	}

	migrateUserGroups()

	var got User
	if err := DB.First(&got, "username = ?", "u1").Error; err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if got.Groups != "ga" {
		t.Fatalf("Groups = %q, want %q", got.Groups, "ga")
	}

	migrateUserGroups()
	got = User{}
	if err := DB.First(&got, "username = ?", "u1").Error; err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if got.Groups != "ga" {
		t.Fatalf("second run Groups = %q, want stable %q", got.Groups, "ga")
	}
}

func usernames(users []*User) []string {
	out := make([]string, 0, len(users))
	for _, u := range users {
		out = append(out, u.Username)
	}
	return out
}

func TestEditPersistsGroups(t *testing.T) {
	setupUserGroupsTestDB(t)
	user := User{Username: "u2", Password: "12345678", Group: "ga"}
	if err := DB.Create(&user).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	user.Group = "gb"
	user.Groups = "gb,gc"
	if err := user.Edit(false); err != nil {
		t.Fatalf("edit: %v", err)
	}
	var got User
	DB.First(&got, user.Id)
	if got.Groups != "gb,gc" || got.Group != "gb" {
		t.Errorf("got (%q,%q), want (gb,gc / gb)", got.Group, got.Groups)
	}
}

func TestSearchUsersMatchesAnyBoundGroup(t *testing.T) {
	setupUserGroupsTestDB(t)
	DB.Create(&User{Username: "u3", Password: "12345678", Group: "ga", Groups: "ga,gb", AffCode: "a3"})
	DB.Create(&User{Username: "u4", Password: "12345678", Group: "gc", AffCode: "a4"})
	DB.Create(&User{Username: "u5", Password: "12345678", Group: "ga", AffCode: "a5"})

	users, total, err := SearchUsers("", "gb", 0, 10, "")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if total != 1 || len(users) != 1 || users[0].Username != "u3" {
		t.Errorf("search gb got total=%d users=%v", total, usernames(users))
	}

	users, total, err = SearchUsers("", "ga", 0, 10, "")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if total != 2 { // u3 (ga,gb) + u5 旧数据 group 精确匹配
		t.Errorf("search ga got total=%d users=%v", total, usernames(users))
	}
}
