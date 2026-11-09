package app

import (
	"context"
	exchangeapp "github.com/dujiao-next/internal/modules/settings/application/exchangerate"
	"time"
)

// exchangeRateRefreshService runs without Redis, sharing the persisted refresh interval with HTTP requests.
type exchangeRateRefreshService struct {
	rates *exchangeapp.ExchangeRateService
}

func (s *exchangeRateRefreshService) Name() string { return "exchange-rate-refresh" }
func (s *exchangeRateRefreshService) Start(ctx context.Context) error {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		refreshCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
		_, _ = s.rates.GetExchangeRate(refreshCtx, false)
		cancel()
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}
func (s *exchangeRateRefreshService) Stop(context.Context) error { return nil }
