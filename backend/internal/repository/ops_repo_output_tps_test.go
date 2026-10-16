package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestQueryOutputTPS(t *testing.T) {
	db, mock := newSQLMock(t)
	start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	groupID := int64(7)
	mock.ExpectQuery(`(?s)percentile_cont\(0.05\).*ul.output_tokens > 0.*ul.duration_ms > 0.*COALESCE\(ul.image_count, 0\) = 0.*COALESCE\(ul.image_output_tokens, 0\) = 0.*COALESCE\(ul.billing_mode, ''\) <> 'image'.*ul.request_type IN \(1, 2, 3, 4\)`).
		WithArgs(start, end, groupID, "openai").
		WillReturnRows(sqlmock.NewRows([]string{"p5", "p10", "p50", "avg", "sample_count"}).AddRow(2.35, 3.7, 55.0, 277.75, 4))
	repo := &opsRepository{db: db}
	got, err := repo.queryOutputTPS(context.Background(), &service.OpsDashboardFilter{
		StartTime: start, EndTime: end, Platform: " OpenAI ", GroupID: &groupID,
	})
	require.NoError(t, err)
	require.NotNil(t, got)
	require.EqualValues(t, 4, got.SampleCount)
	require.InDelta(t, 2.35, *got.P5, 1e-9)
	require.InDelta(t, 3.7, *got.P10, 1e-9)
	require.InDelta(t, 55.0, *got.P50, 1e-9)
	require.InDelta(t, 277.75, *got.Avg, 1e-9)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestQueryOutputTPSZeroSamples(t *testing.T) {
	db, mock := newSQLMock(t)
	start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	mock.ExpectQuery(`(?s)percentile_cont\(0.05\).*COALESCE\(ul.image_count, 0\) = 0.*COALESCE\(ul.image_output_tokens, 0\) = 0`).
		WithArgs(start, end).
		WillReturnRows(sqlmock.NewRows([]string{"p5", "p10", "p50", "avg", "sample_count"}).AddRow(nil, nil, nil, nil, int64(0)))
	repo := &opsRepository{db: db}
	got, err := repo.queryOutputTPS(context.Background(), &service.OpsDashboardFilter{StartTime: start, EndTime: end})
	require.NoError(t, err)
	require.NotNil(t, got)
	require.EqualValues(t, 0, got.SampleCount)
	require.Nil(t, got.P5)
	require.Nil(t, got.P10)
	require.Nil(t, got.P50)
	require.Nil(t, got.Avg)
	require.NoError(t, mock.ExpectationsWereMet())
}
