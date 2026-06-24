package sessionaudit

import "testing"

func TestDefaultConfigIsSafeAndDisabled(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Enabled {
		t.Fatal("default session audit must be disabled")
	}
	if cfg.OutputDir != "/app/audit_logs" {
		t.Fatalf("OutputDir = %q, want /app/audit_logs", cfg.OutputDir)
	}
	if cfg.MaxBytesPerUser != 10*1024*1024 {
		t.Fatalf("MaxBytesPerUser = %d, want 10 MiB", cfg.MaxBytesPerUser)
	}
	if cfg.MaxRequestBodyBytes != 64*1024 {
		t.Fatalf("MaxRequestBodyBytes = %d, want 64 KiB", cfg.MaxRequestBodyBytes)
	}
	if !cfg.CaptureRequestBody {
		t.Fatal("request body capture should be enabled by default once audit is enabled")
	}
	if cfg.CaptureRequestHeaders {
		t.Fatal("request headers must not be captured by default")
	}
	if !cfg.RedactSensitiveHeaders {
		t.Fatal("sensitive headers must be redacted by default")
	}
}

func TestConfigFromEnvParsesDefaultsAndForceConstraints(t *testing.T) {
	t.Setenv("SESSION_AUDIT_ENABLED", "true")
	t.Setenv("SESSION_AUDIT_SAMPLE_RATE", "0.25")
	t.Setenv("SESSION_AUDIT_RETENTION_DAYS", "3")
	t.Setenv("SESSION_AUDIT_OUTPUT_DIR", "/tmp/audit")
	t.Setenv("SESSION_AUDIT_MAX_BYTES_PER_USER", "2048")
	t.Setenv("SESSION_AUDIT_MAX_REQUEST_BODY_BYTES", "1024")
	t.Setenv("SESSION_AUDIT_INCLUDE_USER_IDS", "1, 2")
	t.Setenv("SESSION_AUDIT_EXCLUDE_TOKEN_IDS", "7,8")
	t.Setenv("SESSION_AUDIT_INCLUDE_MODEL_PATTERNS", "^gpt-,^claude-")
	t.Setenv("SESSION_AUDIT_CAPTURE_REQUEST_HEADERS", "true")
	t.Setenv("SESSION_AUDIT_FORCE_DISABLED", "true")
	t.Setenv("SESSION_AUDIT_OUTPUT_DIR_LOCKED", "true")
	t.Setenv("SESSION_AUDIT_MAX_BYTES_PER_USER_LIMIT", "4096")
	t.Setenv("SESSION_AUDIT_MAX_REQUEST_BODY_BYTES_LIMIT", "1536")

	cfg := ConfigFromEnv(DefaultConfig())

	if !cfg.Enabled {
		t.Fatal("env should enable session audit before force constraints are evaluated")
	}
	if cfg.SampleRate != 0.25 {
		t.Fatalf("SampleRate = %v, want 0.25", cfg.SampleRate)
	}
	if cfg.RetentionDays != 3 {
		t.Fatalf("RetentionDays = %d, want 3", cfg.RetentionDays)
	}
	if cfg.OutputDir != "/tmp/audit" {
		t.Fatalf("OutputDir = %q, want /tmp/audit", cfg.OutputDir)
	}
	if cfg.MaxBytesPerUser != 2048 || cfg.MaxRequestBodyBytes != 1024 {
		t.Fatalf("limits = %d/%d, want 2048/1024", cfg.MaxBytesPerUser, cfg.MaxRequestBodyBytes)
	}
	if !cfg.CaptureRequestHeaders {
		t.Fatal("CaptureRequestHeaders should be true from env")
	}
	if !cfg.ForceDisabled || !cfg.OutputDirLocked {
		t.Fatal("force disabled and output dir locked flags should be parsed")
	}
	if cfg.MaxBytesPerUserLimit != 4096 || cfg.MaxRequestBodyBytesLimit != 1536 {
		t.Fatalf("force limits = %d/%d, want 4096/1536", cfg.MaxBytesPerUserLimit, cfg.MaxRequestBodyBytesLimit)
	}
	if len(cfg.IncludeUserIDs) != 2 || cfg.IncludeUserIDs[0] != 1 || cfg.IncludeUserIDs[1] != 2 {
		t.Fatalf("IncludeUserIDs = %#v, want [1 2]", cfg.IncludeUserIDs)
	}
	if len(cfg.ExcludeTokenIDs) != 2 || cfg.ExcludeTokenIDs[0] != 7 || cfg.ExcludeTokenIDs[1] != 8 {
		t.Fatalf("ExcludeTokenIDs = %#v, want [7 8]", cfg.ExcludeTokenIDs)
	}
	if len(cfg.IncludeModelPatterns) != 2 {
		t.Fatalf("IncludeModelPatterns = %#v, want two patterns", cfg.IncludeModelPatterns)
	}
}

func TestApplyForceConstraints(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.OutputDir = "/page/configured"
	cfg.MaxBytesPerUser = 8192
	cfg.MaxRequestBodyBytes = 4096
	cfg.ForceDisabled = true
	cfg.OutputDirLocked = true
	cfg.EnvOutputDir = "/env/locked"
	cfg.MaxBytesPerUserLimit = 4096
	cfg.MaxRequestBodyBytesLimit = 1024

	got := ApplyForceConstraints(cfg)

	if got.Enabled {
		t.Fatal("ForceDisabled should disable session audit")
	}
	if got.OutputDir != "/env/locked" {
		t.Fatalf("OutputDir = %q, want locked env output dir", got.OutputDir)
	}
	if got.MaxBytesPerUser != 4096 || got.MaxRequestBodyBytes != 1024 {
		t.Fatalf("clamped limits = %d/%d, want 4096/1024", got.MaxBytesPerUser, got.MaxRequestBodyBytes)
	}
}
