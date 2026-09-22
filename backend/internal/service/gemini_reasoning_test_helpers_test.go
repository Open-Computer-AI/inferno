//go:build unit

package service

// geminiSignalTestAccount is the minimal API-key account fixture needed by the
// reasoning-effort pricing tests. The broader Gemini response-signal fixture
// is not part of this candidate, so keep this dependency local to the lane.
func geminiSignalTestAccount() *Account {
	return &Account{
		ID:       703,
		Name:     "gemini-reasoning-test",
		Platform: PlatformGemini,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key": "test-key",
		},
	}
}
