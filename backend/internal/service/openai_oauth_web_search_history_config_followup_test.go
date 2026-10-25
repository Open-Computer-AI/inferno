//go:build unit

package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestResponsesCompatibilityOptionsWireWebSearchHistory(t *testing.T) {
	body := []byte(openAIWebSearchHistoryCompactionBody)
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	cfg := &config.Config{}

	t.Run("regular Responses injects top-level tool", func(t *testing.T) {
		out, _, err := normalizeOpenAIResponsesCompatibilityBodyWithOptions(body, account, openAIResponsesCompatibilityOptions{Config: cfg})
		require.NoError(t, err)
		require.Equal(t, "web_search", gjson.GetBytes(out, "tools.0.type").String())
		require.Equal(t, "none", gjson.GetBytes(out, "tool_choice").String())
	})

	t.Run("Responses Lite injects additional_tools", func(t *testing.T) {
		out, _, err := normalizeOpenAIResponsesCompatibilityBodyWithOptions(body, account, openAIResponsesCompatibilityOptions{Config: cfg, ResponsesLite: true})
		require.NoError(t, err)
		require.False(t, gjsonToolsContainWebSearch(gjson.GetBytes(out, "tools")))
		found := false
		for _, item := range gjson.GetBytes(out, "input").Array() {
			if item.Get("type").String() == openAIAdditionalToolsItemType && item.Get("tools.0.type").String() == "web_search" {
				found = true
			}
		}
		require.True(t, found, "history repair should declare web_search in input additional_tools")
	})

	t.Run("compact leaves history wire unchanged", func(t *testing.T) {
		out, _, err := normalizeOpenAIResponsesCompatibilityBodyWithOptions(body, account, openAIResponsesCompatibilityOptions{Config: cfg, Compact: true})
		require.NoError(t, err)
		require.False(t, gjson.GetBytes(out, "tools.0.type").Exists())
		for _, item := range gjson.GetBytes(out, "input").Array() {
			require.NotEqual(t, openAIAdditionalToolsItemType, item.Get("type").String())
		}
	})
}
