package relay

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service/sessionaudit"
	"github.com/QuantumNous/new-api/setting/audit_setting"

	"github.com/gin-gonic/gin"
)

const sessionAuditLoggedKey = "session_audit_logged"

func AuditRelayRequest(c *gin.Context, info *relaycommon.RelayInfo, body []byte) {
	if c == nil || info == nil || c.Request == nil || c.Request.URL == nil {
		return
	}
	if c.GetBool(sessionAuditLoggedKey) {
		return
	}
	c.Set(sessionAuditLoggedKey, true)

	cfg := audit_setting.GetEffectiveSessionAuditConfig()
	channelID := 0
	if info.ChannelMeta != nil {
		channelID = info.ChannelMeta.ChannelId
	}
	requestID := c.GetString(common.RequestIdKey)
	if requestID == "" {
		requestID = info.RequestId
	}
	_, audited, err := sessionaudit.AuditRequestIfNeeded(cfg, sessionaudit.WriteInput{
		RequestID:   requestID,
		SessionID:   requestID,
		UserID:      info.UserId,
		Username:    c.GetString("username"),
		TokenID:     info.TokenId,
		TokenName:   c.GetString("token_name"),
		Model:       info.OriginModelName,
		Path:        c.Request.URL.Path,
		RelayMode:   info.RelayMode,
		ChannelID:   channelID,
		Group:       info.UsingGroup,
		IsStream:    info.IsStream,
		RequestBody: body,
		Headers:     cloneHeaders(c.Request.Header),
	})
	if audited && err != nil {
		logger.LogError(c, "failed to write session audit log: "+err.Error())
	}
}

func cloneHeaders(headers http.Header) http.Header {
	clone := make(http.Header, len(headers))
	for key, values := range headers {
		clone[key] = append([]string(nil), values...)
	}
	return clone
}
