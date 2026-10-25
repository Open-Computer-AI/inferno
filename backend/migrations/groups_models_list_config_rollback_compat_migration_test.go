package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestGroupsModelsListConfigRollbackCompatMigration pins 915: it only adds the pre-235 column back (same type and
// default as 143) and copies the allowlist into it; it never drops, renames or rewrites model_allowlist.
func TestGroupsModelsListConfigRollbackCompatMigration(t *testing.T) {
	content, err := FS.ReadFile("915_groups_models_list_config_rollback_compat.sql")
	require.NoError(t, err)

	var stmts []string
	for _, line := range strings.Split(string(content), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "--") {
			continue
		}
		stmts = append(stmts, trimmed)
	}
	sql := strings.Join(stmts, " ")
	require.Equal(t, "ALTER TABLE groups ADD COLUMN IF NOT EXISTS models_list_config JSONB NOT NULL DEFAULT '{}'::jsonb; "+
		"UPDATE groups SET models_list_config = model_allowlist "+
		"WHERE models_list_config = '{}'::jsonb AND model_allowlist <> '{}'::jsonb;", sql)

	// The 143 definition the pre-235 images expect.
	original, err := FS.ReadFile("143_group_models_list_config.sql")
	require.NoError(t, err)
	require.Contains(t, string(original), "models_list_config JSONB NOT NULL DEFAULT '{}'::jsonb")
}
