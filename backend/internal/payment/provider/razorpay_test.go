package provider

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

func TestRazorpayCreateAndVerifyCapturedPayment(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Basic a2V5LWlkOnNlY3JldA==", r.Header.Get("Authorization"))
		switch r.URL.Path {
		case "/orders":
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			var request razorpayOrderRequest
			require.NoError(t, json.Unmarshal(body, &request))
			require.Equal(t, int64(1234), request.Amount)
			require.Equal(t, "INR", request.Currency)
			require.Equal(t, "inferno-order-1", request.Receipt)
			_, _ = w.Write([]byte(`{"id":"order_123","amount":1234,"currency":"INR","receipt":"inferno-order-1","status":"created"}`))
		case "/payments/pay_123":
			_, _ = w.Write([]byte(`{"id":"pay_123","order_id":"order_123","amount":1234,"currency":"INR","status":"captured","captured":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	provider, err := NewRazorpay("instance-1", map[string]string{
		"keyId":         "key-id",
		"keySecret":     "secret",
		"webhookSecret": "webhook-secret",
		"currency":      "INR",
		"apiBase":       server.URL,
	})
	require.NoError(t, err)

	created, err := provider.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		OrderID: "inferno-order-1",
		Amount:  "12.34",
	})
	require.NoError(t, err)
	require.Equal(t, "order_123", created.TradeNo)
	require.Equal(t, "order_123", created.IntentID)
	require.Equal(t, "INR", created.Currency)
	require.Equal(t, "key-id", created.PublicKey)

	message := "order_123|pay_123"
	signature := razorpayTestSignature(message, "secret")
	notification, err := provider.VerifyClientPayment(context.Background(), payment.ClientPaymentVerificationRequest{
		InternalOrderID: "inferno-order-1",
		ProviderOrderID: "order_123",
		PaymentID:       "pay_123",
		Signature:       signature,
	})
	require.NoError(t, err)
	require.Equal(t, payment.ProviderStatusSuccess, notification.Status)
	require.Equal(t, "pay_123", notification.TradeNo)
	require.Equal(t, "inferno-order-1", notification.OrderID)
	require.Equal(t, 12.34, notification.Amount)
}

func TestRazorpayRejectsInvalidSignatureAndUncapturedPayment(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"pay_123","order_id":"order_123","amount":1234,"currency":"INR","status":"authorized","captured":false}`))
	}))
	defer server.Close()

	provider, err := NewRazorpay("instance-1", map[string]string{
		"keyId":         "key-id",
		"keySecret":     "secret",
		"webhookSecret": "webhook-secret",
		"apiBase":       server.URL,
	})
	require.NoError(t, err)

	_, err = provider.VerifyClientPayment(context.Background(), payment.ClientPaymentVerificationRequest{
		InternalOrderID: "inferno-order-1",
		ProviderOrderID: "order_123",
		PaymentID:       "pay_123",
		Signature:       "not-a-signature",
	})
	require.ErrorContains(t, err, "signature mismatch")

	_, err = provider.VerifyClientPayment(context.Background(), payment.ClientPaymentVerificationRequest{
		InternalOrderID: "inferno-order-1",
		ProviderOrderID: "order_123",
		PaymentID:       "pay_123",
		Signature:       razorpayTestSignature("order_123|pay_123", "secret"),
	})
	require.ErrorContains(t, err, "not captured")
}

func TestRazorpayWebhookVerifiesSignature(t *testing.T) {
	provider, err := NewRazorpay("instance-1", map[string]string{
		"keyId":         "key-id",
		"keySecret":     "secret",
		"webhookSecret": "webhook-secret",
	})
	require.NoError(t, err)

	body := `{"event":"payment.captured","payload":{"payment":{"entity":{"id":"pay_123","order_id":"order_123","amount":1234,"currency":"INR","status":"captured","captured":true}}}}`
	notification, err := provider.VerifyNotification(context.Background(), body, map[string]string{
		"x-razorpay-signature": razorpayTestSignature(body, "webhook-secret"),
	})
	require.NoError(t, err)
	require.Equal(t, "pay_123", notification.TradeNo)
	require.Equal(t, "order_123", notification.OrderID)
	require.Equal(t, payment.ProviderStatusSuccess, notification.Status)

	_, err = provider.VerifyNotification(context.Background(), body, map[string]string{
		"x-razorpay-signature": razorpayTestSignature(body, "wrong-secret"),
	})
	require.ErrorContains(t, err, "signature mismatch")
}

