//go:build unit

package service

import (
	"encoding/json"
	"os"
	"regexp"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestBuildPlatformCatalogMirrorsPlatformListAndProfiles(t *testing.T) {
	catalog := BuildPlatformCatalog()
	require.Equal(t, domain.CompositePrecedencePlatformIDs(), catalog.CompositePrecedence)
	require.Len(t, catalog.Platforms, len(domain.ConcretePlatformIDs()))

	for i, entry := range catalog.Platforms {
		require.Equal(t, domain.ConcretePlatformIDs()[i], entry.ID)
		profile := LookupProviderProfile(entry.ID)
		if profile == nil {
			require.Nil(t, entry.MultiProtocol, entry.ID)
			continue
		}
		require.NotNil(t, entry.MultiProtocol, entry.ID)
		require.Equal(t, profile.DefaultMode, entry.MultiProtocol.DefaultMode, entry.ID)
		require.Equal(t, profile.DefaultMode, entry.MultiProtocol.Modes[0].Mode, entry.ID)
		require.Equal(t, profile.ResponsesPath, entry.MultiProtocol.ResponsesPath, entry.ID)
		require.Len(t, entry.MultiProtocol.Modes, len(profile.Modes), entry.ID)
		for _, mode := range entry.MultiProtocol.Modes {
			endpoints := profile.Modes[mode.Mode]
			require.Equal(t, endpoints.BaseURLs, mode.BaseURLs, "%s/%s", entry.ID, mode.Mode)
			require.Equal(t, len(endpoints.ProtocolRules), len(mode.ProtocolRules), "%s/%s", entry.ID, mode.Mode)
			for j, rule := range endpoints.ProtocolRules {
				require.Equal(t, rule, mode.ProtocolRules[j])
			}
		}
	}
}

func TestBuildPlatformCatalogJSONShape(t *testing.T) {
	raw, err := json.Marshal(BuildPlatformCatalog())
	require.NoError(t, err)

	type modeJSON struct {
		Mode          string            `json:"mode"`
		BaseURLs      map[string]string `json:"base_urls"`
		ProtocolRules []ProtocolRule    `json:"protocol_rules"`
	}
	type platformJSON struct {
		ID            string `json:"id"`
		DisplayName   string `json:"display_name"`
		Gateway       string `json:"gateway"`
		CNProvider    *bool  `json:"cn_provider"`
		MultiProtocol *struct {
			DefaultMode   string     `json:"default_mode"`
			Routing       string     `json:"routing"`
			ResponsesPath string     `json:"responses_path"`
			Modes         []modeJSON `json:"modes"`
		} `json:"multi_protocol"`
	}
	var decoded struct {
		Platforms           []platformJSON `json:"platforms"`
		CompositePrecedence []string       `json:"composite_precedence"`
	}
	require.NoError(t, json.Unmarshal(raw, &decoded))
	require.Equal(t, domain.CompositePrecedencePlatformIDs(), decoded.CompositePrecedence)
	byID := map[string]platformJSON{}
	for _, p := range decoded.Platforms {
		byID[p.ID] = p
	}

	anthropic := byID[PlatformAnthropic]
	require.Equal(t, "Anthropic", anthropic.DisplayName)
	require.Equal(t, "anthropic", anthropic.Gateway)
	require.NotNil(t, anthropic.CNProvider)
	require.False(t, *anthropic.CNProvider)
	require.Nil(t, anthropic.MultiProtocol)
	require.NotContains(t, string(raw), `"multi_protocol":null`)

	deepseek := byID[PlatformDeepseek].MultiProtocol
	require.NotNil(t, deepseek)
	require.Equal(t, "by_inbound", deepseek.Routing)
	require.Equal(t, "/responses", deepseek.ResponsesPath)

	openCode := byID[PlatformOpenCodeGo].MultiProtocol
	require.NotNil(t, openCode)
	require.Equal(t, "by_model", openCode.Routing)
	require.Equal(t, AccountModeGo, openCode.DefaultMode)
	require.Len(t, openCode.Modes, 2)
	require.Equal(t, AccountModeGo, openCode.Modes[0].Mode)
	require.Equal(t, DefaultOpenCodeGoBaseURL, openCode.Modes[0].BaseURLs[APIProtocolChatCompletions])
	require.Equal(t, DefaultOpenCodeGoProtocolRules(), openCode.Modes[0].ProtocolRules)
	require.Equal(t, AccountModeZen, openCode.Modes[1].Mode)
}

// 下发的是副本：调用方修改结果不影响内置 profile。
func TestBuildPlatformCatalogReturnsCopies(t *testing.T) {
	catalog := BuildPlatformCatalog()
	for _, entry := range catalog.Platforms {
		if entry.MultiProtocol == nil {
			continue
		}
		for _, mode := range entry.MultiProtocol.Modes {
			for protocol := range mode.BaseURLs {
				mode.BaseURLs[protocol] = "mutated"
			}
			for i := range mode.ProtocolRules {
				mode.ProtocolRules[i].Protocol = "mutated"
			}
		}
	}
	require.Equal(t, DefaultOpenCodeGoBaseURL, LookupProviderProfile(PlatformOpenCodeGo).DefaultBaseURL(AccountModeGo, APIProtocolChatCompletions))
	require.NotEqual(t, "mutated", LookupProviderProfile(PlatformOpenCodeGo).Modes[AccountModeGo].ProtocolRules[0].Protocol)
}

// Inferno's selectors intentionally use a scoped catalog rather than mirroring every
// upstream/backend platform profile. Keep every concrete platform selectable in the
// Inferno UI resolvable by the backend, without comparing against pristine frontend/.
func TestInfernoFrontendConcretePlatformOptionsExistInBackendCatalog(t *testing.T) {
	const path = "../../../inferno-frontend/src/constants/platforms.ts"
	source, err := os.ReadFile(path)
	require.NoError(t, err)

	declaration := regexp.MustCompile(`(?s)export const CONCRETE_PLATFORM_OPTIONS\s*=\s*\[(.*?)\]\s*as const`)
	match := declaration.FindSubmatch(source)
	require.Len(t, match, 2, "Inferno concrete platform selector list not found")
	valuePattern := regexp.MustCompile(`\{\s*value:\s*'([^']+)'`)
	frontendValues := valuePattern.FindAllSubmatch(match[1], -1)
	require.NotEmpty(t, frontendValues, "Inferno concrete platform selector list is empty")

	backendIDs := make(map[string]struct{})
	for _, platform := range BuildPlatformCatalog().Platforms {
		backendIDs[platform.ID] = struct{}{}
	}
	seen := make(map[string]struct{}, len(frontendValues))
	for _, entry := range frontendValues {
		id := string(entry[1])
		_, duplicate := seen[id]
		require.False(t, duplicate, "duplicate Inferno concrete platform %q", id)
		seen[id] = struct{}{}
		_, supported := backendIDs[id]
		require.True(t, supported, "Inferno concrete platform %q is missing from the backend catalog", id)
	}

	groupOptions := regexp.MustCompile(`(?s)export const GROUP_PLATFORM_OPTIONS\s*=\s*\[.*?\{\s*value:\s*'composite'`)
	require.Regexp(t, groupOptions, source, "Inferno group platform selectors must include composite")
}
