package exchangerate

import (
	"context"
	"errors"
	"testing"
	"time"

	settingscontract "github.com/dujiao-next/internal/modules/settings/contract"
	"github.com/dujiao-next/internal/shared/jsonmap"
	"github.com/shopspring/decimal"
)

type memorySettings map[string]jsonmap.JSON

func (m memorySettings) GetByKey(key string) (jsonmap.JSON, error) { return m[key], nil }
func (m memorySettings) Update(key string, value map[string]interface{}) (jsonmap.JSON, error) {
	m[key] = jsonmap.JSON(value)
	return m[key], nil
}

type quoteFetcher struct {
	rates map[string]string
	calls int
}

func (f *quoteFetcher) Fetch(_ context.Context, source settingscontract.ExchangeRateSource, _ string) (settingscontract.ExchangeRateQuote, error) {
	f.calls++
	raw, ok := f.rates[source.ID]
	if !ok {
		return settingscontract.ExchangeRateQuote{}, errors.New("unavailable")
	}
	return settingscontract.ExchangeRateQuote{Rate: decimal.RequireFromString(raw), PublishedAt: time.Now().Add(-time.Hour)}, nil
}
func TestExchangeRatesAggregateFilterCacheAndMarkup(t *testing.T) {
	store := memorySettings{}
	fetcher := &quoteFetcher{rates: map[string]string{"frankfurter": "6.67", "open_er": "6.68", "currency_api": "6.69", "exchangerate_fun": "9"}}
	svc := NewExchangeRateService(store, fetcher)
	first, err := svc.GetExchangeRate(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if !first.MarketRate.Equal(decimal.RequireFromString("6.68")) || !first.Rate.Equal(decimal.RequireFromString("6.88")) || first.Sources[3].Status != "outlier" {
		t.Fatalf("unexpected aggregate: %+v", first)
	}
	if _, err = svc.GetExchangeRate(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if fetcher.calls != 4 {
		t.Fatal("fresh quotes were fetched again")
	}
	cfg := DefaultExchangeRateConfig()
	cfg.MarkupPercent = decimal.NewFromInt(1)
	if _, err = svc.UpdateConfig(cfg); err != nil {
		t.Fatal(err)
	}
	repriced, err := svc.GetExchangeRate(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if !repriced.Rate.Equal(decimal.RequireFromString("6.9468")) {
		t.Fatalf("markup not applied: %+v", repriced)
	}
	// A sustained jump stays rejected on subsequent refreshes, including cached mode.
	fetcher.rates = map[string]string{"frankfurter": "8", "open_er": "8", "currency_api": "8", "exchangerate_fun": "8"}
	for range 2 {
		next, err := svc.GetExchangeRate(context.Background(), true)
		if err != nil {
			t.Fatal(err)
		}
		if next.Mode != "cached" || !next.MarketRate.Equal(first.MarketRate) {
			t.Fatalf("price jump accepted: %+v", next)
		}
	}
}
func TestExchangeRateManualFallbackAndUnavailable(t *testing.T) {
	store := memorySettings{}
	fetcher := &quoteFetcher{}
	svc := NewExchangeRateService(store, fetcher)
	if _, err := svc.GetExchangeRate(context.Background(), false); !errors.Is(err, settingscontract.ErrExchangeRateUnavailable) {
		t.Fatalf("expected unavailable: %v", err)
	}
	cfg := DefaultExchangeRateConfig()
	cfg.ManualRate = decimal.RequireFromString("6.68")
	if _, err := svc.UpdateConfig(cfg); err != nil {
		t.Fatal(err)
	}
	manual, err := svc.GetExchangeRate(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if manual.Mode != "manual" || !manual.Rate.Equal(decimal.RequireFromString("6.88")) {
		t.Fatalf("manual rate: %+v", manual)
	}
	cfg.ManualRate = decimal.Zero
	cfg.FallbackRate = decimal.RequireFromString("6.8")
	if _, err = svc.UpdateConfig(cfg); err != nil {
		t.Fatal(err)
	}
	fallback, err := svc.GetExchangeRate(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if fallback.Mode != "fallback" || !fallback.Rate.Equal(decimal.NewFromInt(7)) {
		t.Fatalf("fallback rate: %+v", fallback)
	}
	cfg.Enabled = false
	if _, err = svc.UpdateConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.GetExchangeRate(context.Background(), false); !errors.Is(err, settingscontract.ErrExchangeRateUnavailable) {
		t.Fatalf("disabled must fail closed: %v", err)
	}
}
func TestExchangeRateConfigValidation(t *testing.T) {
	svc := NewExchangeRateService(memorySettings{}, &quoteFetcher{})
	for _, mutate := range []func(*settingscontract.ExchangeRateConfig){
		func(c *settingscontract.ExchangeRateConfig) { c.RefreshMinutes = 0 },
		func(c *settingscontract.ExchangeRateConfig) { c.ManualRate = decimal.NewFromInt(-1) },
		func(c *settingscontract.ExchangeRateConfig) { c.Sources[0].URL = "http://localhost" },
		func(c *settingscontract.ExchangeRateConfig) { c.Sources[0].ID = "unknown" },
		func(c *settingscontract.ExchangeRateConfig) { c.Sources[1].ID = c.Sources[0].ID },
	} {
		cfg := DefaultExchangeRateConfig()
		mutate(&cfg)
		if _, err := svc.UpdateConfig(cfg); !errors.Is(err, settingscontract.ErrExchangeRateInvalid) {
			t.Fatalf("invalid config accepted: %v", err)
		}
	}
}

func TestExchangeRateRejectsEffectiveRateRoundedToZero(t *testing.T) {
	svc := NewExchangeRateService(memorySettings{}, &quoteFetcher{})
	cfg := DefaultExchangeRateConfig()
	cfg.ManualRate = decimal.RequireFromString("0.0000001")
	cfg.MarkupAmount = decimal.Zero
	if _, err := svc.UpdateConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetExchangeRate(context.Background(), false); !errors.Is(err, settingscontract.ErrExchangeRateUnavailable) {
		t.Fatalf("zero effective rate accepted: %v", err)
	}
}
