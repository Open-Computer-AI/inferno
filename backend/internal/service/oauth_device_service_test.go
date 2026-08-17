package service

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestRequestCodeReturnsEveryRequiredField(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	clients := NewOAuthClientService(client)
	svc := NewOAuthDeviceService(client, "https://portal.example.com")

	oc, err := clients.RegisterSelfHosted(ctx, 1, 42, "https://agent.example.com", "")
	if err != nil {
		t.Fatalf("RegisterSelfHosted: %v", err)
	}

	grant, err := svc.RequestCode(ctx, oc.ClientID, "inference")
	if err != nil {
		t.Fatalf("RequestCode: %v", err)
	}

	// hermes_cli/auth.py hard-fails if ANY of these is missing.
	if grant.DeviceCode == "" {
		t.Error("missing device_code")
	}
	if grant.UserCode == "" {
		t.Error("missing user_code")
	}
	if grant.VerificationURI == "" {
		t.Error("missing verification_uri")
	}
	if grant.VerificationURIComplete == "" {
		t.Error("missing verification_uri_complete")
	}
	if grant.ExpiresIn <= 0 {
		t.Error("missing/invalid expires_in")
	}
	if grant.Interval <= 0 {
		t.Error("missing/invalid interval")
	}
	if !strings.Contains(grant.VerificationURIComplete, grant.UserCode) {
		t.Errorf("verification_uri_complete %q should embed user_code %q",
			grant.VerificationURIComplete, grant.UserCode)
	}
}

func TestUserCodeAvoidsAmbiguousCharacters(t *testing.T) {
	for _, bad := range []string{"0", "O", "1", "I", "L"} {
		if strings.Contains(UserCodeAlphabet, bad) {
			t.Errorf("user code alphabet must not contain %q — it is read aloud and typed", bad)
		}
	}
}

func TestApproveMarksTheRowApproved(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	clients := NewOAuthClientService(client)
	svc := NewOAuthDeviceService(client, "https://portal.example.com")

	oc, err := clients.RegisterSelfHosted(ctx, 1, 42, "https://agent.example.com", "")
	if err != nil {
		t.Fatalf("RegisterSelfHosted: %v", err)
	}
	grant, err := svc.RequestCode(ctx, oc.ClientID, "inference")
	if err != nil {
		t.Fatalf("RequestCode: %v", err)
	}

	if err := svc.Approve(ctx, grant.UserCode, 42); err != nil {
		t.Fatalf("Approve: %v", err)
	}

	row, err := svc.byDeviceCode(ctx, grant.DeviceCode)
	if err != nil {
		t.Fatalf("byDeviceCode: %v", err)
	}
	if row.Status != "approved" {
		t.Fatalf("expected approved, got %q", row.Status)
	}
	if row.ApprovedUserID == nil || *row.ApprovedUserID != 42 {
		t.Fatalf("expected approved_user_id=42, got %v", row.ApprovedUserID)
	}
}

func TestDenyMarksTheRowDenied(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	clients := NewOAuthClientService(client)
	svc := NewOAuthDeviceService(client, "https://portal.example.com")

	oc, err := clients.RegisterSelfHosted(ctx, 1, 42, "https://agent.example.com", "")
	if err != nil {
		t.Fatalf("RegisterSelfHosted: %v", err)
	}
	grant, err := svc.RequestCode(ctx, oc.ClientID, "inference")
	if err != nil {
		t.Fatalf("RequestCode: %v", err)
	}

	if err := svc.Deny(ctx, grant.UserCode); err != nil {
		t.Fatalf("Deny: %v", err)
	}

	row, err := svc.byDeviceCode(ctx, grant.DeviceCode)
	if err != nil {
		t.Fatalf("byDeviceCode: %v", err)
	}
	if row.Status != "denied" {
		t.Fatalf("expected denied, got %q", row.Status)
	}
	if row.ApprovedUserID != nil {
		t.Fatalf("expected no approved_user_id on deny, got %v", *row.ApprovedUserID)
	}
}

func TestRequestCodeRejectsUnknownClient(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	svc := NewOAuthDeviceService(client, "https://portal.example.com")

	if _, err := svc.RequestCode(ctx, "agent:does-not-exist", "inference"); err == nil {
		t.Fatal("expected an error for an unregistered client_id")
	}
}

func TestRequestCodeRejectsEmptyPortalBaseURL(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	clients := NewOAuthClientService(client)
	svc := NewOAuthDeviceService(client, "")

	oc, err := clients.RegisterSelfHosted(ctx, 1, 42, "https://agent.example.com", "")
	if err != nil {
		t.Fatalf("RegisterSelfHosted: %v", err)
	}

	_, err = svc.RequestCode(ctx, oc.ClientID, "inference")
	if !errors.Is(err, ErrPortalNotConfigured) {
		t.Fatalf("expected ErrPortalNotConfigured, got %v", err)
	}
}

func TestApproveUnknownUserCodeReturnsErrDeviceCodeNotFound(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	svc := NewOAuthDeviceService(client, "https://portal.example.com")

	err := svc.Approve(ctx, "ZZZZ-ZZZZ", 1)
	if !errors.Is(err, ErrDeviceCodeNotFound) {
		t.Fatalf("expected ErrDeviceCodeNotFound, got %v", err)
	}
}
