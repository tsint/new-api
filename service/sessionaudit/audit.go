package sessionaudit

func AuditRequestIfNeeded(cfg Config, input WriteInput) (path string, audited bool, err error) {
	decision := DecisionInput{
		RequestID: input.RequestID,
		UserID:    input.UserID,
		TokenID:   input.TokenID,
		Model:     input.Model,
	}
	if !ShouldAuditSession(cfg, decision) {
		return "", false, nil
	}
	path, err = WriteRequestAudit(cfg, input)
	if err != nil {
		return "", true, err
	}
	return path, true, nil
}
