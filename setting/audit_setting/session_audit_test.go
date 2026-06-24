package audit_setting

import (
	"testing"

	"github.com/QuantumNous/new-api/service/sessionaudit"
)

func TestDefaultSessionAuditSetting(t *testing.T) {
	setting := DefaultSessionAuditSetting()

	if setting.Enabled {
		t.Fatal("session audit should be disabled by default")
	}
	if setting.OutputDir != sessionaudit.DefaultOutputDir {
		t.Fatalf("OutputDir = %q, want %q", setting.OutputDir, sessionaudit.DefaultOutputDir)
	}
	if setting.MaxBytesPerUser != sessionaudit.DefaultMaxBytesPerUser {
		t.Fatalf("MaxBytesPerUser = %d, want %d", setting.MaxBytesPerUser, sessionaudit.DefaultMaxBytesPerUser)
	}
}

func TestSessionAuditSettingToRuntimeConfigAppliesForceConstraints(t *testing.T) {
	setting := DefaultSessionAuditSetting()
	setting.Enabled = true
	setting.OutputDir = "/page/audit"
	setting.MaxBytesPerUser = 4096
	setting.MaxRequestBodyBytes = 2048

	env := sessionaudit.DefaultConfig()
	env.ForceDisabled = true
	env.OutputDirLocked = true
	env.EnvOutputDir = "/env/audit"
	env.MaxBytesPerUserLimit = 1024
	env.MaxRequestBodyBytesLimit = 512

	cfg := setting.ToRuntimeConfig(env)

	if cfg.Enabled {
		t.Fatal("force disabled should override page enabled setting")
	}
	if cfg.OutputDir != "/env/audit" {
		t.Fatalf("OutputDir = %q, want locked env output dir", cfg.OutputDir)
	}
	if cfg.MaxBytesPerUser != 1024 || cfg.MaxRequestBodyBytes != 512 {
		t.Fatalf("limits = %d/%d, want 1024/512", cfg.MaxBytesPerUser, cfg.MaxRequestBodyBytes)
	}
}
