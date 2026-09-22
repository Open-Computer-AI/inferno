package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGroupModelAllowlistRepairMigration(t *testing.T) {
	content, err := FS.ReadFile("236_group_model_allowlist_repair.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")

	// Either partially migrated shape must retain both compatibility columns.
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS model_allowlist JSONB NOT NULL DEFAULT '{}'::jsonb")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS models_list_config JSONB NOT NULL DEFAULT '{}'::jsonb")
	require.Contains(t, sql, "ALTER TABLE groups ALTER COLUMN model_allowlist SET NOT NULL")
	require.Contains(t, sql, "ALTER TABLE groups ALTER COLUMN models_list_config SET NOT NULL")
	require.Contains(t, sql, "COMMENT ON COLUMN groups.model_allowlist")

	// 235 用 table_schema = 'public' 判定列是否存在，而 ALTER TABLE 走的是 search_path；
	// 修复迁移必须用 regclass 解析，两者才不会在非 public schema 上分叉。
	require.NotContains(t, sql, "table_schema = 'public'")
	require.Contains(t, sql, "to_regclass('groups')")
}
