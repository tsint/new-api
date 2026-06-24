package sessionaudit

import (
	"bufio"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
)

func TestBuildAuditFilePathSanitizesSessionAndModel(t *testing.T) {
	cfg := DefaultConfig()
	cfg.OutputDir = t.TempDir()

	input := WriteInput{
		SessionID: "../req/../../evil",
		UserID:    42,
		Username:  "../huang dc/../../evil",
		Model:     "../gpt:4o mini",
		Now:       time.UnixMilli(1782144000123),
	}

	path, err := BuildAuditFilePath(cfg, input)
	if err != nil {
		t.Fatalf("BuildAuditFilePath error: %v", err)
	}

	wantDir := filepath.Join(cfg.OutputDir, "huang_dc_.._.._evil")
	if filepath.Dir(path) != wantDir {
		t.Fatalf("dir = %q, want %q", filepath.Dir(path), wantDir)
	}
	if !strings.HasPrefix(filepath.Base(path), "1782144000123_req_.._.._evil_") {
		t.Fatalf("base = %q, want sanitized session prefix", filepath.Base(path))
	}
	if strings.Contains(path, ".."+string(os.PathSeparator)) {
		t.Fatalf("path contains traversal segment: %q", path)
	}
	if !strings.HasSuffix(path, "_gpt_4o_mini.jsonl") {
		t.Fatalf("base = %q, want sanitized model suffix", filepath.Base(path))
	}
}

func TestBuildAuditFilePathFallsBackToUserIDWhenUsernameMissing(t *testing.T) {
	cfg := DefaultConfig()
	cfg.OutputDir = t.TempDir()

	path, err := BuildAuditFilePath(cfg, WriteInput{
		SessionID: "req_abc",
		UserID:    42,
		Model:     "gpt-4o",
		Now:       time.UnixMilli(1782144000123),
	})
	if err != nil {
		t.Fatalf("BuildAuditFilePath error: %v", err)
	}

	wantDir := filepath.Join(cfg.OutputDir, "user_42")
	if filepath.Dir(path) != wantDir {
		t.Fatalf("dir = %q, want fallback user id dir %q", filepath.Dir(path), wantDir)
	}
}

func TestWriteRequestAuditWritesJSONLWithTruncatedBodyAndRedactedHeaders(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.SampleRate = 1
	cfg.OutputDir = t.TempDir()
	cfg.MaxRequestBodyBytes = 12
	cfg.CaptureRequestHeaders = true

	headers := http.Header{}
	headers.Set("Authorization", "Bearer secret")
	headers.Set("X-Trace", "trace-id")
	headers.Set("X-Custom-Token", "secret-token")

	input := WriteInput{
		RequestID:   "req_abc",
		SessionID:   "req_abc",
		UserID:      42,
		Username:    "huangdc",
		TokenID:     7,
		TokenName:   "prod-key",
		Model:       "gpt-4o-mini",
		Path:        "/v1/chat/completions",
		RelayMode:   1,
		ChannelID:   12,
		Group:       "default",
		IsStream:    true,
		RequestBody: []byte(`{"messages":[{"role":"user","content":"hello"}]}`),
		Headers:     headers,
		Now:         time.UnixMilli(1782144000123),
	}

	path, err := WriteRequestAudit(cfg, input)
	if err != nil {
		t.Fatalf("WriteRequestAudit error: %v", err)
	}
	if filepath.Base(filepath.Dir(path)) != "huangdc" {
		t.Fatalf("audit dir = %q, want username dir huangdc", filepath.Dir(path))
	}

	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open audit file: %v", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		t.Fatal("audit file should contain one JSONL line")
	}
	var event map[string]any
	if err := common.Unmarshal([]byte(scanner.Text()), &event); err != nil {
		t.Fatalf("unmarshal JSONL event: %v", err)
	}

	if event["event"] != "request" || event["request_id"] != "req_abc" || event["user_id"].(float64) != 42 {
		t.Fatalf("unexpected event metadata: %#v", event)
	}
	if event["request_body"] != `{"messages":` {
		t.Fatalf("request_body = %#v, want truncated first 12 bytes", event["request_body"])
	}
	if event["request_body_truncated"] != true {
		t.Fatalf("request_body_truncated = %#v, want true", event["request_body_truncated"])
	}
	if event["request_body_bytes"].(float64) <= 12 {
		t.Fatalf("request_body_bytes = %#v, want original size", event["request_body_bytes"])
	}

	eventHeaders, ok := event["headers"].(map[string]any)
	if !ok {
		t.Fatalf("headers missing or invalid: %#v", event["headers"])
	}
	if eventHeaders["Authorization"] != "[REDACTED]" {
		t.Fatalf("Authorization header not redacted: %#v", eventHeaders["Authorization"])
	}
	if eventHeaders["X-Custom-Token"] != "[REDACTED]" {
		t.Fatalf("token-like header not redacted: %#v", eventHeaders["X-Custom-Token"])
	}
	if eventHeaders["X-Trace"] != "trace-id" {
		t.Fatalf("X-Trace header = %#v, want trace-id", eventHeaders["X-Trace"])
	}
}

