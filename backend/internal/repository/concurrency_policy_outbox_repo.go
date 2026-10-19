package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type concurrencyPolicyOutboxRepository struct {
	db *sql.DB
}

func NewConcurrencyPolicyOutboxRepository(db *sql.DB) service.ConcurrencyPolicyOutboxRepository {
	return &concurrencyPolicyOutboxRepository{db: db}
}

func (r *concurrencyPolicyOutboxRepository) Claim(
	ctx context.Context,
	workerID string,
	limit int,
	lease time.Duration,
) ([]service.ConcurrencyPolicyOutboxEvent, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("nil concurrency policy outbox database")
	}
	if limit <= 0 {
		limit = 100
	}
	leaseSeconds := int64(lease / time.Second)
	if leaseSeconds < 1 {
		leaseSeconds = 30
	}
	rows, err := r.db.QueryContext(ctx, `
		WITH candidates AS (
			SELECT id
			FROM concurrency_policy_outbox
			WHERE published_at IS NULL
			  AND available_at <= NOW()
			  AND (claimed_until IS NULL OR claimed_until < NOW())
			ORDER BY id ASC
			LIMIT $2
			FOR UPDATE SKIP LOCKED
		)
		UPDATE concurrency_policy_outbox AS o
		SET claimed_until = NOW() + ($3 * INTERVAL '1 second'),
		    claimed_by = $1
		FROM candidates AS c
		WHERE o.id = c.id
		RETURNING o.id, o.entity_type, o.entity_id, o.generation,
		          o.shared_concurrency, o.live_concurrency,
		          o.request_concurrency, o.attempts
	`, workerID, limit, leaseSeconds)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	events := make([]service.ConcurrencyPolicyOutboxEvent, 0, limit)
	for rows.Next() {
		var (
			event          service.ConcurrencyPolicyOutboxEvent
			shared         int
			live, requests sql.NullInt64
		)
		if err := rows.Scan(
			&event.ID,
			&event.EntityType,
			&event.EntityID,
			&event.Generation,
			&shared,
			&live,
			&requests,
			&event.Attempts,
		); err != nil {
			return nil, err
		}
		event.Policy.Shared = shared
		if live.Valid {
			value := int(live.Int64)
			event.Policy.Live = &value
		}
		if requests.Valid {
			value := int(requests.Int64)
			event.Policy.Request = &value
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return events, nil
}

func (r *concurrencyPolicyOutboxRepository) MarkPublished(ctx context.Context, id int64, workerID string) error {
	if r == nil || r.db == nil {
		return errors.New("nil concurrency policy outbox database")
	}
	result, err := r.db.ExecContext(ctx, `
		UPDATE concurrency_policy_outbox
		SET published_at = NOW(),
		    claimed_until = NULL,
		    claimed_by = NULL,
		    last_error = NULL
		WHERE id = $1 AND claimed_by = $2 AND published_at IS NULL
	`, id, workerID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return fmt.Errorf("concurrency policy outbox claim %d is no longer owned by %s", id, workerID)
	}
	return nil
}

func (r *concurrencyPolicyOutboxRepository) RetryClaimed(
	ctx context.Context,
	id int64,
	workerID string,
	availableAt time.Time,
	lastError string,
) error {
	if r == nil || r.db == nil {
		return errors.New("nil concurrency policy outbox database")
	}
	result, err := r.db.ExecContext(ctx, `
		UPDATE concurrency_policy_outbox
		SET attempts = attempts + 1,
		    available_at = $3,
		    last_error = $4,
		    claimed_until = NULL,
		    claimed_by = NULL
		WHERE id = $1 AND claimed_by = $2 AND published_at IS NULL
	`, id, workerID, availableAt, lastError)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return fmt.Errorf("concurrency policy outbox claim %d is no longer owned by %s", id, workerID)
	}
	return nil
}

func (r *concurrencyPolicyOutboxRepository) ListCurrent(
	ctx context.Context,
	limit int,
	after *service.ConcurrencyPolicyCursor,
) ([]service.ConcurrencyPolicyOutboxEvent, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("nil concurrency policy outbox database")
	}
	if limit <= 0 {
		limit = 100
	}
	query := `
		SELECT entity_type, entity_id, generation, shared_concurrency,
		       live_concurrency, request_concurrency
		FROM (
			SELECT 'account'::TEXT AS entity_type, id AS entity_id,
			       concurrency_policy_generation AS generation,
			       concurrency AS shared_concurrency,
			       live_concurrency, request_concurrency
			FROM accounts
			UNION ALL
			SELECT 'user'::TEXT AS entity_type, id AS entity_id,
			       concurrency_policy_generation AS generation,
			       concurrency AS shared_concurrency,
			       live_concurrency, request_concurrency
			FROM users
		) AS current_policies`
	args := []any{limit}
	if after != nil {
		query += `
		WHERE current_policies.entity_type > $2
		   OR (current_policies.entity_type = $2 AND current_policies.entity_id > $3)`
		args = append(args, after.EntityType, after.EntityID)
	}
	query += `
		ORDER BY entity_type ASC, entity_id ASC
		LIMIT $1`
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	events := make([]service.ConcurrencyPolicyOutboxEvent, 0, limit)
	for rows.Next() {
		var (
			event          service.ConcurrencyPolicyOutboxEvent
			shared         int
			live, requests sql.NullInt64
		)
		if err := rows.Scan(
			&event.EntityType,
			&event.EntityID,
			&event.Generation,
			&shared,
			&live,
			&requests,
		); err != nil {
			return nil, err
		}
		event.Policy.Shared = shared
		if live.Valid {
			value := int(live.Int64)
			event.Policy.Live = &value
		}
		if requests.Valid {
			value := int(requests.Int64)
			event.Policy.Request = &value
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return events, nil
}
