package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
)

func TestDataExportUserRankingLimitEnvironmentOverride(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		option   string
		want     int
	}{
		{name: "option value without environment override", option: "20", want: 20},
		{name: "environment overrides option", envValue: "30", option: "20", want: 30},
		{name: "environment value is clamped to minimum", envValue: "0", option: "20", want: 1},
		{name: "invalid environment value falls back to option", envValue: "invalid", option: "20", want: 20},
	}

	originalLimit := common.DataExportUserRankingLimit
	originalOptionMap := common.OptionMap
	t.Cleanup(func() {
		common.DataExportUserRankingLimit = originalLimit
		common.OptionMap = originalOptionMap
	})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("DATA_EXPORT_USER_RANKING_LIMIT", tt.envValue)
			common.OptionMap = make(map[string]string)

			if err := updateOptionMap("DataExportUserRankingLimit", tt.option); err != nil {
				t.Fatalf("updateOptionMap() error = %v", err)
			}
			if common.DataExportUserRankingLimit != tt.want {
				t.Fatalf("DataExportUserRankingLimit = %d, want %d", common.DataExportUserRankingLimit, tt.want)
			}
			if got := common.OptionMap["DataExportUserRankingLimit"]; got != tt.option {
				t.Fatalf("OptionMap value = %q, want %q", got, tt.option)
			}
		})
	}
}