func TestWriteRequestAuditOmitsBodyWhenDisabled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.SampleRate = 1
	cfg.OutputDir = t.TempDir()
	cfg.CaptureRequestBody = false

	path, err := WriteRequestAudit(cfg, WriteInput{
		RequestID:   "req_no_body",
		SessionID:   "req_no_body",
		UserID:      1,
		Model:       "gpt-4o",
		RequestBody: []byte(`{"secret":"value"}`),
		Now:         time.UnixMilli(1782144000123),
	})
	if err != nil {
		t.Fatalf("WriteRequestAudit error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read audit file: %v", err)
	}
	var event map[string]any
	if err := common.Unmarshal([]byte(strings.TrimSpace(string(data))), &event); err != nil {
		t.Fatalf("unmarshal event: %v", err)
	}
	if _, ok := event["request_body"]; ok {
		t.Fatalf("request_body should be omitted when capture disabled: %#v", event)
	}
	if event["request_body_bytes"].(float64) != float64(len(`{"secret":"value"}`)) {
		t.Fatalf("request_body_bytes = %#v, want original body size", event["request_body_bytes"])
	}
}

func TestWriteRequestAuditGeneratesSessionIDWhenMissing(t *testing.T) {
	cfg := DefaultConfig()
	cfg.OutputDir = t.TempDir()
	cfg.CaptureRequestBody = false

	path, err := WriteRequestAudit(cfg, WriteInput{
		UserID: 1,
		Model:  "gpt-4o",
		Now:    time.UnixMilli(1782144000123),
	})
	if err != nil {
		t.Fatalf("WriteRequestAudit error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read audit file: %v", err)
	}
	var event map[string]any
	if err := common.Unmarshal([]byte(strings.TrimSpace(string(data))), &event); err != nil {
		t.Fatalf("unmarshal event: %v", err)
	}
	sessionID, ok := event["session_id"].(string)
	if !ok || sessionID == "" || sessionID == "session" {
		t.Fatalf("session_id = %#v, want generated non-empty id", event["session_id"])
	}
	if !strings.Contains(filepath.Base(path), sessionID) {
		t.Fatalf("path %q should contain generated session id %q", path, sessionID)
	}
}

func TestCleanupUserAuditLogsDeletesOnlyOldestFilesForSameUser(t *testing.T) {
	cfg := DefaultConfig()
	cfg.OutputDir = t.TempDir()
	cfg.MaxBytesPerUser = 30

	userDir := filepath.Join(cfg.OutputDir, "huangdc")
	otherUserDir := filepath.Join(cfg.OutputDir, "user_99")
	if err := os.MkdirAll(userDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(otherUserDir, 0755); err != nil {
		t.Fatal(err)
	}

	oldFile := writeSizedFile(t, filepath.Join(userDir, "100_old.jsonl"), 20)
	currentFile := writeSizedFile(t, filepath.Join(userDir, "200_current.jsonl"), 20)
	otherUserFile := writeSizedFile(t, filepath.Join(otherUserDir, "050_other.jsonl"), 20)

	oldTime := time.Now().Add(-2 * time.Hour)
	currentTime := time.Now()
	_ = os.Chtimes(oldFile, oldTime, oldTime)
	_ = os.Chtimes(currentFile, currentTime, currentTime)
	_ = os.Chtimes(otherUserFile, oldTime, oldTime)

	if err := CleanupUserAuditLogs(cfg, WriteInput{UserID: 42, Username: "huangdc"}, currentFile); err != nil {
		t.Fatalf("CleanupUserAuditLogs error: %v", err)
	}

	if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
		t.Fatalf("oldest same-user file should be deleted, stat err=%v", err)
	}
	if _, err := os.Stat(currentFile); err != nil {
		t.Fatalf("current file should remain: %v", err)
	}
	if _, err := os.Stat(otherUserFile); err != nil {
		t.Fatalf("other user file should not be deleted: %v", err)
	}
}

func writeSizedFile(t *testing.T, path string, size int) string {
	t.Helper()
	if err := os.WriteFile(path, []byte(strings.Repeat("x", size)), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}
