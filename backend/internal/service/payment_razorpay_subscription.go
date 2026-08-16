package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/idempotencyrecord"
	"github.com/Wei-Shaw/sub2api/ent/paymentorder"
	"github.com/Wei-Shaw/sub2api/internal/payment"
)

const razorpaySubscriptionEventScope = "razorpay_subscription_webhook"

type VerifyRazorpaySubscriptionPaymentRequest struct {
	OutTradeNo             string
	ProviderSubscriptionID string
	PaymentID              string
	Signature              string
}

// VerifyRazorpaySubscriptionPayment verifies the initial subscription charge
// and routes it through the existing order fulfillment state machine.
func (s *PaymentService) VerifyRazorpaySubscriptionPayment(ctx context.Context, userID int64, req VerifyRazorpaySubscriptionPaymentRequest) (*dbent.PaymentOrder, error) {
	order, err := s.entClient.PaymentOrder.Query().Where(
		paymentorder.OutTradeNo(req.OutTradeNo),
		paymentorder.UserIDEQ(userID),
	).Only(ctx)
	if err != nil {
		return nil, fmt.Errorf("find Razorpay subscription order: %w", err)
	}
	if order.OrderType != payment.OrderTypeSubscription || payment.GetBasePaymentType(order.PaymentType) != payment.TypeRazorpay {
		return nil, fmt.Errorf("order is not a Razorpay subscription order")
	}
	prov, err := s.getPinnedOrderProvider(ctx, order)
	if err != nil {
		return nil, err
	}
	subscriptionProvider, ok := prov.(payment.RazorpaySubscriptionProvider)
	if !ok {
		return nil, fmt.Errorf("Razorpay subscription capability is unavailable")
	}
	notification, err := subscriptionProvider.VerifySubscriptionPayment(ctx, payment.RazorpaySubscriptionPaymentVerificationRequest{
		InternalOrderID:        order.OutTradeNo,
		ProviderSubscriptionID: req.ProviderSubscriptionID,
		PaymentID:              req.PaymentID,
		Signature:              req.Signature,
	})
	if err != nil {
		return nil, err
	}
	if err := s.HandlePaymentNotification(ctx, notification, payment.TypeRazorpay); err != nil {
		return nil, err
	}
	return s.entClient.PaymentOrder.Get(ctx, order.ID)
}

