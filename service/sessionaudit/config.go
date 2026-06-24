package sessionaudit

import (
	"hash/fnv"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const (
	DefaultOutputDir           = "/app/audit_logs"
	DefaultMaxBytesPerUser     = 10 * 1024 * 1024
	DefaultMaxRequestBodyBytes = 64 * 1024
)

type Config struct {
	Enabled                bool
	SampleRate             float64
	RetentionDays          int
	OutputDir              string
	EnvOutputDir           string
	MaxBytesPerUser        int64
	MaxRequestBodyBytes    int64
	IncludeUserIDs         []int
	ExcludeUserIDs         []int
	IncludeModelPatterns   []string
	ExcludeModelPatterns   []string
	IncludeTokenIDs        []int
	ExcludeTokenIDs        []int
	CaptureRequestBody     bool
	CaptureRequestHeaders  bool
	RedactSensitiveHeaders bool

	ForceDisabled            bool
	OutputDirLocked          bool
	MaxBytesPerUserLimit     int64
	MaxRequestBodyBytesLimit int64
}

type DecisionInput struct {
	RequestID string
	UserID    int
	TokenID   int
	Model     string
}

func DefaultConfig() Config {
	return Config{
		Enabled:                false,
		SampleRate:             0.01,
		RetentionDays:          7,
		OutputDir:              DefaultOutputDir,
		EnvOutputDir:           DefaultOutputDir,
		MaxBytesPerUser:        DefaultMaxBytesPerUser,
		MaxRequestBodyBytes:    DefaultMaxRequestBodyBytes,
		CaptureRequestBody:     true,
		CaptureRequestHeaders:  false,
		RedactSensitiveHeaders: true,
	}
}

func ConfigFromEnv(base Config) Config {
	cfg := base

	cfg.Enabled = envBool("SESSION_AUDIT_ENABLED", cfg.Enabled)
	cfg.SampleRate = clampSampleRate(envFloat("SESSION_AUDIT_SAMPLE_RATE", cfg.SampleRate))
	cfg.RetentionDays = envInt("SESSION_AUDIT_RETENTION_DAYS", cfg.RetentionDays)
	cfg.EnvOutputDir = envString("SESSION_AUDIT_OUTPUT_DIR", cfg.EnvOutputDir)
	cfg.OutputDir = cfg.EnvOutputDir
	cfg.MaxBytesPerUser = envInt64("SESSION_AUDIT_MAX_BYTES_PER_USER", cfg.MaxBytesPerUser)
	cfg.MaxRequestBodyBytes = envInt64("SESSION_AUDIT_MAX_REQUEST_BODY_BYTES", cfg.MaxRequestBodyBytes)
	cfg.IncludeUserIDs = envIntList("SESSION_AUDIT_INCLUDE_USER_IDS", cfg.IncludeUserIDs)
	cfg.ExcludeUserIDs = envIntList("SESSION_AUDIT_EXCLUDE_USER_IDS", cfg.ExcludeUserIDs)
	cfg.IncludeModelPatterns = envStringList("SESSION_AUDIT_INCLUDE_MODEL_PATTERNS", cfg.IncludeModelPatterns)
	cfg.ExcludeModelPatterns = envStringList("SESSION_AUDIT_EXCLUDE_MODEL_PATTERNS", cfg.ExcludeModelPatterns)
	cfg.IncludeTokenIDs = envIntList("SESSION_AUDIT_INCLUDE_TOKEN_IDS", cfg.IncludeTokenIDs)
	cfg.ExcludeTokenIDs = envIntList("SESSION_AUDIT_EXCLUDE_TOKEN_IDS", cfg.ExcludeTokenIDs)
	cfg.CaptureRequestBody = envBool("SESSION_AUDIT_CAPTURE_REQUEST_BODY", cfg.CaptureRequestBody)
	cfg.CaptureRequestHeaders = envBool("SESSION_AUDIT_CAPTURE_REQUEST_HEADERS", cfg.CaptureRequestHeaders)
	cfg.RedactSensitiveHeaders = envBool("SESSION_AUDIT_REDACT_SENSITIVE_HEADERS", cfg.RedactSensitiveHeaders)

	cfg.ForceDisabled = envBool("SESSION_AUDIT_FORCE_DISABLED", cfg.ForceDisabled)
	cfg.OutputDirLocked = envBool("SESSION_AUDIT_OUTPUT_DIR_LOCKED", cfg.OutputDirLocked)
	cfg.MaxBytesPerUserLimit = envInt64("SESSION_AUDIT_MAX_BYTES_PER_USER_LIMIT", cfg.MaxBytesPerUserLimit)
	cfg.MaxRequestBodyBytesLimit = envInt64("SESSION_AUDIT_MAX_REQUEST_BODY_BYTES_LIMIT", cfg.MaxRequestBodyBytesLimit)

	return cfg
}

func ApplyForceConstraints(cfg Config) Config {
	if cfg.ForceDisabled {
		cfg.Enabled = false
	}
	if cfg.OutputDirLocked && cfg.EnvOutputDir != "" {
		cfg.OutputDir = cfg.EnvOutputDir
	}
	if cfg.MaxBytesPerUserLimit > 0 && cfg.MaxBytesPerUser > cfg.MaxBytesPerUserLimit {
		cfg.MaxBytesPerUser = cfg.MaxBytesPerUserLimit
	}
	if cfg.MaxRequestBodyBytesLimit > 0 && cfg.MaxRequestBodyBytes > cfg.MaxRequestBodyBytesLimit {
		cfg.MaxRequestBodyBytes = cfg.MaxRequestBodyBytesLimit
	}
	cfg.SampleRate = clampSampleRate(cfg.SampleRate)
	if cfg.OutputDir == "" {
		cfg.OutputDir = DefaultOutputDir
	}
	if cfg.EnvOutputDir == "" {
		cfg.EnvOutputDir = cfg.OutputDir
	}
	return cfg
}

func ShouldAuditSession(cfg Config, input DecisionInput) bool {
	cfg = ApplyForceConstraints(cfg)
	if cfg.ForceDisabled || !cfg.Enabled {
		return false
	}
	if containsInt(cfg.ExcludeUserIDs, input.UserID) {
		return false
	}
	if len(cfg.IncludeUserIDs) > 0 && !containsInt(cfg.IncludeUserIDs, input.UserID) {
		return false
	}
	if containsInt(cfg.ExcludeTokenIDs, input.TokenID) {
		return false
	}
	if len(cfg.IncludeTokenIDs) > 0 && !containsInt(cfg.IncludeTokenIDs, input.TokenID) {
		return false
	}
	if matchAnyPattern(cfg.ExcludeModelPatterns, input.Model) {
		return false
	}
	if len(cfg.IncludeModelPatterns) > 0 && !matchAnyPattern(cfg.IncludeModelPatterns, input.Model) {
		return false
	}
	if cfg.SampleRate <= 0 {
		return false
	}
	if cfg.SampleRate >= 1 {
		return true
	}
	return stableSample(input) < cfg.SampleRate
}

func stableSample(input DecisionInput) float64 {
	key := input.RequestID + "|" + strconv.Itoa(input.UserID) + "|" + strconv.Itoa(input.TokenID) + "|" + input.Model
	h := fnv.New64a()
	_, _ = h.Write([]byte(key))
	const maxUint64AsFloat = float64(^uint64(0))
	return float64(h.Sum64()) / maxUint64AsFloat
}

func matchAnyPattern(patterns []string, value string) bool {
	if value == "" {
		return false
	}
	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		if matchModelPattern(pattern, value) {
			return true
		}
	}
	return false
}

func matchModelPattern(pattern string, value string) bool {
	if strings.ContainsAny(pattern, "*?") {
		matched, err := filepath.Match(pattern, value)
		if err == nil && matched {
			return true
		}
	}

	re, err := regexp.Compile(pattern)
	if err == nil && re.MatchString(value) {
		return true
	}

	return strings.Contains(value, pattern)
}

func containsInt(values []int, target int) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func envString(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func envBool(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envInt64(key string, fallback int64) int64 {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func envFloat(key string, fallback float64) float64 {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func envIntList(key string, fallback []int) []int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parts := strings.Split(value, ",")
	result := make([]int, 0, len(parts))
	for _, part := range parts {
		parsed, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil {
			continue
		}
		result = append(result, parsed)
	}
	return result
}

func envStringList(key string, fallback []string) []string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func clampSampleRate(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}
