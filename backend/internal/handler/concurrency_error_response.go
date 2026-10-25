package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

const statusClientClosedRequest = 499

const (
	gatewayQueueFullCode        = "gateway_queue_full"
	gatewayConcurrencyLimitCode = "gateway_concurrency_limit"
	// gatewayConcurrencyCapacityCode matches the code stage's split-concurrency
	// handlers already emit for ConcurrencyCapacityError (see
	// handleConcurrencyCapacityError).
	gatewayConcurrencyCapacityCode = "concurrency_capacity"
)

func concurrencyErrorResponse(err error, slotType string) (int, string, string, string) {
	var capacityErr *service.ConcurrencyCapacityError
	if errors.As(err, &capacityErr) {
		return http.StatusTooManyRequests, "rate_limit_error", gatewayConcurrencyCapacityCode, capacityErr.Error()
	}

	var waitQueueFullErr *WaitQueueFullError
	if errors.As(err, &waitQueueFullErr) {
		return http.StatusTooManyRequests, "rate_limit_error", gatewayQueueFullCode,
			"Too many pending requests, please retry later"
	}

	var concurrencyErr *ConcurrencyError
	if errors.As(err, &concurrencyErr) {
		if concurrencyErr.SlotType != "" {
			slotType = concurrencyErr.SlotType
		}
		return http.StatusTooManyRequests, "rate_limit_error", gatewayConcurrencyLimitCode,
			fmt.Sprintf("Concurrency limit exceeded for %s, please retry later", slotType)
	}

	if errors.Is(err, context.Canceled) {
		return statusClientClosedRequest, "api_error", "", "context canceled"
	}

	return http.StatusServiceUnavailable, "api_error", "", "Service temporarily unavailable, please retry later"
}

func concurrencyCapacityErrorFrom(err error) *service.ConcurrencyCapacityError {
	var capacityErr *service.ConcurrencyCapacityError
	if errors.As(err, &capacityErr) {
		return capacityErr
	}
	return nil
}

func concurrencyCapacityMetadata(capacityErr *service.ConcurrencyCapacityError) map[string]string {
	if capacityErr == nil {
		return nil
	}
	return map[string]string{
		"concurrency_class":  string(capacityErr.Class),
		"concurrency_scope":  capacityErr.Scope,
		"concurrency_reason": capacityErr.Reason,
	}
}
