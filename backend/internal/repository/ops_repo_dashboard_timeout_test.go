package repository

import (
	"context"
	"fmt"
	"testing"

	"github.com/lib/pq"
)

func TestIsQueryTimeoutErr(t *testing.T) {
	if !isQueryTimeoutErr(context.DeadlineExceeded) {
		t.Fatalf("context.DeadlineExceeded should be treated as query timeout")
	}
	if !isQueryTimeoutErr(fmt.Errorf("wrapped: %w", context.DeadlineExceeded)) {
		t.Fatalf("wrapped context.DeadlineExceeded should be treated as query timeout")
	}
	if isQueryTimeoutErr(context.Canceled) {
		t.Fatalf("context.Canceled should not be treated as query timeout")
	}
	if isQueryTimeoutErr(fmt.Errorf("wrapped: %w", context.Canceled)) {
		t.Fatalf("wrapped context.Canceled should not be treated as query timeout")
	}
	if !isQueryTimeoutErr(&pq.Error{Code: "57014", Message: "canceling statement due to statement timeout"}) {
		t.Fatalf("PostgreSQL query_canceled should be treated as query timeout")
	}
	if !isQueryTimeoutErr(fmt.Errorf("wrapped: %w", &pq.Error{Code: "57014"})) {
		t.Fatalf("wrapped PostgreSQL query_canceled should be treated as query timeout")
	}
}
