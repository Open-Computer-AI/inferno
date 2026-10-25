//go:build integration

package repository

import (
	"encoding/json"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"time"
)

func (s *ProxyExpirySuite) TestSweepProxyFallbackClearsNetworkBoundUsageSnapshots() {
	for _, tc := range []struct {
		name   string
		mode   string
		backup bool
	}{
		{name: "direct fallback", mode: service.FallbackModeDirect},
		{name: "backup proxy fallback", mode: service.FallbackModeProxy, backup: true},
	} {
		s.Run(tc.name, func() {
			now := time.Now()
			past := now.Add(-time.Hour)
			var backupID *int64
			var target *int64
			if tc.backup {
				future := now.Add(24 * time.Hour)
				backup := s.mkProxy("usage-snapshot-backup", service.FallbackModeNone, &future, nil)
				backupID = &backup
				target = &backup
			}
			source := s.mkProxy("usage-snapshot-source", tc.mode, &past, backupID)
			account := s.mkAccountWithProxy(source)
			_, err := s.tx.ExecContext(s.ctx, `UPDATE accounts SET type='apikey',platform='opencode_go',extra='{"upstream_billing_probe":{"status":"ok"},"ollama_cloud_usage_snapshot":{"status":"ok"},"opencode_go_usage_auto_refresh":true,"opencode_go_usage_snapshot":{"status":"ok"},"keep_me":true}'::jsonb WHERE id=$1`, account)
			s.Require().NoError(err)

			changed, err := s.repo.sweepOneExpiredProxy(s.ctx, service.Proxy{ID: source, Status: service.StatusActive, ExpiresAt: &past, FallbackMode: tc.mode, BackupProxyID: backupID}, now, target, true)
			s.Require().NoError(err)
			s.Equal([]int64{account}, changed)

			var raw []byte
			s.Require().NoError(scanSingleRow(s.ctx, s.tx, `SELECT extra FROM accounts WHERE id=$1`, []any{account}, &raw))
			var extra map[string]any
			s.Require().NoError(json.Unmarshal(raw, &extra))
			s.NotContains(extra, "upstream_billing_probe")
			s.NotContains(extra, "ollama_cloud_usage_snapshot")
			s.NotContains(extra, "opencode_go_usage_snapshot")
			s.Equal(true, extra["opencode_go_usage_auto_refresh"])
			s.Equal(true, extra["keep_me"])
			s.Equal(target, s.accountProxyID(account))
		})
	}
}

// Reproduce the ordering deterministically without sleeps: scan snapshot,
// administrator edit, then the per-proxy transaction consuming that snapshot.
func (s *ProxyExpirySuite) TestSweep_SkipsChangedSnapshot() {
	for _, edit := range []string{"renew", "clear expiry", "disable", "change mode", "change backup"} {
		s.Run(edit, func() {
			now := time.Now()
			past := now.Add(-time.Hour)
			future := now.Add(24 * time.Hour)
			backup := s.mkProxy("snapshot-backup", service.FallbackModeNone, &future, nil)
			source := s.mkProxy("snapshot-source", service.FallbackModeDirect, &past, nil)
			if edit == "change backup" {
				_, err := s.tx.ExecContext(s.ctx, `UPDATE proxies SET fallback_mode='proxy', backup_proxy_id=$1 WHERE id=$2`, backup, source)
				s.Require().NoError(err)
			}
			account := s.mkAccountWithProxy(source)
			snapshot, err := s.repo.GetByID(s.ctx, source)
			s.Require().NoError(err)
			target, change := service.ResolveProxyFallbackTarget(*snapshot, map[int64]service.Proxy{
				backup: {ID: backup, Status: service.StatusActive, ExpiresAt: &future},
			}, now)
			s.Require().True(change)
			updated := *snapshot
			switch edit {
			case "renew":
				updated.ExpiresAt = &future
			case "clear expiry":
				updated.ExpiresAt = nil
			case "disable":
				updated.Status = "inactive"
			case "change mode":
				updated.FallbackMode = service.FallbackModeNone
			case "change backup":
				otherBackup := s.mkProxy("replacement-backup", service.FallbackModeNone, &future, nil)
				updated.BackupProxyID = &otherBackup
			}
			s.Require().NoError(s.repo.Update(s.ctx, &updated))
			var eventsBefore int64
			s.Require().NoError(scanSingleRow(s.ctx, s.tx, `SELECT COUNT(*) FROM scheduler_outbox`, nil, &eventsBefore))
			changed, err := s.repo.sweepOneExpiredProxy(s.ctx, *snapshot, now, target, change)
			s.Require().NoError(err)
			s.Empty(changed)
			got, err := s.repo.GetByID(s.ctx, source)
			s.Require().NoError(err)
			s.Equal(updated.Status, got.Status)
			s.Equal(&source, s.accountProxyID(account))
			var origin *int64
			s.Require().NoError(scanSingleRow(s.ctx, s.tx, `SELECT proxy_fallback_origin_id FROM accounts WHERE id=$1`, []any{account}, &origin))
			s.Nil(origin)
			var eventsAfter int64
			s.Require().NoError(scanSingleRow(s.ctx, s.tx, `SELECT COUNT(*) FROM scheduler_outbox`, nil, &eventsAfter))
			s.Equal(eventsBefore, eventsAfter, "stale sweep must not publish account changes")
		})
	}
}

func (s *ProxyExpirySuite) TestSweep_ChangedModeIsUsedOnNextScan() {
	now := time.Now()
	past := now.Add(-time.Hour)
	source := s.mkProxy("mode-source", service.FallbackModeDirect, &past, nil)
	account := s.mkAccountWithProxy(source)
	snapshot, err := s.repo.GetByID(s.ctx, source)
	s.Require().NoError(err)
	updated := *snapshot
	updated.FallbackMode = service.FallbackModeNone
	s.Require().NoError(s.repo.Update(s.ctx, &updated))
	changed, err := s.repo.sweepOneExpiredProxy(s.ctx, *snapshot, now, nil, true)
	s.Require().NoError(err)
	s.Empty(changed)
	_, err = s.repo.SweepExpiredProxies(s.ctx, now)
	s.Require().NoError(err)
	got, err := s.repo.GetByID(s.ctx, source)
	s.Require().NoError(err)
	s.Equal(service.StatusExpired, got.Status)
	s.Equal(&source, s.accountProxyID(account), "new none policy must preserve account binding")
}
