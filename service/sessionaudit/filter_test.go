package sessionaudit

import "testing"

func baseEnabledConfig() Config {
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.SampleRate = 1
	return cfg
}

func baseInput() DecisionInput {
	return DecisionInput{
		RequestID: "req_abc",
		UserID:    42,
		TokenID:   7,
		Model:     "gpt-4o-mini",
	}
}

func TestShouldAuditSessionHonorsDisabledAndForceDisabled(t *testing.T) {
	cfg := baseEnabledConfig()
	input := baseInput()

	cfg.Enabled = false
	if ShouldAuditSession(cfg, input) {
		t.Fatal("disabled config should not audit")
	}

	cfg.Enabled = true
	cfg.ForceDisabled = true
	if ShouldAuditSession(cfg, input) {
		t.Fatal("force disabled config should not audit")
	}
}

func TestShouldAuditSessionExcludeRulesWinOverIncludeRules(t *testing.T) {
	cfg := baseEnabledConfig()
	cfg.IncludeUserIDs = []int{42}
	cfg.ExcludeUserIDs = []int{42}

	if ShouldAuditSession(cfg, baseInput()) {
		t.Fatal("excluded user should not be audited even when included")
	}

	cfg = baseEnabledConfig()
	cfg.IncludeTokenIDs = []int{7}
	cfg.ExcludeTokenIDs = []int{7}
	if ShouldAuditSession(cfg, baseInput()) {
		t.Fatal("excluded token should not be audited even when included")
	}

	cfg = baseEnabledConfig()
	cfg.IncludeModelPatterns = []string{"^gpt-"}
	cfg.ExcludeModelPatterns = []string{"4o-mini"}
	if ShouldAuditSession(cfg, baseInput()) {
		t.Fatal("excluded model pattern should win over include pattern")
	}
}

func TestShouldAuditSessionRequiresIncludeMatchesWhenIncludeListsAreSet(t *testing.T) {
	cfg := baseEnabledConfig()
	cfg.IncludeUserIDs = []int{99}
	if ShouldAuditSession(cfg, baseInput()) {
		t.Fatal("non-included user should not be audited")
	}

	cfg = baseEnabledConfig()
	cfg.IncludeTokenIDs = []int{99}
	if ShouldAuditSession(cfg, baseInput()) {
		t.Fatal("non-included token should not be audited")
	}

	cfg = baseEnabledConfig()
	cfg.IncludeModelPatterns = []string{"^claude-"}
	if ShouldAuditSession(cfg, baseInput()) {
		t.Fatal("non-matching model should not be audited")
	}
}

func TestShouldAuditSessionSampleRateBoundariesAndStableSampling(t *testing.T) {
	input := baseInput()

	cfg := baseEnabledConfig()
	cfg.SampleRate = 0
	if ShouldAuditSession(cfg, input) {
		t.Fatal("sample rate 0 should not audit")
	}

	cfg.SampleRate = 1
	if !ShouldAuditSession(cfg, input) {
		t.Fatal("sample rate 1 should audit")
	}

	cfg.SampleRate = 0.5
	first := ShouldAuditSession(cfg, input)
	for i := 0; i < 20; i++ {
		if ShouldAuditSession(cfg, input) != first {
			t.Fatal("sampling decision should be stable for the same request")
		}
	}
}

func TestShouldAuditSessionIgnoresInvalidRegexWithoutPanic(t *testing.T) {
	cfg := baseEnabledConfig()
	cfg.ExcludeModelPatterns = []string{"["}
	cfg.IncludeModelPatterns = []string{"^gpt-"}

	if !ShouldAuditSession(cfg, baseInput()) {
		t.Fatal("invalid exclude regex should be treated as non-match")
	}
}

func TestShouldAuditSessionSupportsWildcardModelPatterns(t *testing.T) {
	cfg := baseEnabledConfig()
	cfg.IncludeModelPatterns = []string{"gpt-*-mini"}

	if !ShouldAuditSession(cfg, baseInput()) {
		t.Fatal("wildcard include pattern should match the model")
	}

	cfg = baseEnabledConfig()
	cfg.ExcludeModelPatterns = []string{"gpt-*-mini"}
	if ShouldAuditSession(cfg, baseInput()) {
		t.Fatal("wildcard exclude pattern should match the model")
	}
}
