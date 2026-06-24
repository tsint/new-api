package sessionaudit

import (
	"os"
	"testing"
	"time"
)

func TestAuditRequestIfNeededWritesOnlyWhenDecisionMatches(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.SampleRate = 1
	cfg.OutputDir = t.TempDir()
	cfg.IncludeUserIDs = []int{42}

	path, audited, err := AuditRequestIfNeeded(cfg, WriteInput{
		RequestID:   "req_match",
		SessionID:   "req_match",
		UserID:      42,
		TokenID:     7,
		Model:       "gpt-4o",
		RequestBody: []byte(`{"model":"gpt-4o"}`),
		Now:         time.UnixMilli(1782144000123),
	})
	if err != nil {
		t.Fatalf("AuditRequestIfNeeded error: %v", err)
	}
	if !audited || path == "" {
		t.Fatalf("audited=%v path=%q, want audited with path", audited, path)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("audit file should exist: %v", err)
	}

	path, audited, err = AuditRequestIfNeeded(cfg, WriteInput{
		RequestID:   "req_skip",
		SessionID:   "req_skip",
		UserID:      99,
		TokenID:     7,
		Model:       "gpt-4o",
		RequestBody: []byte(`{"model":"gpt-4o"}`),
		Now:         time.UnixMilli(1782144000124),
	})
	if err != nil {
		t.Fatalf("skipped audit should not error: %v", err)
	}
	if audited || path != "" {
		t.Fatalf("audited=%v path=%q, want skipped without path", audited, path)
	}
}
