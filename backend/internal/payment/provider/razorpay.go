package provider

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
)

const (
	razorpayDefaultAPIBase           = "https://api.razorpay.com/v1"
	razorpayDefaultCurrency          = "INR"
	razorpayHTTPTimeout              = 15 * time.Second
	razorpayMaxResponseSize          = 1 << 20
	razorpayMinimumMinorAmount int64 = 100
)

// Razorpay implements Standard Web Checkout for one-time payments.
// Subscriptions deliberately remain outside this adapter for now.
type Razorpay struct {
	instanceID string
	config     map[string]string
	httpClient *http.Client
	apiBase    string
}

func NewRazorpay(instanceID string, config map[string]string) (*Razorpay, error) {
	return newRazorpay(instanceID, config, &http.Client{Timeout: razorpayHTTPTimeout})
}

func newRazorpay(instanceID string, config map[string]string, client *http.Client) (*Razorpay, error) {
	for _, key := range []string{"keyId", "keySecret", "webhookSecret"} {
		if strings.TrimSpace(config[key]) == "" {
			return nil, fmt.Errorf("razorpay config missing required key: %s", key)
		}
	}

	rawCurrency := strings.TrimSpace(config["currency"])
	if rawCurrency == "" {
		rawCurrency = razorpayDefaultCurrency
	}
	currency, err := payment.NormalizePaymentCurrency(rawCurrency)
	if err != nil {
		return nil, fmt.Errorf("razorpay config currency: %w", err)
	}
	if !strings.EqualFold(currency, razorpayDefaultCurrency) {
		return nil, fmt.Errorf("razorpay currently supports INR top-ups only")
	}

	apiBase := strings.TrimRight(strings.TrimSpace(config["apiBase"]), "/")
	if apiBase == "" {
		apiBase = razorpayDefaultAPIBase
	}
	parsed, err := url.Parse(apiBase)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("razorpay apiBase must be an absolute URL without query or fragment")
	}

	cfg := cloneStringMap(config)
	cfg["currency"] = razorpayDefaultCurrency
	return &Razorpay{
		instanceID: instanceID,
		config:     cfg,
		httpClient: client,
		apiBase:    apiBase,
	}, nil
}

func (r *Razorpay) Name() string { return "Razorpay" }

func (r *Razorpay) ProviderKey() string { return payment.TypeRazorpay }

func (r *Razorpay) SupportedTypes() []payment.PaymentType {
	return []payment.PaymentType{payment.TypeRazorpay}
}

func (r *Razorpay) MerchantIdentityMetadata() map[string]string {
	return map[string]string{"currency": r.currency()}
}

func (r *Razorpay) currency() string {
	if r == nil || strings.TrimSpace(r.config["currency"]) == "" {
		return razorpayDefaultCurrency
	}
	return razorpayDefaultCurrency
}

type razorpayOrderRequest struct {
	Amount   int64             `json:"amount"`
	Currency string            `json:"currency"`
	Receipt  string            `json:"receipt"`
	Notes    map[string]string `json:"notes,omitempty"`
}

type razorpayOrder struct {
	ID         string            `json:"id"`
	Amount     int64             `json:"amount"`
	AmountPaid int64             `json:"amount_paid"`
	AmountDue  int64             `json:"amount_due"`
	Currency   string            `json:"currency"`
	Receipt    string            `json:"receipt"`
	Status     string            `json:"status"`
	Notes      map[string]string `json:"notes"`
}

type razorpayPayment struct {
	ID       string `json:"id"`
	OrderID  string `json:"order_id"`
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
	Status   string `json:"status"`
	Captured bool   `json:"captured"`
}

func (r *Razorpay) CreatePayment(ctx context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	amount, err := payment.AmountToMinorUnit(req.Amount, r.currency())
	if err != nil {
		return nil, fmt.Errorf("razorpay create payment: %w", err)
	}
	if amount < razorpayMinimumMinorAmount {
		return nil, fmt.Errorf("razorpay minimum payment amount is %d paise", razorpayMinimumMinorAmount)
	}

	order := razorpayOrderRequest{
		Amount:   amount,
		Currency: r.currency(),
		Receipt:  req.OrderID,
		Notes: map[string]string{
			"inferno_order_id": req.OrderID,
		},
	}
	var created razorpayOrder
	if err := r.doJSON(ctx, http.MethodPost, "/orders", order, &created); err != nil {
		return nil, fmt.Errorf("razorpay create order: %w", err)
	}
	if strings.TrimSpace(created.ID) == "" {
		return nil, fmt.Errorf("razorpay create order returned no order id")
	}

	return &payment.CreatePaymentResponse{
		TradeNo:    created.ID,
		IntentID:   created.ID,
		Currency:   r.currency(),
		PaymentEnv: "razorpay",
		PublicKey:  strings.TrimSpace(r.config["keyId"]),
	}, nil
}

