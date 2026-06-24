package middleware

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
)

func TestDeriveFormatGroupFromPathTreatsResponsesAsOpenAICompatible(t *testing.T) {
	tests := []string{
		"/v1/responses",
		"/v1/responses/compact",
	}

	for _, path := range tests {
		t.Run(path, func(t *testing.T) {
			if got := deriveFormatGroupFromPath(path); got != common.FormatGroupOpenAI {
				t.Fatalf("deriveFormatGroupFromPath(%q) = %v, want %v", path, got, common.FormatGroupOpenAI)
			}
		})
	}
}

func TestDeriveFormatGroupFromPathDoesNotTreatOpenAIModelsAsGemini(t *testing.T) {
	tests := []struct {
		path string
		want common.APIFormatGroup
	}{
		{path: "/v1/models/gpt-4o:generateContent", want: common.FormatGroupOther},
		{path: "/v1beta/models/gemini-2.0-flash:generateContent", want: common.FormatGroupGemini},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := deriveFormatGroupFromPath(tt.path); got != tt.want {
				t.Fatalf("deriveFormatGroupFromPath(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}
