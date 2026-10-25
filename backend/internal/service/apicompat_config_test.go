package service

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/stretchr/testify/require"
)

func TestChatCompletionsToResponsesUsesConfiguredMinOutputTokens(t *testing.T) {
	maxTokens := 1
	req := &apicompat.ChatCompletionsRequest{
		Model:     "gpt-4o",
		MaxTokens: &maxTokens,
		Messages:  []apicompat.ChatMessage{{Role: "user", Content: json.RawMessage(`"hello"`)}},
	}
	cfg := &config.Config{Gateway: config.GatewayConfig{MinOutputTokens: 32}}
	converted, err := chatCompletionsToResponses(cfg, req)
	require.NoError(t, err)
	require.NotNil(t, converted.MaxOutputTokens)
	require.Equal(t, 32, *converted.MaxOutputTokens)
}
