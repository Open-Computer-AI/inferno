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

func razorpayTestSignature(message, secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	_, _ = h.Write([]byte(message))
	return hex.EncodeToString(h.Sum(nil))
}
