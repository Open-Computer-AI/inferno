package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentorder"
	"github.com/Wei-Shaw/sub2api/ent/paymentproviderinstance"
	"github.com/Wei-Shaw/sub2api/internal/payment"
)

// GetWebhookProvider returns the provider instance that should verify a webhook.
// It resolves the original provider instance from the order whenever possible and
// only falls back to a registry provider for legacy/single-instance scenarios.
func (s *PaymentService) GetWebhookProvider(ctx context.Context, providerKey, outTradeNo string) (payment.Provider, error) {
	providers, err := s.GetWebhookProviders(ctx, providerKey, outTradeNo)
	if err != nil {
		return nil, err
	}
	if len(providers) == 0 {
		return nil, payment.ErrProviderNotFound
	}
	return providers[0], nil
}

// GetWebhookProviders returns provider candidates that can verify the webhook.
// Official WeChat Pay may require multiple candidates because the callback body
// cannot be bound to a merchant before decryption.
func (s *PaymentService) GetWebhookProviders(ctx context.Context, providerKey, outTradeNo string) ([]payment.Provider, error) {
	if outTradeNo != "" {
		order, err := s.entClient.PaymentOrder.Query().Where(paymentorder.OutTradeNo(outTradeNo)).Only(ctx)
		if err == nil {
			if psHasPinnedProviderInstance(order) {
				prov, err := s.getPinnedOrderProvider(ctx, order)
				if err != nil {
					return nil, err
				}
				return []payment.Provider{prov}, nil
			}
			inst, err := s.getOrderProviderInstance(ctx, order)
			if err != nil {
				return nil, fmt.Errorf("load order provider instance: %w", err)
			}
			if inst != nil {
				prov, err := s.createProviderFromInstance(ctx, inst)
				if err != nil {
					return nil, err
				}
				return []payment.Provider{prov}, nil
			}
			if strings.TrimSpace(providerKey) == payment.TypeWxpay {
				return s.getEnabledWebhookProvidersByKey(ctx, providerKey)
			}
			if !s.webhookRegistryFallbackAllowed(ctx, providerKey) {
				return nil, fmt.Errorf("webhook provider fallback is ambiguous for %s", providerKey)
			}
			s.EnsureProviders(ctx)
			prov, err := s.registry.GetProviderByKey(providerKey)
			if err != nil {
				return nil, err
			}
			return []payment.Provider{prov}, nil
		}
	}

	if strings.TrimSpace(providerKey) == payment.TypeWxpay {
		return s.getEnabledWebhookProvidersByKey(ctx, providerKey)
	}

	if !s.webhookRegistryFallbackAllowed(ctx, providerKey) {
		return nil, fmt.Errorf("webhook provider fallback is ambiguous for %s", providerKey)
	}

	s.EnsureProviders(ctx)
	prov, err := s.registry.GetProviderByKey(providerKey)
	if err != nil {
		return nil, err
	}
	return []payment.Provider{prov}, nil
}

// GetWebhookProvidersForNotification also resolves a pinned provider by the
// upstream trade number. Razorpay payment.captured events can contain the
// upstream order ID without Inferno's receipt, so this preserves correct
// routing when multiple Razorpay instances exist.
func (s *PaymentService) GetWebhookProvidersForNotification(ctx context.Context, providerKey, outTradeNo, providerTradeNo string) ([]payment.Provider, error) {
	if strings.TrimSpace(outTradeNo) != "" {
		return s.GetWebhookProviders(ctx, providerKey, outTradeNo)
	}
	providerTradeNo = strings.TrimSpace(providerTradeNo)
	if providerTradeNo != "" {
		order, err := s.entClient.PaymentOrder.Query().Where(paymentorder.PaymentTradeNo(providerTradeNo)).Only(ctx)
		if err == nil {
			prov, provErr := s.getPinnedOrderProvider(ctx, order)
			if provErr == nil {
				return []payment.Provider{prov}, nil
			}
		}
		if providerKey == payment.TypeRazorpay {
			orders, queryErr := s.entClient.PaymentOrder.Query().Where(paymentorder.ProviderKeyEQ(providerKey)).All(ctx)
			if queryErr == nil {
				for _, candidate := range orders {
					snapshot := psOrderProviderSnapshot(candidate)
					if snapshot == nil || !strings.EqualFold(strings.TrimSpace(snapshot.ProviderOrderID), providerTradeNo) {
						continue
					}
					prov, provErr := s.getPinnedOrderProvider(ctx, candidate)
					if provErr == nil {
						return []payment.Provider{prov}, nil
					}
				}
			}
		}
	}
	return s.GetWebhookProviders(ctx, providerKey, "")
}

func (s *PaymentService) getPinnedOrderProvider(ctx context.Context, o *dbent.PaymentOrder) (payment.Provider, error) {
	inst, err := s.getOrderProviderInstance(ctx, o)
	if err != nil {
		return nil, fmt.Errorf("load order provider instance: %w", err)
	}
	if inst == nil {
		return nil, fmt.Errorf("order %d provider instance is missing", o.ID)
	}
	return s.createProviderFromInstance(ctx, inst)
}

func (s *PaymentService) webhookRegistryFallbackAllowed(ctx context.Context, providerKey string) bool {
	providerKey = strings.TrimSpace(providerKey)
	if providerKey == "" || s == nil || s.entClient == nil {
		return false
	}

	count, err := s.entClient.PaymentProviderInstance.Query().
		Where(
			paymentproviderinstance.ProviderKeyEQ(providerKey),
			paymentproviderinstance.EnabledEQ(true),
		).
		Count(ctx)
	if err != nil {
		slog.Warn("payment webhook fallback instance count failed", "provider", providerKey, "error", err)
		return false
	}
	return count <= 1
}

func psHasPinnedProviderInstance(order *dbent.PaymentOrder) bool {
	return order != nil && (psOrderProviderSnapshot(order) != nil || (order.ProviderInstanceID != nil && strings.TrimSpace(*order.ProviderInstanceID) != ""))
}

func (s *PaymentService) getEnabledWebhookProvidersByKey(ctx context.Context, providerKey string) ([]payment.Provider, error) {
	providerKey = strings.TrimSpace(providerKey)
	instances, err := s.entClient.PaymentProviderInstance.Query().
		Where(
			paymentproviderinstance.ProviderKeyEQ(providerKey),
			paymentproviderinstance.EnabledEQ(true),
		).
		Order(dbent.Asc(paymentproviderinstance.FieldSortOrder)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query webhook provider instances: %w", err)
	}
	if len(instances) == 0 {
		return nil, payment.ErrProviderNotFound
	}

	providers := make([]payment.Provider, 0, len(instances))
	for _, inst := range instances {
		prov, provErr := s.createProviderFromInstance(ctx, inst)
		if provErr != nil {
			slog.Warn("skip webhook provider instance", "provider", providerKey, "instanceID", inst.ID, "error", provErr)
			continue
		}
		providers = append(providers, prov)
	}
	if len(providers) == 0 {
		return nil, payment.ErrProviderNotFound
	}
	return providers, nil
}
