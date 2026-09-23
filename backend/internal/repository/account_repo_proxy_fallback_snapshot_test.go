//go:build unit

package repository

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestRevertProxyFallbackInvalidatesNetworkBoundSnapshots(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectExec(`(?s)UPDATE accounts SET.*proxy_id IS DISTINCT FROM proxy_fallback_origin_id.*- 'upstream_billing_probe'.*- 'ollama_cloud_usage_snapshot'.*- 'opencode_go_usage_snapshot'`).
		WithArgs(int64(42)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?s)INSERT INTO scheduler_outbox \(event_type, account_id, group_id, payload, dedup_key\).*ON CONFLICT`).
		WithArgs(service.SchedulerOutboxEventAccountChanged, int64(42), nil, nil, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))

	repo := newAccountRepositoryWithSQL(nil, db, nil)
	require.NoError(t, repo.RevertProxyFallback(context.Background(), 42))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSweepProxyFallbackInvalidatesNetworkBoundSnapshots(t *testing.T) {
	for _, tc := range []struct {
		name       string
		mode       string
		backupID   *int64
		target     *int64
		wantBranch string
	}{
		{name: "direct fallback", mode: service.FallbackModeDirect, wantBranch: "proxy_id IS DISTINCT FROM NULL"},
		{name: "backup proxy fallback", mode: service.FallbackModeProxy, backupID: int64Pointer(11), target: int64Pointer(11), wantBranch: "proxy_id IS DISTINCT FROM $2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			t.Cleanup(func() { _ = db.Close() })

			now := time.Date(2026, time.September, 23, 12, 0, 0, 0, time.UTC)
			expiresAt := now.Add(-time.Hour)
			snapshot := service.Proxy{
				ID: 9, Status: service.StatusActive, ExpiresAt: &expiresAt,
				FallbackMode: tc.mode, BackupProxyID: tc.backupID,
			}
			mock.ExpectExec(regexp.QuoteMeta("UPDATE proxies SET status=$1, updated_at=NOW()")).
				WithArgs(service.StatusExpired, int64(9), service.StatusActive, sqlmock.AnyArg(), sqlmock.AnyArg(), tc.mode, tc.backupID).
				WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectQuery(`(?s)UPDATE accounts SET proxy_id=.*` + regexp.QuoteMeta(tc.wantBranch) + `.*- 'upstream_billing_probe'.*- 'ollama_cloud_usage_snapshot'.*- 'opencode_go_usage_snapshot'.*RETURNING id`).
				WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(42)))

			got, err := (&proxyRepository{}).sweepOneExpiredProxyOnExec(context.Background(), db, snapshot, now, tc.target, true)

			require.NoError(t, err)
			require.Equal(t, []int64{42}, got)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func int64Pointer(value int64) *int64 { return &value }
