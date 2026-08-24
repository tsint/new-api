package common

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newTestContext(keys map[constant.ContextKey]any) *gin.Context {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	for k, v := range keys {
		common.SetContextKey(c, k, v)
	}
	return c
}

func TestGenBaseRelayInfoTokenGroupFallback(t *testing.T) {
	tests := []struct {
		name string
		keys map[constant.ContextKey]any
		want string
	}{
		{
			name: "token group wins",
			keys: map[constant.ContextKey]any{
				constant.ContextKeyTokenGroup: "gb", constant.ContextKeyUsingGroup: "gb", constant.ContextKeyUserGroup: "ga",
			},
			want: "gb",
		},
		{
			name: "multi-group sentinel preserved",
			keys: map[constant.ContextKey]any{
				constant.ContextKeyTokenGroup: "", constant.ContextKeyUsingGroup: "", constant.ContextKeyUserGroup: "ga",
			},
			want: "",
		},
		{
			name: "single group unchanged",
			keys: map[constant.ContextKey]any{
				constant.ContextKeyTokenGroup: "", constant.ContextKeyUsingGroup: "ga", constant.ContextKeyUserGroup: "ga",
			},
			want: "ga",
		},
		{
			name: "playground group used for retry",
			keys: map[constant.ContextKey]any{
				constant.ContextKeyTokenGroup: "", constant.ContextKeyUsingGroup: "gb", constant.ContextKeyUserGroup: "ga",
			},
			want: "gb",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := genBaseRelayInfo(newTestContext(tt.keys), &dto.GeneralOpenAIRequest{})
			require.Equal(t, tt.want, info.TokenGroup)
		})
	}
}

func TestRelayInfoGetFinalRequestRelayFormatPrefersExplicitFinal(t *testing.T) {
	info := &RelayInfo{
		RelayFormat:             types.RelayFormatOpenAI,
		RequestConversionChain:  []types.RelayFormat{types.RelayFormatOpenAI, types.RelayFormatClaude},
		FinalRequestRelayFormat: types.RelayFormatOpenAIResponses,
	}

	require.Equal(t, types.RelayFormat(types.RelayFormatOpenAIResponses), info.GetFinalRequestRelayFormat())
}

func TestRelayInfoGetFinalRequestRelayFormatFallsBackToConversionChain(t *testing.T) {
	info := &RelayInfo{
		RelayFormat:            types.RelayFormatOpenAI,
		RequestConversionChain: []types.RelayFormat{types.RelayFormatOpenAI, types.RelayFormatClaude},
	}

	require.Equal(t, types.RelayFormat(types.RelayFormatClaude), info.GetFinalRequestRelayFormat())
}

func TestRelayInfoGetFinalRequestRelayFormatFallsBackToRelayFormat(t *testing.T) {
	info := &RelayInfo{
		RelayFormat: types.RelayFormatGemini,
	}

	require.Equal(t, types.RelayFormat(types.RelayFormatGemini), info.GetFinalRequestRelayFormat())
}

func TestRelayInfoGetFinalRequestRelayFormatNilReceiver(t *testing.T) {
	var info *RelayInfo
	require.Equal(t, types.RelayFormat(""), info.GetFinalRequestRelayFormat())
}
