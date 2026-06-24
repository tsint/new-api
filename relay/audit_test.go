package relay

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/audit_setting"

	"github.com/gin-gonic/gin"
)

func TestAuditRelayRequestWritesAtMostOncePerGinContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	setting := audit_setting.GetSessionAuditSetting()
	original := *setting
	t.Cleanup(func() {
		*setting = original
	})

	*setting = audit_setting.DefaultSessionAuditSetting()
	setting.Enabled = true
	setting.SampleRate = 1
	setting.OutputDir = dir
	setting.MaxBytesPerUser = 10 * 1024 * 1024
	setting.MaxRequestBodyBytes = 1024
	setting.CaptureRequestBody = true

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	req.Header.Set("Authorization", "Bearer secret")
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req
	c.Set(common.RequestIdKey, "req_audit_once")
	c.Set("username", "alice")
	c.Set("token_name", "prod")

	info := &relaycommon.RelayInfo{
		RequestId:       "req_audit_once",
		UserId:          42,
		TokenId:         7,
		OriginModelName: "gpt-4o-mini",
		UsingGroup:      "default",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId: 3,
		},
	}

	AuditRelayRequest(c, info, []byte(`{"model":"gpt-4o-mini"}`))
	AuditRelayRequest(c, info, []byte(`{"model":"gpt-4o-mini","retry":true}`))

	files, err := filepath.Glob(filepath.Join(dir, "alice", "*.jsonl"))
	if err != nil {
		t.Fatalf("glob audit files: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected one audit file, got %d: %v", len(files), files)
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatalf("read audit file: %v", err)
	}
	if string(data) == "" {
		t.Fatal("audit file should not be empty")
	}
}

func TestAuditRelayRequestHandlesMissingChannelMeta(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	setting := audit_setting.GetSessionAuditSetting()
	original := *setting
	t.Cleanup(func() {
		*setting = original
	})

	*setting = audit_setting.DefaultSessionAuditSetting()
	setting.Enabled = true
	setting.SampleRate = 1
	setting.OutputDir = dir
	setting.MaxBytesPerUser = 10 * 1024 * 1024
	setting.MaxRequestBodyBytes = 1024
	setting.CaptureRequestBody = true

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req
	c.Set(common.RequestIdKey, "req_missing_channel_meta")

	info := &relaycommon.RelayInfo{
		RequestId:       "req_missing_channel_meta",
		UserId:          42,
		TokenId:         7,
		OriginModelName: "gpt-4o-mini",
		UsingGroup:      "default",
	}

	AuditRelayRequest(c, info, []byte(`{"model":"gpt-4o-mini"}`))

	files, err := filepath.Glob(filepath.Join(dir, "user_42", "*.jsonl"))
	if err != nil {
		t.Fatalf("glob audit files: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected one audit file, got %d: %v", len(files), files)
	}
}
