package audit_setting

import (
	"github.com/QuantumNous/new-api/service/sessionaudit"
	"github.com/QuantumNous/new-api/setting/config"
)

type SessionAuditSetting struct {
	Enabled                bool     `json:"session_audit_enabled"`
	SampleRate             float64  `json:"session_audit_sample_rate"`
	RetentionDays          int      `json:"session_audit_retention_days"`
	OutputDir              string   `json:"session_audit_output_dir"`
	MaxBytesPerUser        int64    `json:"session_audit_max_bytes_per_user"`
	MaxRequestBodyBytes    int64    `json:"session_audit_max_request_body_bytes"`
	IncludeUserIDs         []int    `json:"session_audit_include_user_ids"`
	ExcludeUserIDs         []int    `json:"session_audit_exclude_user_ids"`
	IncludeModelPatterns   []string `json:"session_audit_include_model_patterns"`
	ExcludeModelPatterns   []string `json:"session_audit_exclude_model_patterns"`
	IncludeTokenIDs        []int    `json:"session_audit_include_token_ids"`
	ExcludeTokenIDs        []int    `json:"session_audit_exclude_token_ids"`
	CaptureRequestBody     bool     `json:"session_audit_capture_request_body"`
	CaptureRequestHeaders  bool     `json:"session_audit_capture_request_headers"`
	RedactSensitiveHeaders bool     `json:"session_audit_redact_sensitive_headers"`
}

var envSessionAuditConfig = sessionaudit.ConfigFromEnv(sessionaudit.DefaultConfig())
var sessionAuditSetting = DefaultSessionAuditSetting()

func init() {
	config.GlobalConfig.Register("audit_setting", &sessionAuditSetting)
}

func DefaultSessionAuditSetting() SessionAuditSetting {
	cfg := envSessionAuditConfig
	return SessionAuditSetting{
		Enabled:                cfg.Enabled,
		SampleRate:             cfg.SampleRate,
		RetentionDays:          cfg.RetentionDays,
		OutputDir:              cfg.OutputDir,
		MaxBytesPerUser:        cfg.MaxBytesPerUser,
		MaxRequestBodyBytes:    cfg.MaxRequestBodyBytes,
		IncludeUserIDs:         append([]int(nil), cfg.IncludeUserIDs...),
		ExcludeUserIDs:         append([]int(nil), cfg.ExcludeUserIDs...),
		IncludeModelPatterns:   append([]string(nil), cfg.IncludeModelPatterns...),
		ExcludeModelPatterns:   append([]string(nil), cfg.ExcludeModelPatterns...),
		IncludeTokenIDs:        append([]int(nil), cfg.IncludeTokenIDs...),
		ExcludeTokenIDs:        append([]int(nil), cfg.ExcludeTokenIDs...),
		CaptureRequestBody:     cfg.CaptureRequestBody,
		CaptureRequestHeaders:  cfg.CaptureRequestHeaders,
		RedactSensitiveHeaders: cfg.RedactSensitiveHeaders,
	}
}

func GetSessionAuditSetting() *SessionAuditSetting {
	return &sessionAuditSetting
}

func GetEffectiveSessionAuditConfig() sessionaudit.Config {
	return sessionAuditSetting.ToRuntimeConfig(envSessionAuditConfig)
}

func (s SessionAuditSetting) ToRuntimeConfig(env sessionaudit.Config) sessionaudit.Config {
	cfg := env
	cfg.Enabled = s.Enabled
	cfg.SampleRate = s.SampleRate
	cfg.RetentionDays = s.RetentionDays
	cfg.OutputDir = s.OutputDir
	cfg.MaxBytesPerUser = s.MaxBytesPerUser
	cfg.MaxRequestBodyBytes = s.MaxRequestBodyBytes
	cfg.IncludeUserIDs = append([]int(nil), s.IncludeUserIDs...)
	cfg.ExcludeUserIDs = append([]int(nil), s.ExcludeUserIDs...)
	cfg.IncludeModelPatterns = append([]string(nil), s.IncludeModelPatterns...)
	cfg.ExcludeModelPatterns = append([]string(nil), s.ExcludeModelPatterns...)
	cfg.IncludeTokenIDs = append([]int(nil), s.IncludeTokenIDs...)
	cfg.ExcludeTokenIDs = append([]int(nil), s.ExcludeTokenIDs...)
	cfg.CaptureRequestBody = s.CaptureRequestBody
	cfg.CaptureRequestHeaders = s.CaptureRequestHeaders
	cfg.RedactSensitiveHeaders = s.RedactSensitiveHeaders
	return sessionaudit.ApplyForceConstraints(cfg)
}
