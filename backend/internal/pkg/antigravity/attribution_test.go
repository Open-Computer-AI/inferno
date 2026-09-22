package antigravity

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStripClaudeAttribution(t *testing.T) {
	const attribution = "x-anthropic-billing-header: cc_version=2.1.271.4bf; cc_entrypoint=claude-desktop-3p;"
	tests := []struct {
		name string
		text string
		want string
	}{
		{"metadata only", attribution, ""},
		{"metadata with newline", attribution + "\n", ""},
		{"leading whitespace", " \t\n" + attribution, ""},
		{"following instructions", attribution + "\nKeep these instructions.", "Keep these instructions."},
		{"CRLF", attribution + "\r\n  Keep indentation.\n", "  Keep indentation.\n"},
		{"CR", attribution + "\rKeep these instructions.", "Keep these instructions."},
		{"ordinary text", "  Keep these instructions.\n", "  Keep these instructions.\n"},
		{"no colon", "x-anthropic-billing-header keep", "x-anthropic-billing-header keep"},
		{"inline mention", "Explain this metadata: " + attribution, "Explain this metadata: " + attribution},
	}
	for _, tt := range tests {
		require.Equal(t, tt.want, stripClaudeAttribution(tt.text), tt.name)
	}
}

func TestTransformClaudeToGemini_AttributionOnlyAffectsSystem(t *testing.T) {
	const attribution = "x-anthropic-billing-header: cc_version=example;"
	system, err := json.Marshal([]SystemBlock{
		{Type: "text", Text: attribution + "\nKeep this instruction."},
		{Type: "text", Text: "Keep the next block."},
	})
	require.NoError(t, err)
	input := &ClaudeRequest{
		Model:    "gemini-3.8-flash-high",
		System:   system,
		Messages: []ClaudeMessage{{Role: "user", Content: json.RawMessage(`"` + attribution + `"`)}},
		Tools: []ClaudeTool{{
			Name:        "example",
			Description: attribution,
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{"reason": map[string]any{"type": "string", "description": "Reason for calling this tool"}}, "required": []string{"reason"}, "additionalProperties": false},
		}},
	}
	body, err := TransformClaudeToGeminiWithOptions(input, "test-project", input.Model, TransformOptions{})
	require.NoError(t, err)

	var got V1InternalRequest
	require.NoError(t, json.Unmarshal(body, &got))
	var texts []string
	for _, part := range got.Request.SystemInstruction.Parts {
		if part.Text != "\n--- [SYSTEM_PROMPT_END] ---" {
			texts = append(texts, part.Text)
		}
	}
	require.Equal(t, []string{"Keep this instruction.", "Keep the next block."}, texts)
	require.Equal(t, attribution, got.Request.Contents[0].Parts[0].Text)
	require.Equal(t, attribution, got.Request.Tools[0].FunctionDeclarations[0].Description)
	require.False(t, strings.Contains(texts[0], "cc_version="))
}