func (r *Razorpay) QueryOrder(ctx context.Context, tradeNo string) (*payment.QueryOrderResponse, error) {
	tradeNo = strings.TrimSpace(tradeNo)
	if tradeNo == "" {
		return nil, fmt.Errorf("razorpay query order: missing trade number")
	}

	if strings.HasPrefix(tradeNo, "pay_") {
		var p razorpayPayment
		if err := r.doJSON(ctx, http.MethodGet, "/payments/"+url.PathEscape(tradeNo), nil, &p); err != nil {
			return nil, fmt.Errorf("razorpay query payment: %w", err)
		}
		status := payment.ProviderStatusPending
		if p.Captured || strings.EqualFold(p.Status, "captured") {
			status = payment.ProviderStatusPaid
		} else if strings.EqualFold(p.Status, "failed") {
			status = payment.ProviderStatusFailed
		}
		return &payment.QueryOrderResponse{
			TradeNo:  p.ID,
			Status:   status,
			Amount:   payment.MinorUnitToAmount(p.Amount, r.currency()),
			Metadata: map[string]string{"currency": r.currency(), "status": p.Status},
		}, nil
	}

	var order razorpayOrder
	if err := r.doJSON(ctx, http.MethodGet, "/orders/"+url.PathEscape(tradeNo), nil, &order); err != nil {
		return nil, fmt.Errorf("razorpay query order: %w", err)
	}
	status := payment.ProviderStatusPending
	if strings.EqualFold(order.Status, "paid") {
		status = payment.ProviderStatusPaid
	} else if strings.EqualFold(order.Status, "cancelled") {
		status = payment.ProviderStatusFailed
	}
	paidAmount := order.AmountPaid
	if paidAmount == 0 {
		paidAmount = order.Amount
	}
	return &payment.QueryOrderResponse{
		TradeNo:  order.ID,
		Status:   status,
		Amount:   payment.MinorUnitToAmount(paidAmount, r.currency()),
		Metadata: map[string]string{"currency": r.currency(), "status": order.Status},
	}, nil
}

func (r *Razorpay) VerifyClientPayment(ctx context.Context, req payment.ClientPaymentVerificationRequest) (*payment.PaymentNotification, error) {
	providerOrderID := strings.TrimSpace(req.ProviderOrderID)
	paymentID := strings.TrimSpace(req.PaymentID)
	signature := strings.TrimSpace(req.Signature)
	if providerOrderID == "" || paymentID == "" || signature == "" || strings.TrimSpace(req.InternalOrderID) == "" {
		return nil, fmt.Errorf("razorpay payment verification requires order id, payment id, signature, and internal order id")
	}
	if !verifyHMAC(providerOrderID+"|"+paymentID, signature, r.config["keySecret"]) {
		return nil, fmt.Errorf("razorpay payment signature mismatch")
	}

	var p razorpayPayment
	if err := r.doJSON(ctx, http.MethodGet, "/payments/"+url.PathEscape(paymentID), nil, &p); err != nil {
		return nil, fmt.Errorf("razorpay verify payment status: %w", err)
	}
	if !strings.EqualFold(strings.TrimSpace(p.OrderID), providerOrderID) {
		return nil, fmt.Errorf("razorpay payment does not belong to the created order")
	}
	if !p.Captured && !strings.EqualFold(p.Status, "captured") {
		return nil, fmt.Errorf("razorpay payment is not captured")
	}
	if p.Amount <= 0 {
		return nil, fmt.Errorf("razorpay payment returned an invalid amount")
	}

	return &payment.PaymentNotification{
		TradeNo: paymentID,
		OrderID: req.InternalOrderID,
		Amount:  payment.MinorUnitToAmount(p.Amount, r.currency()),
		Status:  payment.ProviderStatusSuccess,
		Metadata: map[string]string{
			"currency":          r.currency(),
			"status":            "captured",
			"provider_order_id": providerOrderID,
		},
	}, nil
}