func TestRazorpaySubscriptionCreatesPlanAndVerifiesPayment(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/plans":
			switch r.Method {
			case http.MethodGet:
				_, _ = w.Write([]byte(`{"items":[]}`))
			case http.MethodPost:
				_, _ = w.Write([]byte(`{"id":"plan_123","period":"monthly","interval":1}`))
			default:
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			}
		case "/subscriptions":
			_, _ = w.Write([]byte(`{"id":"sub_123","plan_id":"plan_123","status":"created"}`))
		case "/payments/pay_sub_123":
			_, _ = w.Write([]byte(`{"id":"pay_sub_123","subscription_id":"sub_123","amount":99900,"currency":"INR","status":"captured","captured":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	provider, err := NewRazorpay("instance-1", map[string]string{
		"keyId":         "key-id",
		"keySecret":     "secret",
		"webhookSecret": "webhook-secret",
		"apiBase":       server.URL,
	})
	require.NoError(t, err)

	created, err := provider.CreateSubscription(context.Background(), payment.RazorpaySubscriptionRequest{
		Plan: payment.RazorpaySubscriptionPlanRequest{
			LocalPlanID: "42",
			Name:        "Inferno Pro",
			Amount:      999,
			Currency:    "INR",
			Period:      "monthly",
			Interval:    1,
			InstanceID:  "instance-1",
		},
		OrderID: "inferno-order-42",
		UserID:  "7",
	})
	require.NoError(t, err)
	require.Equal(t, "sub_123", created.SubscriptionID)
	require.Equal(t, "plan_123", created.PlanID)

	signature := razorpayTestSignature("pay_sub_123|sub_123", "secret")
	notification, err := provider.VerifySubscriptionPayment(context.Background(), payment.RazorpaySubscriptionPaymentVerificationRequest{
		InternalOrderID:        "inferno-order-42",
		ProviderSubscriptionID: "sub_123",
		PaymentID:              "pay_sub_123",
		Signature:              signature,
	})
	require.NoError(t, err)
	require.Equal(t, payment.ProviderStatusSuccess, notification.Status)
	require.Equal(t, "sub_123", notification.Metadata["provider_subscription_id"])
}

func TestRazorpaySubscriptionAcceptsInitialAuthorizationWithoutPaymentSubscriptionID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/payments/pay_auth_123":
			_, _ = w.Write([]byte(`{"id":"pay_auth_123","order_id":"order_auth_123","amount":500,"currency":"INR","status":"captured","captured":true}`))
		case "/subscriptions/sub_123":
			_, _ = w.Write([]byte(`{"id":"sub_123","plan_id":"plan_123","status":"active","notes":{"inferno_order_id":"inferno-order-42"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	provider, err := NewRazorpay("instance-1", map[string]string{
		"keyId":         "key-id",
		"keySecret":     "secret",
		"webhookSecret": "webhook-secret",
		"apiBase":       server.URL,
	})
	require.NoError(t, err)

	notification, err := provider.VerifySubscriptionPayment(context.Background(), payment.RazorpaySubscriptionPaymentVerificationRequest{
		InternalOrderID:        "inferno-order-42",
		ProviderSubscriptionID: "sub_123",
		PaymentID:              "pay_auth_123",
		Signature:              razorpayTestSignature("pay_auth_123|sub_123", "secret"),
	})
	require.NoError(t, err)
	require.Equal(t, payment.ProviderStatusSuccess, notification.Status)
	require.Equal(t, "pay_auth_123", notification.TradeNo)
}

func TestRazorpaySubscriptionWebhookUsesSubscriptionNotesAndEventID(t *testing.T) {
	provider, err := NewRazorpay("instance-1", map[string]string{
		"keyId":         "key-id",
		"keySecret":     "secret",
		"webhookSecret": "webhook-secret",
	})
	require.NoError(t, err)
	body := `{"event":"subscription.charged","payload":{"subscription":{"entity":{"id":"sub_123","notes":{"inferno_order_id":"sub2_order"}}},"payment":{"entity":{"id":"pay_123","subscription_id":"sub_123","amount":99900,"currency":"INR","status":"captured","captured":true}}}}`
	notification, err := provider.VerifyNotification(context.Background(), body, map[string]string{
		"x-razorpay-signature": razorpayTestSignature(body, "webhook-secret"),
		"x-razorpay-event-id":  "evt_123",
	})
	require.NoError(t, err)
	require.Equal(t, "sub2_order", notification.OrderID)
	require.Equal(t, "sub_123", notification.Metadata["provider_subscription_id"])
	require.Equal(t, "subscription.charged", notification.Metadata["razorpay_event"])
	require.Equal(t, "evt_123", notification.Metadata["event_id"])
}

func razorpayTestSignature(message, secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	_, _ = h.Write([]byte(message))
	return hex.EncodeToString(h.Sum(nil))
}
