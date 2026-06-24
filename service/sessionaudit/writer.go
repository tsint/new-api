package sessionaudit

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/QuantumNous/new-api/common"
)

const redactedValue = "[REDACTED]"

type WriteInput struct {
	RequestID   string
	SessionID   string
	UserID      int
	Username    string
	TokenID     int
	TokenName   string
	Model       string
	Path        string
	RelayMode   int
	ChannelID   int
	Group       string
	IsStream    bool
	RequestBody []byte
	Headers     http.Header
	Now         time.Time
}

func BuildAuditFilePath(cfg Config, input WriteInput) (string, error) {
	cfg = ApplyForceConstraints(cfg)
	if input.UserID <= 0 {
		return "", fmt.Errorf("invalid user id: %d", input.UserID)
	}

	now := input.Now
	if now.IsZero() {
		now = time.Now()
	}
	sessionID := input.SessionID
	if sessionID == "" {
		sessionID = input.RequestID
	}
	if sessionID == "" {
		sessionID = newSessionID()
	}
	model := input.Model
	if model == "" {
		model = "unknown-model"
	}

	root, err := filepath.Abs(cfg.OutputDir)
	if err != nil {
		return "", err
	}
	userDir := filepath.Join(root, auditUserDirName(input))
	fileName := fmt.Sprintf(
		"%d_%s_%s.jsonl",
		now.UnixMilli(),
		sanitizePathComponent(sessionID),
		sanitizePathComponent(model),
	)
	fullPath := filepath.Join(userDir, fileName)
	cleanPath := filepath.Clean(fullPath)

	if !strings.HasPrefix(cleanPath, userDir+string(os.PathSeparator)) {
		return "", fmt.Errorf("audit file path escapes user directory")
	}
	return cleanPath, nil
}

func WriteRequestAudit(cfg Config, input WriteInput) (string, error) {
	cfg = ApplyForceConstraints(cfg)
	if input.SessionID == "" {
		input.SessionID = input.RequestID
	}
	if input.SessionID == "" {
		input.SessionID = newSessionID()
	}
	path, err := BuildAuditFilePath(cfg, input)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return "", err
	}

	now := input.Now
	if now.IsZero() {
		now = time.Now()
	}
	sessionID := input.SessionID

	event := map[string]any{
		"version":                1,
		"event":                  "request",
		"timestamp":              now.UTC().Format(time.RFC3339Nano),
		"request_id":             input.RequestID,
		"session_id":             sessionID,
		"user_id":                input.UserID,
		"username":               input.Username,
		"token_id":               input.TokenID,
		"token_name":             input.TokenName,
		"model":                  input.Model,
		"path":                   input.Path,
		"relay_mode":             input.RelayMode,
		"channel_id":             input.ChannelID,
		"group":                  input.Group,
		"is_stream":              input.IsStream,
		"request_body_bytes":     len(input.RequestBody),
		"request_body_truncated": false,
	}

	if cfg.CaptureRequestBody {
		body := input.RequestBody
		if cfg.MaxRequestBodyBytes >= 0 && int64(len(body)) > cfg.MaxRequestBodyBytes {
			body = body[:cfg.MaxRequestBodyBytes]
			event["request_body_truncated"] = true
		}
		event["request_body"] = string(body)
	}
	if cfg.CaptureRequestHeaders {
		event["headers"] = sanitizeHeaders(input.Headers, cfg.RedactSensitiveHeaders)
	}

	jsonLine, err := common.Marshal(event)
	if err != nil {
		return "", err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return "", err
	}
	if _, err := file.Write(append(jsonLine, '\n')); err != nil {
		_ = file.Close()
		return "", err
	}
	if err := file.Close(); err != nil {
		return "", err
	}

	if err := CleanupUserAuditLogs(cfg, input, path); err != nil {
		return path, err
	}
	return path, nil
}

func newSessionID() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err == nil {
		return "sess_" + hex.EncodeToString(buf[:])
	}
	return fmt.Sprintf("sess_%d", time.Now().UnixNano())
}

func CleanupUserAuditLogs(cfg Config, input WriteInput, currentFile string) error {
	cfg = ApplyForceConstraints(cfg)
	if input.UserID <= 0 {
		return fmt.Errorf("invalid user id: %d", input.UserID)
	}
	userDir := filepath.Join(cfg.OutputDir, auditUserDirName(input))
	entries, err := os.ReadDir(userDir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	type auditFile struct {
		path    string
		size    int64
		modTime time.Time
	}
	files := make([]auditFile, 0, len(entries))
	now := time.Now()
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		path := filepath.Join(userDir, entry.Name())
		if cfg.RetentionDays > 0 && info.ModTime().Before(now.AddDate(0, 0, -cfg.RetentionDays)) && path != currentFile {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				return err
			}
			continue
		}
		files = append(files, auditFile{path: path, size: info.Size(), modTime: info.ModTime()})
	}

	var total int64
	for _, file := range files {
		total += file.size
	}
	if cfg.MaxBytesPerUser <= 0 || total <= cfg.MaxBytesPerUser {
		return nil
	}

	sort.Slice(files, func(i, j int) bool {
		if files[i].modTime.Equal(files[j].modTime) {
			return files[i].path < files[j].path
		}
		return files[i].modTime.Before(files[j].modTime)
	})
	for _, file := range files {
		if total <= cfg.MaxBytesPerUser {
			break
		}
		if file.path == currentFile {
			continue
		}
		if err := os.Remove(file.path); err != nil && !os.IsNotExist(err) {
			return err
		}
		total -= file.size
	}
	return nil
}

func auditUserDirName(input WriteInput) string {
	username := sanitizePathComponent(input.Username)
	if username != "unknown" {
		return username
	}
	return fmt.Sprintf("user_%d", input.UserID)
}

func sanitizeHeaders(headers http.Header, redact bool) map[string]string {
	result := make(map[string]string, len(headers))
	for key, values := range headers {
		value := strings.Join(values, ",")
		if redact && isSensitiveHeader(key) {
			value = redactedValue
		}
		result[key] = value
	}
	return result
}

func isSensitiveHeader(key string) bool {
	lower := strings.ToLower(key)
	switch lower {
	case "authorization", "x-api-key", "cookie", "set-cookie", "new-api-user":
		return true
	default:
		return strings.Contains(lower, "token") ||
			strings.Contains(lower, "secret") ||
			strings.Contains(lower, "key")
	}
}

func sanitizePathComponent(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	var b strings.Builder
	lastUnderscore := false
	for _, r := range value {
		valid := unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '_' || r == '-'
		if valid {
			b.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}
	result := strings.Trim(b.String(), "._")
	if result == "" {
		return "unknown"
	}
	return result
}