func (r *Razorpay) VerifyNotification(_ context.Context, rawBody string, headers map[string]string) (*payment.PaymentNotification, error) {
	signature := strings.TrimSpace(headers["x-razorpay-signature"])
	if signature == "" {
		return nil, fmt.Errorf("razorpay notification missing x-razorpay-signature header")
	}
	if !verifyHMAC(rawBody, signature, r.config["webhookSecret"]) {
		return nil, fmt.Errorf("razorpay webhook signature mismatch")
	}

	var event struct {
		Event   string `json:"event"`
		Payload struct {
			Payment struct {
				Entity razorpayPayment `json:"entity"`
			} `json:"payment"`
			Order struct {
				Entity razorpayOrder `json:"entity"`
			} `json:"order"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(rawBody), &event); err != nil {
		return nil, fmt.Errorf("razorpay parse webhook: %w", err)
	}

	p := event.Payload.Payment.Entity
	o := event.Payload.Order.Entity
	status := ""
	switch event.Event {
	case "payment.captured", "order.paid":
		status = payment.ProviderStatusSuccess
	case "payment.failed":
		status = payment.ProviderStatusFailed
	default:
		return nil, nil
	}
	tradeNo := strings.TrimSpace(p.ID)
	amount := p.Amount
	currency := strings.TrimSpace(p.Currency)
	if tradeNo == "" {
		tradeNo = strings.TrimSpace(o.ID)
	}
	if amount == 0 {
		amount = o.AmountPaid
		if amount == 0 {
			amount = o.Amount
		}
	}
	if currency == "" {
		currency = r.currency()
	}
	orderID := strings.TrimSpace(o.Receipt)
	if orderID == "" {
		orderID = strings.TrimSpace(p.OrderID)
	}
	providerOrderID := strings.TrimSpace(p.OrderID)
	if providerOrderID == "" {
		providerOrderID = strings.TrimSpace(o.ID)
	}
	return &payment.PaymentNotification{
		TradeNo: tradeNo,
		OrderID: orderID,
		Amount:  payment.MinorUnitToAmount(amount, currency),
		Status:  status,
		RawData: rawBody,
		Metadata: map[string]string{
			"currency":          currency,
			"status":            strings.TrimSpace(p.Status),
			"provider_order_id": providerOrderID,
		},
	}, nil
}

func (r *Razorpay) Refund(ctx context.Context, req payment.RefundRequest) (*payment.RefundResponse, error) {
	tradeNo := strings.TrimSpace(req.TradeNo)
	if tradeNo == "" || !strings.HasPrefix(tradeNo, "pay_") {
		return nil, fmt.Errorf("razorpay refund requires the captured payment id")
	}
	amount, err := payment.AmountToMinorUnit(req.Amount, r.currency())
	if err != nil {
		return nil, fmt.Errorf("razorpay refund: %w", err)
	}
	var refund struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if err := r.doJSON(ctx, http.MethodPost, "/payments/"+url.PathEscape(tradeNo)+"/refund", map[string]any{"amount": amount}, &refund); err != nil {
		return nil, fmt.Errorf("razorpay refund: %w", err)
	}
	status := payment.ProviderStatusPending
	if strings.EqualFold(refund.Status, "processed") {
		status = payment.ProviderStatusSuccess
	}
	return &payment.RefundResponse{RefundID: refund.ID, Status: status}, nil
}

func verifyHMAC(message, provided, secret string) bool {
	providedBytes, err := hex.DecodeString(strings.TrimSpace(provided))
	if err != nil {
		return false
	}
	h := hmac.New(sha256.New, []byte(secret))
	_, _ = h.Write([]byte(message))
	return hmac.Equal(h.Sum(nil), providedBytes)
}

func (r *Razorpay) doJSON(ctx context.Context, method, path string, body any, out any) error {
	var bodyReader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return err
		}
		bodyReader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, r.apiBase+path, bodyReader)
	if err != nil {
		return err
	}
	credentials := base64.StdEncoding.EncodeToString([]byte(r.config["keyId"] + ":" + r.config["keySecret"]))
	req.Header.Set("Authorization", "Basic "+credentials)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := r.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, razorpayMaxResponseSize))
	if err != nil {
		return err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("api returned HTTP %d: %s", resp.StatusCode, summarizeRazorpayError(responseBody))
	}
	if out == nil || len(responseBody) == 0 {
		return nil
	}
	if err := json.Unmarshal(responseBody, out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func summarizeRazorpayError(body []byte) string {
	var payload struct {
		Error struct {
			Description string `json:"description"`
			Reason      string `json:"reason"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &payload) == nil {
		if description := strings.TrimSpace(payload.Error.Description); description != "" {
			return description
		}
		if reason := strings.TrimSpace(payload.Error.Reason); reason != "" {
			return reason
		}
	}
	return "upstream request failed"
}
