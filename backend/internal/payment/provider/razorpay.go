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
	"strconv"
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

// Razorpay implements Standard Web Checkout for one-time payments and the
// optional recurring-subscription capability.
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
	ID             string `json:"id"`
	OrderID        string `json:"order_id"`
	SubscriptionID string `json:"subscription_id"`
	Amount         int64  `json:"amount"`
	Currency       string `json:"currency"`
	Status         string `json:"status"`
	Captured       bool   `json:"captured"`
}

type razorpayPlan struct {
	ID   string `json:"id"`
	Item struct {
		Name string `json:"name"`
	} `json:"item"`
	Notes    map[string]string `json:"notes"`
	Period   string            `json:"period"`
	Interval int               `json:"interval"`
}

type razorpayPlansResponse struct {
	Items []razorpayPlan `json:"items"`
}

type razorpaySubscription struct {
	ID     string            `json:"id"`
	PlanID string            `json:"plan_id"`
	Status string            `json:"status"`
	Notes  map[string]string `json:"notes"`
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

func (r *Razorpay) CreateSubscription(ctx context.Context, req payment.RazorpaySubscriptionRequest) (*payment.RazorpaySubscriptionResponse, error) {
	planID, err := r.ensurePlan(ctx, req.Plan)
	if err != nil {
		return nil, fmt.Errorf("razorpay ensure subscription plan: %w", err)
	}

	totalCount := req.TotalCount
	if totalCount <= 0 {
		// Razorpay requires a finite count. 1,200 monthly cycles is 100 years;
		// users can still cancel through the provider before the next charge.
		totalCount = 1200
	}
	subscriptionRequest := map[string]any{
		"plan_id":         planID,
		"total_count":     totalCount,
		"quantity":        1,
		"customer_notify": 1,
		"notes": map[string]string{
			"inferno_order_id": req.OrderID,
			"inferno_user_id":  req.UserID,
			"inferno_plan_id":  req.Plan.LocalPlanID,
		},
	}
	var created razorpaySubscription
	if err := r.doJSON(ctx, http.MethodPost, "/subscriptions", subscriptionRequest, &created); err != nil {
		return nil, fmt.Errorf("razorpay create subscription: %w", err)
	}
	if strings.TrimSpace(created.ID) == "" {
		return nil, fmt.Errorf("razorpay create subscription returned no subscription id")
	}
	return &payment.RazorpaySubscriptionResponse{
		SubscriptionID: created.ID,
		PlanID:         planID,
		Status:         created.Status,
	}, nil
}

func (r *Razorpay) ensurePlan(ctx context.Context, req payment.RazorpaySubscriptionPlanRequest) (string, error) {
	if strings.TrimSpace(req.LocalPlanID) == "" || strings.TrimSpace(req.InstanceID) == "" {
		return "", fmt.Errorf("local plan id and provider instance id are required")
	}
	amount, err := payment.AmountToMinorUnit(strconv.FormatFloat(req.Amount, 'f', -1, 64), req.Currency)
	if err != nil {
		return "", fmt.Errorf("subscription plan amount: %w", err)
	}
	if amount < razorpayMinimumMinorAmount {
		return "", fmt.Errorf("subscription plan amount must be at least %d paise", razorpayMinimumMinorAmount)
	}
	period := strings.TrimSpace(strings.ToLower(req.Period))
	if period == "" || req.Interval <= 0 {
		return "", fmt.Errorf("subscription plan period and interval are required")
	}

	// Plan IDs belong to a Razorpay account, so reuse a matching plan after a
	// process restart instead of creating a new provider plan for every order.
	for skip := 0; skip < 2000; skip += 100 {
		var listed razorpayPlansResponse
		path := "/plans?count=100&skip=" + strconv.Itoa(skip)
		if err := r.doJSON(ctx, http.MethodGet, path, nil, &listed); err != nil {
			return "", fmt.Errorf("list plans: %w", err)
		}
		for _, candidate := range listed.Items {
			if candidate.ID == "" || candidate.Notes == nil {
				continue
			}
			if candidate.Notes["inferno_plan_id"] == req.LocalPlanID &&
				candidate.Notes["inferno_provider_instance_id"] == req.InstanceID &&
				strings.EqualFold(candidate.Period, period) && candidate.Interval == req.Interval {
				return candidate.ID, nil
			}
		}
		if len(listed.Items) < 100 {
			break
		}
	}

	planRequest := map[string]any{
		"period":   period,
		"interval": req.Interval,
		"item": map[string]any{
			"name":        firstNonEmpty(req.Name, "Inferno subscription"),
			"amount":      amount,
			"currency":    r.currency(),
			"description": strings.TrimSpace(req.Description),
		},
		"notes": map[string]string{
			"inferno_plan_id":              req.LocalPlanID,
			"inferno_provider_instance_id": req.InstanceID,
		},
	}
	var created razorpayPlan
	if err := r.doJSON(ctx, http.MethodPost, "/plans", planRequest, &created); err != nil {
		return "", fmt.Errorf("create plan: %w", err)
	}
	if strings.TrimSpace(created.ID) == "" {
		return "", fmt.Errorf("create plan returned no plan id")
	}
	return created.ID, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
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

func (r *Razorpay) VerifySubscriptionPayment(ctx context.Context, req payment.RazorpaySubscriptionPaymentVerificationRequest) (*payment.PaymentNotification, error) {
	subscriptionID := strings.TrimSpace(req.ProviderSubscriptionID)
	paymentID := strings.TrimSpace(req.PaymentID)
	signature := strings.TrimSpace(req.Signature)
	if subscriptionID == "" || paymentID == "" || signature == "" || strings.TrimSpace(req.InternalOrderID) == "" {
		return nil, fmt.Errorf("razorpay subscription verification requires subscription id, payment id, signature, and internal order id")
	}
	if !verifyHMAC(paymentID+"|"+subscriptionID, signature, r.config["keySecret"]) {
		return nil, fmt.Errorf("razorpay subscription payment signature mismatch")
	}

	var p razorpayPayment
	if err := r.doJSON(ctx, http.MethodGet, "/payments/"+url.PathEscape(paymentID), nil, &p); err != nil {
		return nil, fmt.Errorf("razorpay verify subscription payment status: %w", err)
	}
	if !strings.EqualFold(strings.TrimSpace(p.SubscriptionID), subscriptionID) {
		// Razorpay's first subscription authorization can be represented as a
		// captured payment without payment.subscription_id. In that case, bind
		// the payment to the subscription through its signed checkout response
		// and the provider-side Inferno order note we set at creation time.
		var subscription razorpaySubscription
		if err := r.doJSON(ctx, http.MethodGet, "/subscriptions/"+url.PathEscape(subscriptionID), nil, &subscription); err != nil {
			return nil, fmt.Errorf("razorpay verify subscription: %w", err)
		}
		if !strings.EqualFold(strings.TrimSpace(subscription.ID), subscriptionID) ||
			!strings.EqualFold(strings.TrimSpace(subscription.Notes["inferno_order_id"]), strings.TrimSpace(req.InternalOrderID)) {
			return nil, fmt.Errorf("razorpay payment does not belong to the created subscription")
		}
	}
	if !p.Captured && !strings.EqualFold(p.Status, "captured") {
		return nil, fmt.Errorf("razorpay subscription payment is not captured")
	}
	if p.Amount <= 0 {
		return nil, fmt.Errorf("razorpay subscription payment returned an invalid amount")
	}
	return &payment.PaymentNotification{
		TradeNo: p.ID,
		OrderID: req.InternalOrderID,
		Amount:  payment.MinorUnitToAmount(p.Amount, r.currency()),
		Status:  payment.ProviderStatusSuccess,
		Metadata: map[string]string{
			"currency":                 r.currency(),
			"status":                   "captured",
			"provider_order_id":        subscriptionID,
			"provider_subscription_id": subscriptionID,
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
	eventID := strings.TrimSpace(headers["x-razorpay-event-id"])
	if eventID == "" {
		hash := sha256.Sum256([]byte(rawBody))
		eventID = hex.EncodeToString(hash[:])
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
			Subscription struct {
				Entity struct {
					ID     string            `json:"id"`
					Status string            `json:"status"`
					Notes  map[string]string `json:"notes"`
				} `json:"entity"`
			} `json:"subscription"`
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
	case "subscription.charged":
		status = payment.ProviderStatusSuccess
	default:
		return nil, nil
	}
	tradeNo := strings.TrimSpace(p.ID)
	amount := p.Amount
	currency := strings.TrimSpace(p.Currency)
	providerSubscriptionID := strings.TrimSpace(event.Payload.Subscription.Entity.ID)
	if event.Event == "subscription.charged" && providerSubscriptionID == "" {
		providerSubscriptionID = strings.TrimSpace(p.SubscriptionID)
	}
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
	if orderID == "" && event.Event == "subscription.charged" {
		orderID = strings.TrimSpace(event.Payload.Subscription.Entity.Notes["inferno_order_id"])
	}
	if orderID == "" {
		orderID = strings.TrimSpace(p.OrderID)
	}
	providerOrderID := strings.TrimSpace(p.OrderID)
	if providerOrderID == "" {
		providerOrderID = strings.TrimSpace(o.ID)
	}
	if event.Event == "subscription.charged" {
		providerOrderID = providerSubscriptionID
	}
	return &payment.PaymentNotification{
		TradeNo: tradeNo,
		OrderID: orderID,
		Amount:  payment.MinorUnitToAmount(amount, currency),
		Status:  status,
		RawData: rawBody,
		Metadata: map[string]string{
			"currency":                 currency,
			"status":                   strings.TrimSpace(p.Status),
			"provider_order_id":        providerOrderID,
			"provider_subscription_id": providerSubscriptionID,
			"razorpay_event":           event.Event,
			"event_id":                 eventID,
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
