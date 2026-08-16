package service

import (
	"context"
	"fmt"
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentorder"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// VerifyRazorpayPayment verifies the browser checkout response against the
// provider instance pinned to the user's order, then routes the verified
// notification through the normal fulfillment/idempotency path.
func (s *PaymentService) VerifyRazorpayPayment(ctx context.Context, userID int64, req VerifyRazorpayPaymentRequest) (*dbent.PaymentOrder, error) {
	if strings.TrimSpace(req.OutTradeNo) == "" ||
		strings.TrimSpace(req.ProviderOrderID) == "" ||
		strings.TrimSpace(req.PaymentID) == "" ||
		strings.TrimSpace(req.Signature) == "" {
		return nil, infraerrors.BadRequest("INVALID_INPUT", "razorpay payment verification fields are required")
	}

	order, err := s.entClient.PaymentOrder.Query().
		Where(paymentorder.UserIDEQ(userID), paymentorder.OutTradeNoEQ(strings.TrimSpace(req.OutTradeNo))).
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, infraerrors.NotFound("NOT_FOUND", "payment order not found")
		}
		return nil, fmt.Errorf("lookup payment order: %w", err)
	}

	provider, err := s.getOrderProvider(ctx, order)
	if err != nil {
		return nil, infraerrors.ServiceUnavailable("PAYMENT_PROVIDER_MISCONFIGURED", "payment provider misconfigured")
	}
	if provider.ProviderKey() != payment.TypeRazorpay {
		return nil, infraerrors.BadRequest("INVALID_PAYMENT_PROVIDER", "order is not a Razorpay payment")
	}
	verifier, ok := provider.(payment.ClientPaymentVerifier)
	if !ok {
		return nil, infraerrors.ServiceUnavailable("PAYMENT_PROVIDER_MISCONFIGURED", "Razorpay client verification is unavailable")
	}

	providerOrderID := strings.TrimSpace(req.ProviderOrderID)
	if snapshot := psOrderProviderSnapshot(order); snapshot != nil && snapshot.ProviderOrderID != "" {
		if !strings.EqualFold(providerOrderID, snapshot.ProviderOrderID) {
			return nil, infraerrors.BadRequest("PAYMENT_ORDER_MISMATCH", "Razorpay order does not match the Inferno order")
		}
	} else if !strings.EqualFold(providerOrderID, strings.TrimSpace(order.PaymentTradeNo)) {
		return nil, infraerrors.BadRequest("PAYMENT_ORDER_MISMATCH", "Razorpay order does not match the Inferno order")
	}

	notification, err := verifier.VerifyClientPayment(ctx, payment.ClientPaymentVerificationRequest{
		InternalOrderID: order.OutTradeNo,
		ProviderOrderID: providerOrderID,
		PaymentID:       strings.TrimSpace(req.PaymentID),
		Signature:       strings.TrimSpace(req.Signature),
	})
	if err != nil {
		return nil, infraerrors.BadRequest("PAYMENT_VERIFICATION_FAILED", "Razorpay payment verification failed")
	}
	if notification == nil || notification.Status != payment.ProviderStatusSuccess {
		return nil, infraerrors.BadRequest("PAYMENT_NOT_CAPTURED", "Razorpay payment is not captured")
	}
	if err := s.HandlePaymentNotification(ctx, notification, payment.TypeRazorpay); err != nil {
		return nil, err
	}
	return s.entClient.PaymentOrder.Get(ctx, order.ID)
}

type VerifyRazorpayPaymentRequest struct {
	OutTradeNo      string
	ProviderOrderID string
	PaymentID       string
	Signature       string
}
