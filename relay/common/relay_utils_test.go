package common

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/require"
)

func TestGetFullRequestURL_DefaultBehavior(t *testing.T) {
	tests := []struct {
		name        string
		baseURL     string
		requestURL  string
		channelType int
		expected    string
	}{
		{
			name:        "standard OpenAI URL",
			baseURL:     "https://api.openai.com",
			requestURL:  "/v1/chat/completions",
			channelType: constant.ChannelTypeOpenAI,
			expected:    "https://api.openai.com/v1/chat/completions",
		},
		{
			name:        "custom base URL without v1",
			baseURL:     "https://custom.api.com/v4",
			requestURL:  "/v1/chat/completions",
			channelType: constant.ChannelTypeOpenAI,
			expected:    "https://custom.api.com/v4/v1/chat/completions",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetFullRequestURL(tt.baseURL, tt.requestURL, tt.channelType)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestGetFullRequestURL_CloudflareGateway(t *testing.T) {
	tests := []struct {
		name        string
		baseURL     string
		requestURL  string
		channelType int
		expected    string
	}{
		{
			name:        "Cloudflare Gateway with OpenAI strips v1",
			baseURL:     "https://gateway.ai.cloudflare.com/v1/openai",
			requestURL:  "/v1/chat/completions",
			channelType: constant.ChannelTypeOpenAI,
			expected:    "https://gateway.ai.cloudflare.com/v1/openai/chat/completions",
		},
		{
			name:        "Cloudflare Gateway with Azure strips deployments prefix",
			baseURL:     "https://gateway.ai.cloudflare.com/v1/azure",
			requestURL:  "/openai/deployments/gpt-4/chat/completions",
			channelType: constant.ChannelTypeAzure,
			expected:    "https://gateway.ai.cloudflare.com/v1/azure/gpt-4/chat/completions",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetFullRequestURL(tt.baseURL, tt.requestURL, tt.channelType)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestGetFullRequestURL_SkipV1Prefix(t *testing.T) {
	tests := []struct {
		name        string
		baseURL     string
		requestURL  string
		channelType int
		skipV1      bool
		expected    string
	}{
		{
			name:        "skip v1 prefix enabled strips /v1 from request path",
			baseURL:     "https://custom.api.com/v4",
			requestURL:  "/v1/chat/completions",
			channelType: constant.ChannelTypeOpenAI,
			skipV1:      true,
			expected:    "https://custom.api.com/v4/chat/completions",
		},
		{
			name:        "skip v1 prefix disabled keeps /v1 in request path",
			baseURL:     "https://custom.api.com/v4",
			requestURL:  "/v1/chat/completions",
			channelType: constant.ChannelTypeOpenAI,
			skipV1:      false,
			expected:    "https://custom.api.com/v4/v1/chat/completions",
		},
		{
			name:        "skip v1 prefix with standard OpenAI base URL",
			baseURL:     "https://api.openai.com",
			requestURL:  "/v1/chat/completions",
			channelType: constant.ChannelTypeOpenAI,
			skipV1:      true,
			expected:    "https://api.openai.com/chat/completions",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requestURL := tt.requestURL
			if tt.skipV1 {
				requestURL = StripV1Prefix(requestURL)
			}
			result := GetFullRequestURL(tt.baseURL, requestURL, tt.channelType)
			require.Equal(t, tt.expected, result)
		})
	}
}
