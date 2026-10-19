//go:build unit

package service

import (
	"context"
	"testing"
)

func TestAdminUpdateUserLegacyConcurrencyPreservesExplicitSplitPolicy(t *testing.T) {
	live, requests := 4, 2
	user := &User{
		ID:                 41,
		Email:              "legacy-policy@example.com",
		Role:               RoleUser,
		Status:             StatusActive,
		Concurrency:        8,
		LiveConcurrency:    &live,
		RequestConcurrency: &requests,
	}
	repo := &userRepoStub{user: user}
	svc := &adminServiceImpl{
		userRepo:       repo,
		redeemCodeRepo: &balanceRedeemRepoStub{redeemRepoStub: &redeemRepoStub{}},
	}
	legacy := 11

	updated, err := svc.UpdateUser(context.Background(), user.ID, &UpdateUserInput{Concurrency: &legacy})
	if err != nil {
		t.Fatalf("UpdateUser() error = %v", err)
	}
	if updated.LiveConcurrency == nil || *updated.LiveConcurrency != live {
		t.Fatalf("live policy = %v, want %d", updated.LiveConcurrency, live)
	}
	if updated.RequestConcurrency == nil || *updated.RequestConcurrency != requests {
		t.Fatalf("request policy = %v, want %d", updated.RequestConcurrency, requests)
	}
	if updated.Concurrency != legacy {
		t.Fatalf("shared concurrency = %d, want %d", updated.Concurrency, legacy)
	}
}