// HandleRazorpaySubscriptionWebhook applies recurring charges. The initial
// charge completes the PaymentOrder; later charges extend the same Inferno
// entitlement and never mutate the original order back through PAID again.
func (s *PaymentService) HandleRazorpaySubscriptionWebhook(ctx context.Context, n *payment.PaymentNotification) error {
	if n == nil || n.Metadata["razorpay_event"] != "subscription.charged" {
		return nil
	}
	eventID := strings.TrimSpace(n.Metadata["event_id"])
	if eventID == "" {
		eventID = n.OrderID + "|" + n.TradeNo + "|" + n.Metadata["razorpay_event"]
	}
	claimed, record, err := s.claimRazorpaySubscriptionEvent(ctx, eventID)
	if err != nil {
		return err
	}
	if !claimed {
		return nil
	}
	markFailed := func(cause error) error {
		if record != nil {
			reason := cause.Error()
			_, _ = s.entClient.IdempotencyRecord.UpdateOneID(record.ID).
				SetStatus("failed").SetErrorReason(reason).ClearLockedUntil().Save(ctx)
		}
		return cause
	}

	order, err := s.entClient.PaymentOrder.Query().Where(paymentorder.OutTradeNo(n.OrderID)).Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return markFailed(fmt.Errorf("%w: out_trade_no=%s", ErrOrderNotFound, n.OrderID))
		}
		return markFailed(err)
	}
	if order.OrderType != payment.OrderTypeSubscription || payment.GetBasePaymentType(order.PaymentType) != payment.TypeRazorpay {
		return markFailed(fmt.Errorf("Razorpay subscription webhook references a non-subscription order"))
	}
	if err := validateProviderSnapshotMetadata(order, payment.TypeRazorpay, n.Metadata); err != nil {
		return markFailed(err)
	}
	if !isValidProviderAmount(n.Amount) || absFloat(n.Amount-order.PayAmount) > paymentAmountToleranceForCurrency(PaymentOrderCurrency(order)) {
		return markFailed(fmt.Errorf("Razorpay subscription amount mismatch: expected %v, got %v", order.PayAmount, n.Amount))
	}

	// If the browser callback was missed, this first charged webhook is still
	// allowed to complete the order. Do not also run the recurring extension in
	// that branch, because the initial fulfillment already assigns the period.
	if order.Status != OrderStatusCompleted {
		if err := s.confirmPayment(ctx, order.ID, n.TradeNo, n.Amount, payment.TypeRazorpay, n.Metadata); err != nil {
			return markFailed(err)
		}
	} else {
		if order.SubscriptionGroupID == nil || order.SubscriptionDays == nil || s.subscriptionSvc == nil {
			return markFailed(fmt.Errorf("Razorpay subscription order is missing entitlement data"))
		}
		if _, _, err := s.subscriptionSvc.assignOrExtendSubscription(ctx, &AssignSubscriptionInput{
			UserID:       order.UserID,
			GroupID:      *order.SubscriptionGroupID,
			ValidityDays: *order.SubscriptionDays,
			AssignedBy:   0,
			Notes:        fmt.Sprintf("Razorpay recurring charge %s", n.TradeNo),
		}, false); err != nil {
			return markFailed(err)
		}
		s.writeAuditLog(ctx, order.ID, "RAZORPAY_SUBSCRIPTION_CHARGED", "razorpay", map[string]any{
			"eventID":           eventID,
			"paymentID":         n.TradeNo,
			"subscriptionID":    n.Metadata["provider_subscription_id"],
			"amount":            n.Amount,
			"fulfillmentAction": "subscription_extended",
		})
	}
	if record != nil {
		_, _ = s.entClient.IdempotencyRecord.UpdateOneID(record.ID).
			SetStatus("succeeded").SetResponseStatus(200).ClearLockedUntil().Save(ctx)
	}
	return nil
}

func absFloat(value float64) float64 {
	if value < 0 {
		return -value
	}
	return value
}

func (s *PaymentService) claimRazorpaySubscriptionEvent(ctx context.Context, eventID string) (bool, *dbent.IdempotencyRecord, error) {
	hash := sha256.Sum256([]byte(eventID))
	keyHash := hex.EncodeToString(hash[:])
	now := time.Now()
	query := s.entClient.IdempotencyRecord.Query().Where(
		idempotencyrecord.ScopeEQ(razorpaySubscriptionEventScope),
		idempotencyrecord.IdempotencyKeyHashEQ(keyHash),
	)
	record, err := query.Only(ctx)
	if err == nil {
		if record.Status == "succeeded" {
			return false, record, nil
		}
		if record.LockedUntil != nil && record.LockedUntil.After(now) {
			return false, record, nil
		}
		updated, updateErr := s.entClient.IdempotencyRecord.UpdateOneID(record.ID).
			SetStatus("processing").SetLockedUntil(now.Add(5 * time.Minute)).ClearErrorReason().Save(ctx)
		return updateErr == nil, updated, updateErr
	}
	if !dbent.IsNotFound(err) {
		return false, nil, err
	}
	record, err = s.entClient.IdempotencyRecord.Create().
		SetScope(razorpaySubscriptionEventScope).
		SetIdempotencyKeyHash(keyHash).
		SetRequestFingerprint(keyHash).
		SetStatus("processing").
		SetLockedUntil(now.Add(5 * time.Minute)).
		SetExpiresAt(now.Add(90 * 24 * time.Hour)).
		Save(ctx)
	if err == nil {
		return true, record, nil
	}
	if dbent.IsConstraintError(err) {
		return s.claimRazorpaySubscriptionEvent(ctx, eventID)
	}
	return false, nil, err
}
